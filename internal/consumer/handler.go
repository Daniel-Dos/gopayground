package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/dlq"
	"github.com/Daniel-Dos/gopayground/internal/events"
	"github.com/Daniel-Dos/gopayground/internal/history"
	"github.com/Daniel-Dos/gopayground/internal/idempotency"
	"github.com/Daniel-Dos/gopayground/internal/retry"
	"github.com/Daniel-Dos/gopayground/internal/status"
	"github.com/Daniel-Dos/gopayground/internal/validator"

	"github.com/IBM/sarama"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Handler implementa sarama.ConsumerGroupHandler para processar mensagens do Kafka.
type Handler struct {
	validator      validator.Validator
	idempotency    idempotency.Checker
	status         status.Updater
	history        history.Recorder
	retry          retry.Handler
	dlq            dlq.Producer
	redisClient    *redis.Client
	eventPublisher events.Publisher

	semaphore chan struct{}
	logger    *slog.Logger
	tracer    trace.Tracer

	// Métricas OTel para observabilidade do consumer
	messagesReceived   metric.Int64Counter
	messagesProcessed  metric.Int64Counter
	processingDuration metric.Float64Histogram
	retryAttempts      metric.Int64Counter
	dlqPublished       metric.Int64Counter
	idempotencyHits    metric.Int64Counter
}

// NewHandler cria um novo handler para processar mensagens do consumidor Kafka.
func NewHandler(
	validator validator.Validator,
	idempotency idempotency.Checker,
	status status.Updater,
	history history.Recorder,
	retry retry.Handler,
	dlq dlq.Producer,
	redisClient *redis.Client,
	eventPublisher events.Publisher,
	workerCount int,
	meter metric.Meter,
	tracer trace.Tracer,
) *Handler {
	h := &Handler{
		validator:      validator,
		idempotency:    idempotency,
		status:         status,
		history:        history,
		retry:          retry,
		dlq:            dlq,
		redisClient:    redisClient,
		eventPublisher: eventPublisher,
		semaphore:      make(chan struct{}, workerCount),
		logger:         slog.Default(),
		tracer:         tracer,
	}

	h.initMetrics(meter)
	return h
}

func (h *Handler) initMetrics(meter metric.Meter) {
	var err error

	h.messagesReceived, err = meter.Int64Counter("payment.consumer.messages_received",
		metric.WithDescription("Total messages received from Kafka"),
	)
	if err != nil {
		h.logger.Warn("failed to create messages_received counter", "error", err)
	}

	h.messagesProcessed, err = meter.Int64Counter("payment.consumer.messages_processed",
		metric.WithDescription("Total messages processed"),
	)
	if err != nil {
		h.logger.Warn("failed to create messages_processed counter", "error", err)
	}

	h.processingDuration, err = meter.Float64Histogram("payment.consumer.processing_duration",
		metric.WithDescription("Processing duration in ms"),
		metric.WithExplicitBucketBoundaries(1, 5, 10, 25, 50, 100, 250, 500, 1000),
	)
	if err != nil {
		h.logger.Warn("failed to create processing_duration histogram", "error", err)
	}

	h.retryAttempts, err = meter.Int64Counter("payment.consumer.retry_attempts",
		metric.WithDescription("Total retry attempts"),
	)
	if err != nil {
		h.logger.Warn("failed to create retry_attempts counter", "error", err)
	}

	h.dlqPublished, err = meter.Int64Counter("payment.consumer.dlq_published",
		metric.WithDescription("Total messages published to DLQ"),
	)
	if err != nil {
		h.logger.Warn("failed to create dlq_published counter", "error", err)
	}

	h.idempotencyHits, err = meter.Int64Counter("payment.consumer.idempotency_hits",
		metric.WithDescription("Total idempotency hits (duplicates)"),
	)
	if err != nil {
		h.logger.Warn("failed to create idempotency_hits counter", "error", err)
	}
}

// Setup é executado no início de uma nova sessão, antes do ConsumeClaim.
func (h *Handler) Setup(session sarama.ConsumerGroupSession) error {
	h.logger.Info("consumer session started",
		"member_id", session.MemberID(),
		"generation_id", session.GenerationID(),
	)
	return nil
}

// Cleanup é executado ao final de uma sessão, após todas as goroutines do ConsumeClaim terminarem.
func (h *Handler) Cleanup(session sarama.ConsumerGroupSession) error {
	h.logger.Info("consumer session ended",
		"member_id", session.MemberID(),
		"generation_id", session.GenerationID(),
	)
	return nil
}

// ConsumeClaim inicia o loop de consumo das mensagens do ConsumerGroupClaim.
func (h *Handler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		h.messagesReceived.Add(session.Context(), 1,
			metric.WithAttributes(
				attribute.Int("partition", int(msg.Partition)),
			),
		)

		msgCtx := session.Context()
		msgCtx, span := h.tracer.Start(msgCtx, "process_message",
			trace.WithAttributes(
				attribute.Int64("offset", msg.Offset),
				attribute.String("partition", strconv.Itoa(int(msg.Partition))),
				attribute.String("messaging.system", "kafka"),
				attribute.String("messaging.destination", msg.Topic),
				attribute.Int64("messaging.kafka.partition", int64(msg.Partition)),
				attribute.Int64("messaging.kafka.offset", msg.Offset),
			),
		)

		start := time.Now()
		err := h.processMessage(msgCtx, msg)
		duration := time.Since(start).Milliseconds()

		span.End()

		statusAttr := "success"
		if err != nil {
			statusAttr = "error"
		}
		h.messagesProcessed.Add(session.Context(), 1,
			metric.WithAttributes(attribute.String("status", statusAttr)),
		)
		h.processingDuration.Record(session.Context(), float64(duration),
			metric.WithAttributes(attribute.String("status", statusAttr)),
		)

		if err != nil {
			h.logger.ErrorContext(msgCtx, "message processing failed",
				"error", err,
				"offset", msg.Offset,
				"partition", msg.Partition,
				"duration_ms", duration,
			)
		} else {
			session.MarkMessage(msg, "")
		}
	}

	return nil
}

