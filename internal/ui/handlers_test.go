package ui_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Daniel-Dos/gopayground/internal/models"
	"github.com/Daniel-Dos/gopayground/internal/ui"

	"github.com/alicebob/miniredis/v2"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDynamoDB implements ui.DynamoDBQueryAPI for testing.
type mockDynamoDB struct {
	queryFunc func(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

func (m *mockDynamoDB) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	return m.queryFunc(ctx, params, optFns...)
}

func newTestHandlers(t *testing.T) (*ui.Handlers, *redis.Client, *miniredis.Miniredis, *mockDynamoDB) {
	t.Helper()
	client, mr := setupMiniredis(t)

	mockDyn := &mockDynamoDB{
		queryFunc: func(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil
		},
	}

	logger := newTestLogger()
	h := ui.NewHandlers(client, mockDyn, "test_history", nil, "", logger)

	return h, client, mr, mockDyn
}

// --- handleListPayments tests ---

func TestHandleListPayments_Empty(t *testing.T) {
	h, _, _, _ := newTestHandlers(t)

	req := httptest.NewRequest("GET", "/api/payments", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/payments", h.HandleListPayments)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var payments []map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &payments)
	require.NoError(t, err)
	assert.Empty(t, payments)
}

func TestHandleListPayments_WithData(t *testing.T) {
	h, _, mr, _ := newTestHandlers(t)

	setupRedisPayment(t, mr, "payment-1", "confirmed")
	setupRedisPayment(t, mr, "payment-2", "pending")
	setupRedisPayment(t, mr, "payment-3", "failed")

	req := httptest.NewRequest("GET", "/api/payments", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/payments", h.HandleListPayments)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var payments []map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &payments)
	require.NoError(t, err)
	assert.Len(t, payments, 3)
}

func TestHandleListPayments_FilterByPaymentID(t *testing.T) {
	h, _, mr, _ := newTestHandlers(t)

	setupRedisPayment(t, mr, "abc-123", "confirmed")
	setupRedisPayment(t, mr, "def-456", "pending")
	setupRedisPayment(t, mr, "abc-789", "failed")

	req := httptest.NewRequest("GET", "/api/payments?payment_id=abc", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/payments", h.HandleListPayments)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var payments []map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &payments)
	require.NoError(t, err)
	assert.Len(t, payments, 2)
}

func TestHandleListPayments_FilterByStatus(t *testing.T) {
	h, _, mr, _ := newTestHandlers(t)

	setupRedisPayment(t, mr, "payment-1", "confirmed")
	setupRedisPayment(t, mr, "payment-2", "pending")
	setupRedisPayment(t, mr, "payment-3", "confirmed")

	req := httptest.NewRequest("GET", "/api/payments?status=confirmed", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/payments", h.HandleListPayments)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var payments []map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &payments)
	require.NoError(t, err)
	assert.Len(t, payments, 2)
	for _, p := range payments {
		assert.Equal(t, "confirmed", p["status"])
	}
}

func TestHandleListPayments_FilterCombined(t *testing.T) {
	h, _, mr, _ := newTestHandlers(t)

	setupRedisPayment(t, mr, "abc-123", "confirmed")
	setupRedisPayment(t, mr, "abc-456", "pending")
	setupRedisPayment(t, mr, "def-789", "confirmed")

	req := httptest.NewRequest("GET", "/api/payments?payment_id=abc&status=confirmed", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/payments", h.HandleListPayments)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var payments []map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &payments)
	require.NoError(t, err)
	assert.Len(t, payments, 1)
	assert.Equal(t, "abc-123", payments[0]["payment_id"])
}

func TestHandleListPayments_InputValidation(t *testing.T) {
	h, _, _, _ := newTestHandlers(t)

	longID := fmt.Sprintf("%065d", 0)
	req := httptest.NewRequest("GET", "/api/payments?payment_id="+longID, nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/payments", h.HandleListPayments)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- handlePaymentHistory tests ---

func TestHandlePaymentHistory_Empty(t *testing.T) {
	h, _, _, _ := newTestHandlers(t)

	req := httptest.NewRequest("GET", "/api/payments/nonexistent/history", nil)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/payments/{id}/history", h.HandlePaymentHistory)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var history []map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &history)
	require.NoError(t, err)
	assert.Empty(t, history)
}

func TestHandlePaymentHistory_WithData(t *testing.T) {
	client, mr := setupMiniredis(t)
	defer client.Close()

	mockDyn := &mockDynamoDB{
		queryFunc: func(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			item := models.PaymentHistory{
				PaymentID:   "test-payment",
				Status:      "confirmed",
				Amount:      100.0,
				Currency:    "USD",
				Description: "test",
				Timestamp:   "2026-05-24T10:00:00Z",
				TraceID:     "trace-123",
			}
			av, err := attributevalue.MarshalMap(item)
			require.NoError(t, err)
			return &dynamodb.QueryOutput{
				Items: []map[string]types.AttributeValue{av},
			}, nil
		},
	}

	logger := newTestLogger()
	h := ui.NewHandlers(client, mockDyn, "test_history", nil, "", logger)
	_ = mr

	req := httptest.NewRequest("GET", "/api/payments/test-payment/history", nil)
	req.SetPathValue("id", "test-payment")
	w := httptest.NewRecorder()

	// Call handler directly to avoid mux routing issues
	h.HandlePaymentHistory(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var history []map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &history)
	require.NoError(t, err)
	assert.Len(t, history, 1)
	assert.Equal(t, "test-payment", history[0]["payment_id"])
	assert.Equal(t, "confirmed", history[0]["status"])
}

func TestHandlePaymentHistory_MissingID(t *testing.T) {
	h, _, _, _ := newTestHandlers(t)

	req := httptest.NewRequest("GET", "/api/payments//history", nil)
	req.SetPathValue("id", "")
	w := httptest.NewRecorder()

	// Call handler directly to avoid mux routing issues with empty path value
	h.HandlePaymentHistory(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp["error"], "payment_id")
}

func TestHandlePaymentHistory_DynamoDBError(t *testing.T) {
	client, _ := setupMiniredis(t)
	defer client.Close()

	mockDyn := &mockDynamoDB{
		queryFunc: func(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return nil, fmt.Errorf("dynamodb error")
		},
	}

	logger := newTestLogger()
	h := ui.NewHandlers(client, mockDyn, "test_history", nil, "", logger)

	req := httptest.NewRequest("GET", "/api/payments/test-id/history", nil)
	req.SetPathValue("id", "test-id")
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/payments/{id}/history", h.HandlePaymentHistory)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandlePaymentHistory_InputValidation(t *testing.T) {
	h, _, _, _ := newTestHandlers(t)

	longID := fmt.Sprintf("%065d", 0)
	req := httptest.NewRequest("GET", "/api/payments/"+longID+"/history", nil)
	req.SetPathValue("id", longID)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/payments/{id}/history", h.HandlePaymentHistory)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- handleMetrics tests ---

func TestHandleMetrics_Empty(t *testing.T) {
	h, _, _, _ := newTestHandlers(t)

	req := httptest.NewRequest("GET", "/api/metrics", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/metrics", h.HandleMetrics)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var metrics map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &metrics)
	require.NoError(t, err)
	assert.Equal(t, float64(0), metrics["total_processed"])
	assert.Equal(t, float64(0), metrics["success_rate"])
	assert.NotNil(t, metrics["by_status"])
	assert.Equal(t, float64(0), metrics["dlq_count"])
}

func TestHandleMetrics_WithData(t *testing.T) {
	h, _, mr, _ := newTestHandlers(t)

	setupRedisPayment(t, mr, "p1", "confirmed")
	setupRedisPayment(t, mr, "p2", "confirmed")
	setupRedisPayment(t, mr, "p3", "failed")
	setupRedisPayment(t, mr, "p4", "pending")
	setupRedisPayment(t, mr, "p5", "refunded")

	req := httptest.NewRequest("GET", "/api/metrics", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/metrics", h.HandleMetrics)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var metrics map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &metrics)
	require.NoError(t, err)
	assert.Equal(t, float64(5), metrics["total_processed"])

	byStatus := metrics["by_status"].(map[string]interface{})
	assert.Equal(t, float64(2), byStatus["confirmed"])
	assert.Equal(t, float64(1), byStatus["failed"])
	assert.Equal(t, float64(1), byStatus["pending"])
	assert.Equal(t, float64(1), byStatus["refunded"])

	// success_rate = 2 / (2+1+1) = 50%
	assert.InDelta(t, 50.0, metrics["success_rate"], 0.01)
}

// --- handleHealth tests ---

func TestHandleHealth_OK(t *testing.T) {
	h, _, _, _ := newTestHandlers(t)

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.HandleHealth)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "ok", result["status"])
	assert.Equal(t, "connected", result["redis"])
}

func TestHandleHealth_RedisDown(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:16379", // wrong port
	})
	defer client.Close()

	logger := newTestLogger()
	h := ui.NewHandlers(client, &mockDynamoDB{}, "test_history", nil, "", logger)

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.HandleHealth)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// --- handleSSE tests ---

func TestHandleSSE_NoEventBus(t *testing.T) {
	logger := newTestLogger()
	h := ui.NewHandlers(nil, &mockDynamoDB{}, "test_history", nil, "", logger)

	req := httptest.NewRequest("GET", "/api/events", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/events", h.HandleSSE)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
