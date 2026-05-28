package consumer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Daniel-Dos/gopayground/internal/consumer"
	"github.com/Daniel-Dos/gopayground/internal/events"
	"github.com/Daniel-Dos/gopayground/internal/models"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

// ----- mocks for each interface -----

type mockValidator struct {
	validateFunc func(ctx context.Context, data []byte) (*models.PaymentEvent, error)
}

func (m *mockValidator) Validate(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
	return m.validateFunc(ctx, data)
}

type mockIdempotency struct {
	isProcessedFunc   func(ctx context.Context, paymentID string) (bool, error)
	markProcessedFunc func(ctx context.Context, paymentID string) error
}

func (m *mockIdempotency) IsProcessed(ctx context.Context, paymentID string) (bool, error) {
	return m.isProcessedFunc(ctx, paymentID)
}

func (m *mockIdempotency) MarkProcessed(ctx context.Context, paymentID string) error {
	return m.markProcessedFunc(ctx, paymentID)
}

type mockStatus struct {
	updateStatusFunc func(ctx context.Context, paymentID string, status string) error
}

func (m *mockStatus) UpdateStatus(ctx context.Context, paymentID string, status string) error {
	return m.updateStatusFunc(ctx, paymentID, status)
}

type mockHistory struct {
	recordHistoryFunc func(ctx context.Context, event *models.PaymentEvent) error
}

func (m *mockHistory) RecordHistory(ctx context.Context, event *models.PaymentEvent) error {
	return m.recordHistoryFunc(ctx, event)
}

type mockRetry struct {
	doFunc func(ctx context.Context, fn func(context.Context) error) error
}

func (m *mockRetry) Do(ctx context.Context, fn func(context.Context) error) error {
	return m.doFunc(ctx, fn)
}

type mockDLQ struct {
	publishFunc func(ctx context.Context, msg *sarama.ConsumerMessage, err error) error
}

func (m *mockDLQ) Publish(ctx context.Context, msg *sarama.ConsumerMessage, err error) error {
	return m.publishFunc(ctx, msg, err)
}

type mockPublisher struct {
	publishFunc func(ctx context.Context, event *models.PaymentEvent) error
}

func (m *mockPublisher) Publish(ctx context.Context, event *models.PaymentEvent) error {
	return m.publishFunc(ctx, event)
}

// ----- sarama mocks -----

type mockSession struct {
	ctx       context.Context
	markedMsg *sarama.ConsumerMessage
}

func (m *mockSession) Context() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m *mockSession) MarkMessage(msg *sarama.ConsumerMessage, metadata string) {
	m.markedMsg = msg
}

func (m *mockSession) MarkOffset(topic string, partition int32, offset int64, metadata string) {}
func (m *mockSession) ResetOffset(topic string, partition int32, offset int64, metadata string) {}
func (m *mockSession) Claims() map[string][]int32  { return nil }
func (m *mockSession) MemberID() string            { return "test-member" }
func (m *mockSession) GenerationID() int32         { return 1 }
func (m *mockSession) Commit()                     {}

type mockClaim struct {
	messages []*sarama.ConsumerMessage
}

func (m *mockClaim) Topic() string                            { return "payment.events" }
func (m *mockClaim) Partition() int32                         { return 0 }
func (m *mockClaim) InitialOffset() int64                     { return 0 }
func (m *mockClaim) HighWaterMarkOffset() int64               { return 0 }
func (m *mockClaim) Messages() <-chan *sarama.ConsumerMessage {
	ch := make(chan *sarama.ConsumerMessage, len(m.messages))
	for _, msg := range m.messages {
		ch <- msg
	}
	close(ch)
	return ch
}

// ----- helpers -----

func noopMeter() metric.Meter {
	return noop.Meter{}
}

func noopTracer() trace.Tracer {
	return otel.Tracer("test")
}

func noopPublisher() events.Publisher {
	return &mockPublisher{
		publishFunc: func(ctx context.Context, event *models.PaymentEvent) error { return nil },
	}
}

func defaultValidEvent() *models.PaymentEvent {
	return &models.PaymentEvent{
		PaymentID: "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		Status:    "confirmed",
		Amount:    100.0,
		Currency:  "USD",
		Timestamp: "2026-05-24T10:00:00Z",
	}
}

// ----- tests -----

