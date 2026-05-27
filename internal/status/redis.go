package status

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Updater defines the interface for updating payment status.
type Updater interface {
	UpdateStatus(ctx context.Context, paymentID string, status string) error
}

type redisUpdater struct {
	client *redis.Client
	ttl    time.Duration
}

// NewUpdater creates a new Redis-based status updater.
func NewUpdater(client *redis.Client, ttlHours int) Updater {
	return &redisUpdater{
		client: client,
		ttl:    time.Duration(ttlHours) * time.Hour,
	}
}

func (ru *redisUpdater) UpdateStatus(ctx context.Context, paymentID string, status string) error {
	key := fmt.Sprintf("payment:%s", paymentID)
	pipe := ru.client.Pipeline()
	pipe.HSet(ctx, key, map[string]interface{}{
		"payment_id": paymentID,
		"status":     status,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	})
	pipe.Expire(ctx, key, ru.ttl)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("redis pipeline error: %w", err)
	}
	return nil
}
