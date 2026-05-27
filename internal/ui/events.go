package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Daniel-Dos/gopayground/internal/models"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// EventBus distribui eventos do consumer para a UI via Redis Pub/Sub.
type EventBus struct {
	redis       *redis.Client
	channel     string
	subscribers map[string]chan *models.PaymentEvent
	mu          sync.RWMutex
	logger      *slog.Logger
	bufSize     int
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewEventBus creates a new EventBus.
func NewEventBus(rdb *redis.Client, channel string, bufSize int, logger *slog.Logger) *EventBus {
	ctx, cancel := context.WithCancel(context.Background())
	eb := &EventBus{
		redis:       rdb,
		channel:     channel,
		subscribers: make(map[string]chan *models.PaymentEvent),
		logger:      logger,
		bufSize:     bufSize,
		ctx:         ctx,
		cancel:      cancel,
	}
	go eb.listenRedis()
	return eb
}

// Publish publica um evento no canal Redis.
func (eb *EventBus) Publish(ctx context.Context, event *models.PaymentEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	return eb.redis.Publish(ctx, eb.channel, string(data)).Err()
}

// Subscribe registra um subscriber e retorna um canal de eventos e uma função de unsubscribe.
func (eb *EventBus) Subscribe() (<-chan *models.PaymentEvent, func()) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	id := uuid.New().String()
	ch := make(chan *models.PaymentEvent, eb.bufSize)
	eb.subscribers[id] = ch

	unsubscribe := func() {
		eb.mu.Lock()
		if ch, ok := eb.subscribers[id]; ok {
			close(ch)
			delete(eb.subscribers, id)
		}
		eb.mu.Unlock()
	}

	return ch, unsubscribe
}

// listenRedis escuta o canal Redis e distribui para subscribers locais.
func (eb *EventBus) listenRedis() {
	pubsub := eb.redis.Subscribe(eb.ctx, eb.channel)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-eb.ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var event models.PaymentEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				eb.logger.Error("failed to unmarshal event from Redis", "error", err)
				continue
			}

			eb.mu.RLock()
			for id, sub := range eb.subscribers {
				select {
				case sub <- &event:
				default:
					// Subscriber lento, descarta evento
					eb.logger.Warn("dropping event for slow subscriber", "subscriber_id", id)
				}
			}
			eb.mu.RUnlock()
		}
	}
}

// Close finaliza o EventBus, limpando todos os subscribers.
func (eb *EventBus) Close() {
	eb.cancel()
	eb.mu.Lock()
	defer eb.mu.Unlock()
	for id, ch := range eb.subscribers {
		close(ch)
		delete(eb.subscribers, id)
	}
}