func TestConsumeClaimValidMessage(t *testing.T) {
	v := &mockValidator{
		validateFunc: func(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
			return defaultValidEvent(), nil
		},
	}
	i := &mockIdempotency{
		isProcessedFunc:   func(ctx context.Context, paymentID string) (bool, error) { return false, nil },
		markProcessedFunc: func(ctx context.Context, paymentID string) error { return nil },
	}
	s := &mockStatus{
		updateStatusFunc: func(ctx context.Context, paymentID string, status string) error { return nil },
	}
	h := &mockHistory{
		recordHistoryFunc: func(ctx context.Context, event *models.PaymentEvent) error { return nil },
	}
	r := &mockRetry{
		doFunc: func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
	}
	d := &mockDLQ{
		publishFunc: func(ctx context.Context, msg *sarama.ConsumerMessage, err error) error { return nil },
	}

	handler := consumer.NewHandler(v, i, s, h, r, d, nil, noopPublisher(), 10, noopMeter(), noopTracer())
	msg := &sarama.ConsumerMessage{Value: []byte(`{}`)}

	session := &mockSession{ctx: context.Background()}
	err := handler.ConsumeClaim(session, &mockClaim{messages: []*sarama.ConsumerMessage{msg}})
	require.NoError(t, err)
	assert.NotNil(t, session.markedMsg, "message should be marked as processed")
}

func TestConsumeClaimDuplicate(t *testing.T) {
	statusCalled := false

	v := &mockValidator{
		validateFunc: func(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
			return defaultValidEvent(), nil
		},
	}
	i := &mockIdempotency{
		isProcessedFunc:   func(ctx context.Context, paymentID string) (bool, error) { return true, nil },
		markProcessedFunc: func(ctx context.Context, paymentID string) error { return nil },
	}
	s := &mockStatus{
		updateStatusFunc: func(ctx context.Context, paymentID string, status string) error {
			statusCalled = true
			return nil
		},
	}

	handler := consumer.NewHandler(v, i, s,
		&mockHistory{recordHistoryFunc: func(ctx context.Context, event *models.PaymentEvent) error { return nil }},
		&mockRetry{doFunc: func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }},
		&mockDLQ{publishFunc: func(ctx context.Context, msg *sarama.ConsumerMessage, err error) error { return nil }},
		nil, noopPublisher(), 10, noopMeter(), noopTracer())

	msg := &sarama.ConsumerMessage{Value: []byte(`{}`)}
	session := &mockSession{ctx: context.Background()}

	err := handler.ConsumeClaim(session, &mockClaim{messages: []*sarama.ConsumerMessage{msg}})
	require.NoError(t, err)
	assert.False(t, statusCalled, "status should not be updated for duplicate message")
	assert.NotNil(t, session.markedMsg, "duplicate message should still be marked (skipped)")
}

func TestConsumeClaimInvalidPayload(t *testing.T) {
	dlqCalled := false

	v := &mockValidator{
		validateFunc: func(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
			return nil, errors.New("invalid payload")
		},
	}
	d := &mockDLQ{
		publishFunc: func(ctx context.Context, msg *sarama.ConsumerMessage, err error) error {
			dlqCalled = true
			return nil
		},
	}

	idempotencyCalled := false
	i := &mockIdempotency{
		isProcessedFunc: func(ctx context.Context, paymentID string) (bool, error) {
			idempotencyCalled = true
			return false, nil
		},
	}

	handler := consumer.NewHandler(v, i,
		&mockStatus{updateStatusFunc: func(ctx context.Context, paymentID string, status string) error { return nil }},
		&mockHistory{recordHistoryFunc: func(ctx context.Context, event *models.PaymentEvent) error { return nil }},
		&mockRetry{doFunc: func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }},
		d, nil, noopPublisher(), 10, noopMeter(), noopTracer())

	msg := &sarama.ConsumerMessage{Value: []byte(`invalid`)}
	session := &mockSession{ctx: context.Background()}

	// ConsumeClaim should not return an error as it logs and continues
	err := handler.ConsumeClaim(session, &mockClaim{messages: []*sarama.ConsumerMessage{msg}})
	require.NoError(t, err)
	assert.True(t, dlqCalled, "DLQ should be called for invalid payload")
	assert.False(t, idempotencyCalled, "idempotency should not be called for invalid payload")
	assert.Nil(t, session.markedMsg, "invalid payload should NOT be marked (goes to DLQ but not committed)")
}

