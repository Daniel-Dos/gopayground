package validator_test

import (
	"context"
	"testing"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/validator"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validPayload() []byte {
	return []byte(`{
		"payment_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"status": "confirmed",
		"amount": 150.50,
		"currency": "USD",
		"description": "Test payment",
		"timestamp": "2026-05-24T10:00:00Z"
	}`)
}

func TestValidatorValidPayload(t *testing.T) {
	v := validator.New()
	event, err := v.Validate(context.Background(), validPayload())
	require.NoError(t, err)
	require.NotNil(t, event)
	assert.Equal(t, "f47ac10b-58cc-4372-a567-0e02b2c3d479", event.PaymentID)
	assert.Equal(t, "confirmed", event.Status)
	assert.Equal(t, 150.50, event.Amount)
	assert.Equal(t, "USD", event.Currency)
	assert.Equal(t, "Test payment", event.Description)
	assert.Equal(t, "2026-05-24T10:00:00Z", event.Timestamp)
}

func TestValidatorInvalidUUID(t *testing.T) {
	v := validator.New()
	payload := []byte(`{
		"payment_id": "not-a-uuid",
		"status": "confirmed",
		"amount": 100,
		"currency": "USD",
		"timestamp": "2026-05-24T10:00:00Z"
	}`)
	_, err := v.Validate(context.Background(), payload)
	assert.Error(t, err)
}

func TestValidatorInvalidStatus(t *testing.T) {
	v := validator.New()
	payload := []byte(`{
		"payment_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"status": "invalid_status",
		"amount": 100,
		"currency": "USD",
		"timestamp": "2026-05-24T10:00:00Z"
	}`)
	_, err := v.Validate(context.Background(), payload)
	assert.Error(t, err)
}

func TestValidatorZeroAmount(t *testing.T) {
	v := validator.New()
	payload := []byte(`{
		"payment_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"status": "pending",
		"amount": 0,
		"currency": "USD",
		"timestamp": "2026-05-24T10:00:00Z"
	}`)
	_, err := v.Validate(context.Background(), payload)
	assert.Error(t, err)
}

func TestValidatorNegativeAmount(t *testing.T) {
	v := validator.New()
	payload := []byte(`{
		"payment_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"status": "pending",
		"amount": -10,
		"currency": "USD",
		"timestamp": "2026-05-24T10:00:00Z"
	}`)
	_, err := v.Validate(context.Background(), payload)
	assert.Error(t, err)
}

func TestValidatorShortCurrency(t *testing.T) {
	v := validator.New()
	payload := []byte(`{
		"payment_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"status": "pending",
		"amount": 100,
		"currency": "US",
		"timestamp": "2026-05-24T10:00:00Z"
	}`)
	_, err := v.Validate(context.Background(), payload)
	assert.Error(t, err)
}

func TestValidatorLowercaseCurrency(t *testing.T) {
	v := validator.New()
	payload := []byte(`{
		"payment_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"status": "pending",
		"amount": 100,
		"currency": "brl",
		"timestamp": "2026-05-24T10:00:00Z"
	}`)
	_, err := v.Validate(context.Background(), payload)
	assert.Error(t, err)
}

func TestValidatorInvalidTimestamp(t *testing.T) {
	v := validator.New()
	payload := []byte(`{
		"payment_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"status": "pending",
		"amount": 100,
		"currency": "USD",
		"timestamp": "not-a-timestamp"
	}`)
	_, err := v.Validate(context.Background(), payload)
	assert.Error(t, err)
}

func TestValidatorFutureTimestamp(t *testing.T) {
	v := validator.New()
	future := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
	payload := []byte(`{
		"payment_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"status": "pending",
		"amount": 100,
		"currency": "USD",
		"timestamp": "` + future + `"
	}`)
	_, err := v.Validate(context.Background(), payload)
	assert.Error(t, err)
}

func TestValidatorDescriptionTooLong(t *testing.T) {
	v := validator.New()
	desc := make([]byte, 256)
	for i := range desc {
		desc[i] = 'a'
	}
	payload := []byte(`{
		"payment_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"status": "pending",
		"amount": 100,
		"currency": "USD",
		"description": "` + string(desc) + `",
		"timestamp": "2026-05-24T10:00:00Z"
	}`)
	_, err := v.Validate(context.Background(), payload)
	assert.Error(t, err)
}

func TestValidatorControlCharsInDescription(t *testing.T) {
	v := validator.New()
	payload := []byte(`{
		"payment_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"status": "pending",
		"amount": 100,
		"currency": "USD",
		"description": "line1\nline2",
		"timestamp": "2026-05-24T10:00:00Z"
	}`)
	_, err := v.Validate(context.Background(), payload)
	assert.Error(t, err)
}

func TestValidatorExtraFields(t *testing.T) {
	v := validator.New()
	payload := []byte(`{
		"payment_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"status": "confirmed",
		"amount": 100,
		"currency": "USD",
		"timestamp": "2026-05-24T10:00:00Z",
		"extra": "ignored"
	}`)
	event, err := v.Validate(context.Background(), payload)
	require.NoError(t, err)
	require.NotNil(t, event)
}

func TestValidatorMalformedJSON(t *testing.T) {
	v := validator.New()
	payload := []byte(`{"payment_id": broken}`)
	_, err := v.Validate(context.Background(), payload)
	assert.Error(t, err)
}

func TestValidatorEmptyPayload(t *testing.T) {
	v := validator.New()
	_, err := v.Validate(context.Background(), []byte(`{}`))
	assert.Error(t, err)
}

func TestValidatorPayloadTooLarge(t *testing.T) {
	v := validator.New()
	payload := make([]byte, 11*1024)
	for i := range payload {
		payload[i] = ' '
	}
	_, err := v.Validate(context.Background(), payload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "payload too large")
}

func TestValidatorEmptyBytes(t *testing.T) {
	v := validator.New()
	_, err := v.Validate(context.Background(), []byte{})
	assert.Error(t, err)
}

func TestValidatorAllStatuses(t *testing.T) {
	v := validator.New()
	statuses := []string{"pending", "confirmed", "failed", "refunded"}
	for _, s := range statuses {
		payload := []byte(`{
			"payment_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"status": "` + s + `",
			"amount": 100,
			"currency": "USD",
			"timestamp": "2026-05-24T10:00:00Z"
		}`)
		event, err := v.Validate(context.Background(), payload)
		require.NoError(t, err, "status: %s", s)
		require.NotNil(t, event)
	}
}

func TestValidatorTimestampWithNanos(t *testing.T) {
	v := validator.New()
	payload := []byte(`{
		"payment_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"status": "pending",
		"amount": 100,
		"currency": "USD",
		"timestamp": "2026-05-24T10:00:00.123456Z"
	}`)
	event, err := v.Validate(context.Background(), payload)
	require.NoError(t, err)
	require.NotNil(t, event)
}
