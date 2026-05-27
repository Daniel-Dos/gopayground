package events

import (
	"context"

	"github.com/Daniel-Dos/gopayground/internal/models"
)

// Publisher publishes payment events to be consumed by the UI (SSE).
type Publisher interface {
	Publish(ctx context.Context, event *models.PaymentEvent) error
}
