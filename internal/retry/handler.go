package retry

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"
)

// Handler define a interface para lógica de retry (repetição com backoff).
type Handler interface {
	Do(ctx context.Context, fn func(context.Context) error) error
}

type retryHandler struct {
	maxAttempts int
	baseDelay   time.Duration
	jitter      float64
}

// NewHandler cria um novo handler de retry.
// Progressão: 1x, 3x, 9x o atraso base (não é shift binário).
// Com baseDelay=100ms: atrasos de 100ms, 300ms, 900ms para tentativas 2, 3, 4.
func NewHandler(maxAttempts int, baseDelayMs int) Handler {
	return &retryHandler{
		maxAttempts: maxAttempts,
		baseDelay:   time.Duration(baseDelayMs) * time.Millisecond,
		jitter:      0.25,
	}
}

func (rh *retryHandler) Do(ctx context.Context, fn func(context.Context) error) error {
	var lastErr error

	for attempt := 0; attempt < rh.maxAttempts; attempt++ {
		if attempt > 0 {
			delay := rh.baseDelay * time.Duration(math.Pow(3, float64(attempt-1)))
			delay = addJitter(delay, rh.jitter)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err
		slog.Warn("retry attempt failed",
			"attempt", attempt+1,
			"max_attempts", rh.maxAttempts,
			"error", err,
		)
	}

	return fmt.Errorf("all %d attempts failed: %w", rh.maxAttempts, lastErr)
}

func addJitter(d time.Duration, jitterPct float64) time.Duration {
	if d == 0 {
		return 0
	}
	jitter := time.Duration(float64(d) * jitterPct * (rand.Float64()*2 - 1))
	// Clamp to prevent negative delay (theoretical safeguard)
	result := d + jitter
	if result < 0 {
		return 0
	}
	return result
}
