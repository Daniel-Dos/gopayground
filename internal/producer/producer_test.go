package producer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/models"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSyncProducer struct {
	sendMessageFn func(msg *sarama.ProducerMessage) (int32, int64, error)
}

func (m *mockSyncProducer) SendMessage(msg *sarama.ProducerMessage) (int32, int64, error) {
	return m.sendMessageFn(msg)
}

type mockValidator struct {
	validateFn func(ctx context.Context, data []byte) (*models.PaymentEvent, error)
}

func (m *mockValidator) Validate(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
	return m.validateFn(ctx, data)
}

func TestPublish_Success(t *testing.T) {
	mockProd := &mockSyncProducer{
		sendMessageFn: func(msg *sarama.ProducerMessage) (int32, int64, error) {
			return 0, 42, nil
		},
	}
	mockVal := &mockValidator{
		validateFn: func(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
			var e models.PaymentEvent
			return &e, nil
		},
	}

	svc := New(mockProd, "test-topic", mockVal)
	events := []*models.PaymentEvent{
		{PaymentID: "550e8400-e29b-41d4-a716-446655440000", Status: "confirmed", Amount: 100, Currency: "BRL", Timestamp: time.Now().UTC().Format(time.RFC3339)},
	}

	results := svc.Publish(context.Background(), events, 0)

	require.Len(t, results, 1)
	assert.NoError(t, results[0].Error)
	assert.Equal(t, int32(0), results[0].Partition)
	assert.Equal(t, int64(42), results[0].Offset)
}

func TestPublish_ValidationError(t *testing.T) {
	mockProd := &mockSyncProducer{
		sendMessageFn: func(msg *sarama.ProducerMessage) (int32, int64, error) {
			return 0, 0, nil
		},
	}
	mockVal := &mockValidator{
		validateFn: func(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
			return nil, errors.New("validation error: amount must be greater than 0")
		},
	}

	svc := New(mockProd, "test-topic", mockVal)
	events := []*models.PaymentEvent{
		{PaymentID: "550e8400-e29b-41d4-a716-446655440000", Status: "confirmed", Amount: 0, Currency: "BRL", Timestamp: time.Now().UTC().Format(time.RFC3339)},
	}

	results := svc.Publish(context.Background(), events, 0)

	require.Len(t, results, 1)
	assert.Error(t, results[0].Error)
	assert.Contains(t, results[0].Error.Error(), "validation error")
}

func TestPublish_KafkaError(t *testing.T) {
	mockProd := &mockSyncProducer{
		sendMessageFn: func(msg *sarama.ProducerMessage) (int32, int64, error) {
			return 0, 0, errors.New("kafka: connection refused")
		},
	}
	mockVal := &mockValidator{
		validateFn: func(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
			var e models.PaymentEvent
			return &e, nil
		},
	}

	svc := New(mockProd, "test-topic", mockVal)
	events := []*models.PaymentEvent{
		{PaymentID: "550e8400-e29b-41d4-a716-446655440000", Status: "confirmed", Amount: 100, Currency: "BRL", Timestamp: time.Now().UTC().Format(time.RFC3339)},
	}

	results := svc.Publish(context.Background(), events, 0)

	require.Len(t, results, 1)
	assert.Error(t, results[0].Error)
	assert.Contains(t, results[0].Error.Error(), "connection refused")
}

func TestPublish_InvalidEventContinues(t *testing.T) {
	mockProd := &mockSyncProducer{
		sendMessageFn: func(msg *sarama.ProducerMessage) (int32, int64, error) {
			return 0, 42, nil
		},
	}

	callCount := 0
	mockVal := &mockValidator{
		validateFn: func(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
			callCount++
			if callCount == 1 {
				return nil, errors.New("validation error: invalid")
			}
			var e models.PaymentEvent
			return &e, nil
		},
	}

	svc := New(mockProd, "test-topic", mockVal)
	events := []*models.PaymentEvent{
		{PaymentID: "11111111-1111-1111-1111-111111111111", Status: "confirmed", Amount: 0, Currency: "BRL", Timestamp: time.Now().UTC().Format(time.RFC3339)},
		{PaymentID: "22222222-2222-2222-2222-222222222222", Status: "confirmed", Amount: 100, Currency: "BRL", Timestamp: time.Now().UTC().Format(time.RFC3339)},
	}

	results := svc.Publish(context.Background(), events, 0)

	require.Len(t, results, 2)
	assert.Error(t, results[0].Error)
	assert.NoError(t, results[1].Error)
}

