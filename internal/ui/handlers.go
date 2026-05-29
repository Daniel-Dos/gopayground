package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Semáforo SSE limita conexões SSE concorrentes.
var sseSemaphore = make(chan struct{}, 100)

// Contadores de conexão SSE.
var (
	sseConnections      int64
	sseTotalConnections int64
)

// Metrics representa a resposta agregada de métricas.
type Metrics struct {
	TotalProcessed int            `json:"total_processed"`
	ByStatus       map[string]int `json:"by_status"`
	SuccessRate    float64        `json:"success_rate"`
	DLQCount       int            `json:"dlq_count"`
}

// DynamoDBQueryAPI define a interface para operações Query do DynamoDB.
type DynamoDBQueryAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

// Handlers contém as dependências para os handlers HTTP.
type Handlers struct {
	redis       *redis.Client
	dynamo      DynamoDBQueryAPI
	dynamoTbl   string
	eventBus    *EventBus
	producerURL string
	httpClient  *http.Client
	logger      *slog.Logger
}

// NewHandlers cria uma nova instância de Handlers.
func NewHandlers(rdb *redis.Client, dynamoClient DynamoDBQueryAPI, dynamoTbl string, eventBus *EventBus, producerURL string, logger *slog.Logger) *Handlers {
	return &Handlers{
		redis:       rdb,
		dynamo:      dynamoClient,
		dynamoTbl:   dynamoTbl,
		eventBus:    eventBus,
		producerURL: producerURL,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		logger:      logger,
	}
}

// HandleSSE transmite eventos em tempo real via Server-Sent Events.
func (h *Handlers) HandleSSE(w http.ResponseWriter, r *http.Request) {
	if h.eventBus == nil {
		http.Error(w, "event bus not available", http.StatusInternalServerError)
		return
	}

	// Acquire semaphore
	select {
	case sseSemaphore <- struct{}{}:
		defer func() { <-sseSemaphore }()
	default:
		h.logger.Warn("SSE semaphore full, rejecting connection", "max_connections", 100)
		http.Error(w, "too many connections", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	atomic.AddInt64(&sseConnections, 1)
	atomic.AddInt64(&sseTotalConnections, 1)
	defer atomic.AddInt64(&sseConnections, -1)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cancel := h.eventBus.Subscribe()
	defer cancel()

	h.logger.Debug("SSE client connected", "remote_addr", r.RemoteAddr)
	defer h.logger.Debug("SSE client disconnected", "remote_addr", r.RemoteAddr)

	// Send initial event so the client knows the connection is alive immediately
	fmt.Fprintf(w, "event: heartbeat\ndata: {\"connected\":true}\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				h.logger.Error("failed to marshal SSE event", "error", err)
				continue
			}
			h.logger.InfoContext(r.Context(), "sending SSE payment event",
				"payment_id", event.PaymentID,
				"status", event.Status,
			)
			fmt.Fprintf(w, "event: payment\ndata: %s\n\n", data)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprintf(w, "event: heartbeat\ndata: {}\n\n")
			flusher.Flush()
		}
	}
}