func (h *Handler) processMessage(ctx context.Context, msg *sarama.ConsumerMessage) error {
	// Acquire semaphore
	select {
	case h.semaphore <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-h.semaphore }()

	// 1. Validate payload
	event, err := h.validator.Validate(ctx, msg.Value)
	if err != nil {
		h.logger.WarnContext(ctx, "payload validation failed",
			"error", err,
			"offset", msg.Offset,
		)
		h.dlqPublished.Add(ctx, 1,
			metric.WithAttributes(attribute.String("reason", "validation")),
		)
		if dlqErr := h.dlq.Publish(ctx, msg, err); dlqErr != nil {
			h.logger.ErrorContext(ctx, "failed to publish to DLQ",
				"error", dlqErr,
			)
		}
		// Increment DLQ counter in Redis (best-effort)
		if h.redisClient != nil {
			if incrErr := h.redisClient.Incr(ctx, "dlq:count").Err(); incrErr != nil {
				h.logger.WarnContext(ctx, "failed to increment dlq:count",
					"error", incrErr,
				)
			}
		}
		return fmt.Errorf("validation error: %w", err)
	}

	// Add payment_id to span
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("payment_id", event.PaymentID))

	h.logger.DebugContext(ctx, "message received",
		"payment_id", event.PaymentID,
		"status", event.Status,
		"offset", msg.Offset,
		"partition", msg.Partition,
	)

	// 2. Idempotency check
	checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
	defer checkCancel()

	processed, err := h.idempotency.IsProcessed(checkCtx, event.PaymentID)
	if err != nil {
		h.logger.WarnContext(ctx, "idempotency check failed, proceeding optimistically",
			"error", err,
			"payment_id", event.PaymentID,
		)
	}
	if processed {
		h.idempotencyHits.Add(ctx, 1)
		h.logger.InfoContext(ctx, "message already processed, skipping",
			"payment_id", event.PaymentID,
		)
		return nil
	}

	// 3. Mark as processed (best effort — if fails, next consumer may reprocess)
	markCtx, markCancel := context.WithTimeout(ctx, 5*time.Second)
	defer markCancel()

	if err := h.idempotency.MarkProcessed(markCtx, event.PaymentID); err != nil {
		h.logger.WarnContext(ctx, "failed to mark idempotency",
			"error", err,
			"payment_id", event.PaymentID,
		)
	}

	// 4. Process with retry (status + history)
	var attemptNum int
	err = h.retry.Do(ctx, func(retryCtx context.Context) error {
		attemptNum++
		h.retryAttempts.Add(ctx, 1,
			metric.WithAttributes(attribute.Int("attempt", attemptNum)),
		)

		// Update status in Redis
		statusCtx, statusCancel := context.WithTimeout(retryCtx, 5*time.Second)
		defer statusCancel()

		if err := h.status.UpdateStatus(statusCtx, event.PaymentID, event.Status); err != nil {
			return fmt.Errorf("status update failed: %w", err)
		}

		// Record history in DynamoDB
		historyCtx, historyCancel := context.WithTimeout(retryCtx, 10*time.Second)
		defer historyCancel()

		if err := h.history.RecordHistory(historyCtx, event); err != nil {
			return fmt.Errorf("history record failed: %w", err)
		}

		return nil
	})

	if err != nil {
		// Retry exhausted → DLQ
		h.dlqPublished.Add(ctx, 1,
			metric.WithAttributes(attribute.String("reason", "retry")),
		)
		h.logger.ErrorContext(ctx, "retry exhausted, publishing to DLQ",
			"payment_id", event.PaymentID,
			"error", err,
		)
		if dlqErr := h.dlq.Publish(ctx, msg, err); dlqErr != nil {
			h.logger.ErrorContext(ctx, "failed to publish to DLQ after retry exhaustion",
				"error", dlqErr,
				"payment_id", event.PaymentID,
			)
		}
		// Increment DLQ counter in Redis (best-effort)
		if h.redisClient != nil {
			if incrErr := h.redisClient.Incr(ctx, "dlq:count").Err(); incrErr != nil {
				h.logger.WarnContext(ctx, "failed to increment dlq:count",
					"error", incrErr,
				)
			}
		}
		return err
	}

	h.logger.InfoContext(ctx, "message processed successfully",
		"payment_id", event.PaymentID,
		"status", event.Status,
	)

	// Best-effort publish to EventBus so the UI SSE feed receives the event.
	if pubErr := h.eventPublisher.Publish(ctx, event); pubErr != nil {
		h.logger.WarnContext(ctx, "failed to publish event to EventBus",
			"error", pubErr,
			"payment_id", event.PaymentID,
		)
	}

	return nil
}
