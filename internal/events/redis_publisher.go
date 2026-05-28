package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Daniel-Dos/gopayground/internal/models"

	"github.com/redis/go-redis/v9"
)

// RedisPublisher publica eventos em um canal Redis Pub/Sub.
type RedisPublisher struct {
	client  *redis.Client
	channel string
}

// NewRedisPublisher cria um novo RedisPublisher.
func NewRedisPublisher(client *redis.Client, channel string) *RedisPublisher {
	return &RedisPublisher{client: client, channel: channel}
}

// Publish serializa o evento e publica no canal Redis configurado.
func (p *RedisPublisher) Publish(ctx context.Context, event *models.PaymentEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	return p.client.Publish(ctx, p.channel, string(data)).Err()
}
