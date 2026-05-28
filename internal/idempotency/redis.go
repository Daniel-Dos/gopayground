package idempotency

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Checker define a interface para verificação de idempotência.
type Checker interface {
	IsProcessed(ctx context.Context, paymentID string) (bool, error)
	MarkProcessed(ctx context.Context, paymentID string) error
}

type redisChecker struct {
	client *redis.Client
	ttl    time.Duration
}

// NewChecker cria um novo verificador de idempotência baseado em Redis.
func NewChecker(client *redis.Client, ttlHours int) Checker {
	return &redisChecker{
		client: client,
		ttl:    time.Duration(ttlHours) * time.Hour,
	}
}

func (rc *redisChecker) IsProcessed(ctx context.Context, paymentID string) (bool, error) {
	key := fmt.Sprintf("idempotency:%s", paymentID)
	exists, err := rc.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists error: %w", err)
	}
	return exists == 1, nil
}

func (rc *redisChecker) MarkProcessed(ctx context.Context, paymentID string) error {
	key := fmt.Sprintf("idempotency:%s", paymentID)
	ok, err := rc.client.SetNX(ctx, key, "1", rc.ttl).Result()
	if err != nil {
		return fmt.Errorf("redis setnx error: %w", err)
	}
	if !ok {
		return fmt.Errorf("idempotency key already exists: %s", paymentID)
	}
	return nil
}