func TestPublish_ContextCancel(t *testing.T) {
	mockProd := &mockSyncProducer{
		sendMessageFn: func(msg *sarama.ProducerMessage) (int32, int64, error) {
			time.Sleep(50 * time.Millisecond)
			return 0, 42, nil
		},
	}
	mockVal := &mockValidator{
		validateFn: func(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
			var e models.PaymentEvent
			return &e, nil
		},
	}

	svc := New(mockProd, "test-topic", mockVal)
	events := []*models.PaymentEvent{
		{PaymentID: "11111111-1111-1111-1111-111111111111", Status: "confirmed", Amount: 100, Currency: "BRL", Timestamp: time.Now().UTC().Format(time.RFC3339)},
		{PaymentID: "22222222-2222-2222-2222-222222222222", Status: "confirmed", Amount: 100, Currency: "BRL", Timestamp: time.Now().UTC().Format(time.RFC3339)},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := svc.Publish(ctx, events, 0)

	require.Len(t, results, 1)
	assert.ErrorIs(t, results[0].Error, context.Canceled)
}

func TestPublish_RateLimit(t *testing.T) {
	mockProd := &mockSyncProducer{
		sendMessageFn: func(msg *sarama.ProducerMessage) (int32, int64, error) {
			return 0, 42, nil
		},
	}
	mockVal := &mockValidator{
		validateFn: func(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
			var e models.PaymentEvent
			return &e, nil
		},
	}

	svc := New(mockProd, "test-topic", mockVal)
	events := make([]*models.PaymentEvent, 5)
	for i := range events {
		events[i] = &models.PaymentEvent{
			PaymentID: "550e8400-e29b-41d4-a716-446655440000",
			Status:    "confirmed",
			Amount:    100,
			Currency:  "BRL",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
	}

	start := time.Now()
	results := svc.Publish(context.Background(), events, 10)
	elapsed := time.Since(start)

	require.Len(t, results, 5)
	// 5 events at 10/s = ~400ms minimum (4 intervals of 100ms)
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(350))
}

func TestPublish_EmptyEvents(t *testing.T) {
	mockProd := &mockSyncProducer{}
	mockVal := &mockValidator{}

	svc := New(mockProd, "test-topic", mockVal)
	results := svc.Publish(context.Background(), []*models.PaymentEvent{}, 0)

	assert.Empty(t, results)
}

func TestPublish_RateLimitContextCancel(t *testing.T) {
	mockProd := &mockSyncProducer{
		sendMessageFn: func(msg *sarama.ProducerMessage) (int32, int64, error) {
			return 0, 42, nil
		},
	}
	mockVal := &mockValidator{
		validateFn: func(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
			var e models.PaymentEvent
			return &e, nil
		},
	}

	svc := New(mockProd, "test-topic", mockVal)
	events := make([]*models.PaymentEvent, 100)
	for i := range events {
		events[i] = &models.PaymentEvent{
			PaymentID: "550e8400-e29b-41d4-a716-446655440000",
			Status:    "confirmed",
			Amount:    100,
			Currency:  "BRL",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	results := svc.Publish(ctx, events, 10)
	require.Less(t, len(results), 100)
	assert.ErrorIs(t, results[len(results)-1].Error, context.Canceled)
}

func TestGenerateEvent_AllFields(t *testing.T) {
	event := GenerateEvent(
		"550e8400-e29b-41d4-a716-446655440000",
		"failed",
		250.50,
		"USD",
		"Test payment",
	)

	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", event.PaymentID)
	assert.Equal(t, "failed", event.Status)
	assert.Equal(t, 250.50, event.Amount)
	assert.Equal(t, "USD", event.Currency)
	assert.Equal(t, "Test payment", event.Description)
	assert.NotEmpty(t, event.Timestamp)
}

func TestGenerateEvent_Defaults(t *testing.T) {
	event := GenerateEvent("", "", 0, "", "")

	assert.NotEmpty(t, event.PaymentID)
	assert.Equal(t, "confirmed", event.Status)
	assert.Equal(t, 100.00, event.Amount)
	assert.Equal(t, "BRL", event.Currency)
	assert.Empty(t, event.Description)
	assert.NotEmpty(t, event.Timestamp)
}

func TestGenerateEvent_DefaultPaymentID(t *testing.T) {
	e1 := GenerateEvent("", "confirmed", 100, "BRL", "")
	e2 := GenerateEvent("", "confirmed", 100, "BRL", "")
	assert.NotEqual(t, e1.PaymentID, e2.PaymentID)
}

func TestGenerateEvent_Timestamp(t *testing.T) {
	event := GenerateEvent("", "confirmed", 100, "BRL", "")
	_, err := time.Parse(time.RFC3339, event.Timestamp)
	assert.NoError(t, err)
}

func TestGenerateBulkEvents_Count(t *testing.T) {
	events := GenerateBulkEvents(100)
	assert.Len(t, events, 100)
}

func TestGenerateBulkEvents_UniqueIDs(t *testing.T) {
	events := GenerateBulkEvents(50)
	ids := make(map[string]bool, 50)
	for _, e := range events {
		assert.False(t, ids[e.PaymentID], "duplicate payment_id: %s", e.PaymentID)
		ids[e.PaymentID] = true
	}
}

func TestGenerateBulkEvents_SequentialAmounts(t *testing.T) {
	events := GenerateBulkEvents(5)
	for i, e := range events {
		assert.Equal(t, float64(i+1)*10.0, e.Amount, "event %d amount mismatch", i)
	}
}

func TestGenerateBulkEvents_AllConfirmed(t *testing.T) {
	events := GenerateBulkEvents(10)
	for _, e := range events {
		assert.Equal(t, "confirmed", e.Status)
		assert.Equal(t, "BRL", e.Currency)
	}
}
