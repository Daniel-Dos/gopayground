package events

import (
	"context"

	"github.com/Daniel-Dos/gopayground/internal/models"
)

// Publisher publica eventos de pagamento para serem consumidos pela UI (SSE).
type Publisher interface {
	Publish(ctx context.Context, event *models.PaymentEvent) error
}
