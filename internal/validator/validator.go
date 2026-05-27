package validator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/models"

	goValidator "github.com/go-playground/validator/v10"
)

const maxPayloadSize = 10 * 1024 // 10 KB

// Validator defines the interface for payload validation.
type Validator interface {
	Validate(ctx context.Context, data []byte) (*models.PaymentEvent, error)
}

type paymentValidator struct {
	validate *goValidator.Validate
}

// New creates a new Validator with custom validation rules.
func New() Validator {
	v := goValidator.New()
	if err := v.RegisterValidation("rfc3339", validateRFC3339); err != nil {
		panic(fmt.Sprintf("failed to register rfc3339 validator: %v", err))
	}
	if err := v.RegisterValidation("printascii", validatePrintASCII); err != nil {
		panic(fmt.Sprintf("failed to register printascii validator: %v", err))
	}
	return &paymentValidator{validate: v}
}

func (pv *paymentValidator) Validate(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
	if len(data) > maxPayloadSize {
		return nil, fmt.Errorf("payload too large: %d bytes (max %d)", len(data), maxPayloadSize)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("empty payload")
	}

	var event models.PaymentEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	if err := pv.validate.StructCtx(ctx, event); err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}

	return &event, nil
}

// validateRFC3339 checks if the string is a valid RFC3339 timestamp,
// is not in the future (max 5 min skew allowed).
// validatePrintASCII checks that the string contains only printable ASCII characters
// (0x20-0x7E), rejecting control characters.
func validatePrintASCII(fl goValidator.FieldLevel) bool {
	s := fl.Field().String()
	for _, r := range s {
		if r < 0x20 || r > 0x7E {
			return false
		}
	}
	return true
}

func validateRFC3339(fl goValidator.FieldLevel) bool {
	ts := fl.Field().String()
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		// Try with nanoseconds
		t, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return false
		}
	}

	now := time.Now().UTC()
	maxSkew := 5 * time.Minute

	if t.After(now.Add(maxSkew)) {
		return false
	}

	return true
}