func TestConsumeClaimRetryExhaustion(t *testing.T) {
	dlqCalled := false

	v := &mockValidator{
		validateFunc: func(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
			return defaultValidEvent(), nil
		},
	}
	r := &mockRetry{
		doFunc: func(ctx context.Context, fn func(context.Context) error) error {
			return errors.New("retry exhausted after 3 attempts")
		},
	}
	d := &mockDLQ{
		publishFunc: func(ctx context.Context, msg *sarama.ConsumerMessage, err error) error {
			dlqCalled = true
			assert.Contains(t, err.Error(), "retry exhausted")
			return nil
		},
	}

	handler := consumer.NewHandler(v,
		&mockIdempotency{
			isProcessedFunc:   func(ctx context.Context, paymentID string) (bool, error) { return false, nil },
			markProcessedFunc: func(ctx context.Context, paymentID string) error { return nil },
		},
		&mockStatus{updateStatusFunc: func(ctx context.Context, paymentID string, status string) error { return nil }},
		&mockHistory{recordHistoryFunc: func(ctx context.Context, event *models.PaymentEvent) error { return nil }},
		r, d, nil, noopPublisher(), 10, noopMeter(), noopTracer())

	msg := &sarama.ConsumerMessage{Value: []byte(`{}`)}
	session := &mockSession{ctx: context.Background()}

	err := handler.ConsumeClaim(session, &mockClaim{messages: []*sarama.ConsumerMessage{msg}})
	require.NoError(t, err)
	assert.True(t, dlqCalled)
	assert.Nil(t, session.markedMsg, "message should NOT be marked on retry exhaustion")
}

func TestConsumeClaimIdempotencyFallback(t *testing.T) {
	v := &mockValidator{
		validateFunc: func(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
			return defaultValidEvent(), nil
		},
	}
	i := &mockIdempotency{
		isProcessedFunc: func(ctx context.Context, paymentID string) (bool, error) {
			return false, errors.New("redis down")
		},
		markProcessedFunc: func(ctx context.Context, paymentID string) error {
			return errors.New("redis down")
		},
	}

	statusCalled := false
	s := &mockStatus{
		updateStatusFunc: func(ctx context.Context, paymentID string, status string) error {
			statusCalled = true
			return nil
		},
	}

	handler := consumer.NewHandler(v, i, s,
		&mockHistory{recordHistoryFunc: func(ctx context.Context, event *models.PaymentEvent) error { return nil }},
		&mockRetry{doFunc: func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }},
		&mockDLQ{publishFunc: func(ctx context.Context, msg *sarama.ConsumerMessage, err error) error { return nil }},
		nil, noopPublisher(), 10, noopMeter(), noopTracer())

	msg := &sarama.ConsumerMessage{Value: []byte(`{}`)}
	session := &mockSession{ctx: context.Background()}

	err := handler.ConsumeClaim(session, &mockClaim{messages: []*sarama.ConsumerMessage{msg}})
	require.NoError(t, err)
	assert.True(t, statusCalled, "processing should continue despite idempotency failure")
	assert.NotNil(t, session.markedMsg, "message should be marked on successful processing")
}

func TestSetupAndCleanup(t *testing.T) {
	handler := consumer.NewHandler(
		&mockValidator{validateFunc: func(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
			return defaultValidEvent(), nil
		}},
		&mockIdempotency{
			isProcessedFunc:   func(ctx context.Context, paymentID string) (bool, error) { return false, nil },
			markProcessedFunc: func(ctx context.Context, paymentID string) error { return nil },
		},
		&mockStatus{updateStatusFunc: func(ctx context.Context, paymentID string, status string) error { return nil }},
		&mockHistory{recordHistoryFunc: func(ctx context.Context, event *models.PaymentEvent) error { return nil }},
		&mockRetry{doFunc: func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }},
		&mockDLQ{publishFunc: func(ctx context.Context, msg *sarama.ConsumerMessage, err error) error { return nil }},
		nil, noopPublisher(), 10, noopMeter(), noopTracer(),
	)

	session := &mockSession{ctx: context.Background()}
	err := handler.Setup(session)
	require.NoError(t, err)

	err = handler.Cleanup(session)
	require.NoError(t, err)
}
