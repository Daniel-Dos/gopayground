package models_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaymentEventJSONRoundTrip(t *testing.T) {
	event := models.PaymentEvent{
		PaymentID:   "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		Status:      "confirmed",
		Amount:      150.50,
		Currency:    "USD",
		Description: "Test payment",
		Timestamp:   "2026-05-24T10:00:00Z",
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded models.PaymentEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, event.PaymentID, decoded.PaymentID)
	assert.Equal(t, event.Status, decoded.Status)
	assert.Equal(t, event.Amount, decoded.Amount)
	assert.Equal(t, event.Currency, decoded.Currency)
	assert.Equal(t, event.Description, decoded.Description)
	assert.Equal(t, event.Timestamp, decoded.Timestamp)
}

func TestPaymentEventJSONFields(t *testing.T) {
	raw := `{
		"payment_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"status": "confirmed",
		"amount": 99.99,
		"currency": "BRL",
		"description": "hello",
		"timestamp": "2026-05-24T10:00:00Z"
	}`

	var event models.PaymentEvent
	err := json.Unmarshal([]byte(raw), &event)
	require.NoError(t, err)
	assert.Equal(t, "f47ac10b-58cc-4372-a567-0e02b2c3d479", event.PaymentID)
	assert.Equal(t, "confirmed", event.Status)
	assert.Equal(t, 99.99, event.Amount)
	assert.Equal(t, "BRL", event.Currency)
	assert.Equal(t, "hello", event.Description)
	assert.Equal(t, "2026-05-24T10:00:00Z", event.Timestamp)
}

func TestPaymentEventExtraFieldsIgnored(t *testing.T) {
	raw := `{
		"payment_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"status": "pending",
		"amount": 10.0,
		"currency": "EUR",
		"timestamp": "2026-05-24T10:00:00Z",
		"extra_field": "should be ignored"
	}`

	var event models.PaymentEvent
	err := json.Unmarshal([]byte(raw), &event)
	require.NoError(t, err)
	assert.Equal(t, "f47ac10b-58cc-4372-a567-0e02b2c3d479", event.PaymentID)
}

func TestPaymentHistoryFromEvent(t *testing.T) {
	event := &models.PaymentEvent{
		PaymentID:   "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		Status:      "confirmed",
		Amount:      250.00,
		Currency:    "USD",
		Description: "Purchase",
		Timestamp:   "2026-05-24T10:00:00Z",
	}

	traceID := "abc123trace"
	history := models.NewPaymentHistoryFromEvent(event, traceID)

	assert.Equal(t, event.PaymentID, history.PaymentID)
	assert.Equal(t, event.Status, history.Status)
	assert.Equal(t, event.Amount, history.Amount)
	assert.Equal(t, event.Currency, history.Currency)
	assert.Equal(t, event.Description, history.Description)
	assert.Equal(t, event.Timestamp, history.Timestamp)
	assert.Equal(t, traceID, history.TraceID)
	assert.False(t, history.ProcessedAt.IsZero())
	assert.Equal(t, time.UTC, history.ProcessedAt.Location())
}

func TestPaymentHistoryFromEventEmptyTraceID(t *testing.T) {
	event := &models.PaymentEvent{
		PaymentID: "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		Status:    "pending",
		Amount:    1.0,
		Currency:  "USD",
		Timestamp: "2026-05-24T10:00:00Z",
	}

	history := models.NewPaymentHistoryFromEvent(event, "")
	assert.Equal(t, "", history.TraceID)
	assert.NotNil(t, history)
}

func TestPaymentEventJSONTags(t *testing.T) {
	event := models.PaymentEvent{
		PaymentID: "test-uuid",
		Status:    "failed",
		Amount:    0.01,
		Currency:  "EUR",
		Timestamp: "2026-05-24T10:00:00Z",
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)
	jsonStr := string(data)

	assert.True(t, strings.Contains(jsonStr, "payment_id"))
	assert.True(t, strings.Contains(jsonStr, "amount"))
	assert.True(t, strings.Contains(jsonStr, "currency"))
	assert.True(t, strings.Contains(jsonStr, "timestamp"))
}

func TestPaymentStatusJSON(t *testing.T) {
	status := models.PaymentStatus{
		PaymentID: "pid-123",
		Status:    "confirmed",
		UpdatedAt: "2026-05-24T10:00:00Z",
	}

	data, err := json.Marshal(status)
	require.NoError(t, err)

	var decoded models.PaymentStatus
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, status.PaymentID, decoded.PaymentID)
	assert.Equal(t, status.Status, decoded.Status)
	assert.Equal(t, status.UpdatedAt, decoded.UpdatedAt)
}