// HandleListPayments lista pagamentos do Redis com filtros opcionais.
func (h *Handlers) HandleListPayments(w http.ResponseWriter, r *http.Request) {
	filterID := strings.TrimSpace(r.URL.Query().Get("payment_id"))
	filterStatus := strings.TrimSpace(r.URL.Query().Get("status"))

	// Validate input sizes
	if len(filterID) > 64 {
		writeError(w, http.StatusBadRequest, "payment_id too long (max 64)")
		return
	}
	if len(filterStatus) > 16 {
		writeError(w, http.StatusBadRequest, "status too long (max 16)")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var cursor uint64
	var payments []models.PaymentStatus

	for {
		keys, nextCursor, err := h.redis.Scan(ctx, cursor, "payment:*", 100).Result()
		if err != nil {
			h.logger.ErrorContext(ctx, "redis scan error", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to fetch payments")
			return
		}

		for _, key := range keys {
			paymentID := strings.TrimPrefix(key, "payment:")

			// Filter by payment_id (LIKE)
			if filterID != "" && !strings.Contains(strings.ToLower(paymentID), strings.ToLower(filterID)) {
				continue
			}

			fields, err := h.redis.HGetAll(ctx, key).Result()
			if err != nil {
				h.logger.WarnContext(ctx, "redis hgetall error", "key", key, "error", err)
				continue
			}

			status := fields["status"]

			// Filter by status (exact)
			if filterStatus != "" && !strings.EqualFold(status, filterStatus) {
				continue
			}

			payments = append(payments, models.PaymentStatus{
				PaymentID: paymentID,
				Status:    status,
				UpdatedAt: fields["updated_at"],
			})
		}

		if nextCursor == 0 {
			break
		}
		cursor = nextCursor
	}

	writeJSON(w, http.StatusOK, payments)
}

// HandlePaymentHistory retorna o histórico completo de um pagamento do DynamoDB.
func (h *Handlers) HandlePaymentHistory(w http.ResponseWriter, r *http.Request) {
	paymentID := r.PathValue("id")
	if paymentID == "" {
		writeError(w, http.StatusBadRequest, "payment_id is required")
		return
	}

	if len(paymentID) > 64 {
		writeError(w, http.StatusBadRequest, "payment_id too long (max 64)")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var historyItems []models.PaymentHistory

	result, err := h.dynamo.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(h.dynamoTbl),
		KeyConditionExpression: aws.String("payment_id = :pid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pid": &types.AttributeValueMemberS{Value: paymentID},
		},
	})
	if err != nil {
		h.logger.ErrorContext(ctx, "dynamodb query error",
			"payment_id", paymentID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch payment history")
		return
	}

	for _, item := range result.Items {
		var hItem models.PaymentHistory
		if err := attributevalue.UnmarshalMap(item, &hItem); err != nil {
			h.logger.WarnContext(ctx, "dynamodb unmarshal error",
				"payment_id", paymentID, "error", err)
			continue
		}
		historyItems = append(historyItems, hItem)
	}

	// Sort by timestamp ascending
	sort.Slice(historyItems, func(i, j int) bool {
		return historyItems[i].Timestamp < historyItems[j].Timestamp
	})

	writeJSON(w, http.StatusOK, historyItems)
}

// HandleMetrics retorna métricas agregadas do Redis.
func (h *Handlers) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	metrics := Metrics{
		ByStatus: make(map[string]int),
	}

	var cursor uint64
	for {
		keys, nextCursor, err := h.redis.Scan(ctx, cursor, "payment:*", 100).Result()
		if err != nil {
			h.logger.WarnContext(ctx, "redis scan error in metrics", "error", err)
			// Return partial metrics if scan fails
			break
		}
		for _, key := range keys {
			metrics.TotalProcessed++
			status, err := h.redis.HGet(ctx, key, "status").Result()
			if err == nil {
				metrics.ByStatus[status]++
			}
		}
		if nextCursor == 0 {
			break
		}
		cursor = nextCursor
	}

	total := metrics.ByStatus["confirmed"] + metrics.ByStatus["failed"] + metrics.ByStatus["refunded"]
	if total > 0 {
		metrics.SuccessRate = float64(metrics.ByStatus["confirmed"]) / float64(total) * 100
	}

	// Read dlq:count counter from Redis (best-effort)
	dlqCount, err := h.redis.Get(ctx, "dlq:count").Int()
	if err == nil {
		metrics.DLQCount = dlqCount
	} else if err != redis.Nil {
		h.logger.WarnContext(ctx, "failed to read dlq:count", "error", err)
	}

	writeJSON(w, http.StatusOK, metrics)
}

// HandleHealth realiza uma verificação de saúde (health check).
func (h *Handlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.redis.Ping(ctx).Err(); err != nil {
		h.logger.WarnContext(ctx, "health check failed", "component", "redis", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unhealthy",
			"redis":  "down",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"redis":  "connected",
	})
}

// --- Publish types ---

type publishRequest struct {
	PaymentID   string  `json:"payment_id"`
	Status      string  `json:"status"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	Description string  `json:"description"`
}

type publishResponse struct {
	Status    string `json:"status"`
	PaymentID string `json:"payment_id"`
	Partition int32  `json:"partition"`
	Offset    int64  `json:"offset"`
}

type bulkPublishRequest struct {
	Count int `json:"count"`
}

type bulkPublishItem struct {
	PaymentID string `json:"payment_id"`
	Status    string `json:"status"`
	Partition int32  `json:"partition,omitempty"`
	Offset    int64  `json:"offset,omitempty"`
	Error     string `json:"error,omitempty"`
}

// validStatuses contém os status de pagamento permitidos.
var validStatuses = map[string]bool{
	"pending":   true,
	"confirmed": true,
	"failed":    true,
	"refunded":  true,
}

// HandlePublish publica um único evento de pagamento via Producer HTTP e no EventBus.
func (h *Handlers) HandlePublish(w http.ResponseWriter, r *http.Request) {
	if h.producerURL == "" {
		writeError(w, http.StatusBadGateway, "producer not configured")
		return
	}

	ct := r.Header.Get("Content-Type")
	if ct != "" && ct != "application/json" && ct != "application/json; charset=utf-8" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var req publishRequest
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.PaymentID == "" {
		req.PaymentID = uuid.New().String()
	}
	if req.Status == "" {
		req.Status = "pending"
	}

	body, err := json.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode request")
		return
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", h.producerURL+"/publish", bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create request")
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		h.logger.ErrorContext(ctx, "producer HTTP call failed", "error", err)
		writeError(w, http.StatusBadGateway, "producer unavailable")
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to read producer response")
		return
	}

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		if json.Unmarshal(respBody, &errResp) == nil && errResp["error"] != "" {
			writeError(w, resp.StatusCode, errResp["error"])
		} else {
			writeError(w, http.StatusBadGateway, "producer returned "+resp.Status)
		}
		return
	}

	var publishResp publishResponse
	if err := json.Unmarshal(respBody, &publishResp); err != nil {
		writeError(w, http.StatusBadGateway, "invalid producer response")
		return
	}

	// Publish to EventBus so the SSE feed picks it up
	if h.eventBus != nil {
		event := &models.PaymentEvent{
			PaymentID:   publishResp.PaymentID,
			Status:      req.Status,
			Amount:      req.Amount,
			Currency:    req.Currency,
			Description: req.Description,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		if pubErr := h.eventBus.Publish(ctx, event); pubErr != nil {
			h.logger.WarnContext(ctx, "eventbus publish error", "payment_id", event.PaymentID, "error", pubErr)
		}
	}

	h.logger.InfoContext(ctx, "payment published",
		"payment_id", publishResp.PaymentID,
		"partition", publishResp.Partition,
		"offset", publishResp.Offset,
	)

	writeJSON(w, http.StatusOK, publishResp)
}

// HandlePublishBulk publica múltiplos eventos de pagamento via Producer HTTP.
func (h *Handlers) HandlePublishBulk(w http.ResponseWriter, r *http.Request) {
	if h.producerURL == "" {
		writeError(w, http.StatusBadGateway, "producer not configured")
		return
	}

	ct := r.Header.Get("Content-Type")
	if ct != "" && ct != "application/json" && ct != "application/json; charset=utf-8" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	var req bulkPublishRequest
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Count < 1 || req.Count > 100 {
		writeError(w, http.StatusBadRequest, "count must be between 1 and 100")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	body, err := json.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode request")
		return
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", h.producerURL+"/publish/bulk", bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create request")
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		h.logger.ErrorContext(ctx, "producer HTTP call failed", "error", err)
		writeError(w, http.StatusBadGateway, "producer unavailable")
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to read producer response")
		return
	}

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		if json.Unmarshal(respBody, &errResp) == nil && errResp["error"] != "" {
			writeError(w, resp.StatusCode, errResp["error"])
		} else {
			writeError(w, http.StatusBadGateway, "producer returned "+resp.Status)
		}
		return
	}

	var items []bulkPublishItem
	if err := json.Unmarshal(respBody, &items); err != nil {
		writeError(w, http.StatusBadGateway, "invalid producer response")
		return
	}

	writeJSON(w, http.StatusOK, items)
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
