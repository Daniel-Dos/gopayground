package ui_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/models"
	"github.com/Daniel-Dos/gopayground/internal/ui"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEventBus(t *testing.T) (*ui.EventBus, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	eb := ui.NewEventBus(client, "test:events", 64, logger)
	return eb, mr
}

func waitForEvent(t *testing.T, ch <-chan *models.PaymentEvent, timeout time.Duration) *models.PaymentEvent {
	t.Helper()
	select {
	case event := <-ch:
		return event
	case <-time.After(timeout):
		return nil
	}
}

func TestEventBus_PublishSubscribe(t *testing.T) {
	eb, _ := newTestEventBus(t)
	defer eb.Close()

	ch, unsubscribe := eb.Subscribe()
	defer unsubscribe()

	event := &models.PaymentEvent{
		PaymentID:   "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		Status:      "confirmed",
		Amount:      100.0,
		Currency:    "USD",
		Description: "test payment",
		Timestamp:   "2026-05-24T10:00:00Z",
	}

	// Give listenRedis goroutine time to subscribe
	time.Sleep(200 * time.Millisecond)

	err := eb.Publish(context.Background(), event)
	require.NoError(t, err)

	received := waitForEvent(t, ch, 3*time.Second)
	require.NotNil(t, received, "should receive event")
	assert.Equal(t, event.PaymentID, received.PaymentID)
	assert.Equal(t, event.Status, received.Status)
	assert.Equal(t, event.Amount, received.Amount)
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	eb, _ := newTestEventBus(t)
	defer eb.Close()

	ch1, unsub1 := eb.Subscribe()
	defer unsub1()
	ch2, unsub2 := eb.Subscribe()
	defer unsub2()

	// Give listenRedis goroutine time to subscribe
	time.Sleep(200 * time.Millisecond)

	event := &models.PaymentEvent{
		PaymentID: "multi-test-1",
		Status:    "pending",
		Amount:    50.0,
		Currency:  "BRL",
		Timestamp: "2026-05-24T10:00:00Z",
	}

	err := eb.Publish(context.Background(), event)
	require.NoError(t, err)

	received1 := waitForEvent(t, ch1, 3*time.Second)
	require.NotNil(t, received1, "subscriber 1 should receive event")
	assert.Equal(t, event.PaymentID, received1.PaymentID)

	received2 := waitForEvent(t, ch2, 3*time.Second)
	require.NotNil(t, received2, "subscriber 2 should receive event")
	assert.Equal(t, event.PaymentID, received2.PaymentID)
}

func TestEventBus_Unsubscribe(t *testing.T) {
	eb, _ := newTestEventBus(t)
	defer eb.Close()

	ch, unsubscribe := eb.Subscribe()

	// Give listenRedis goroutine time to subscribe
	time.Sleep(200 * time.Millisecond)

	// Unsubscribe before publishing
	unsubscribe()

	event := &models.PaymentEvent{
		PaymentID: "unsub-test",
		Status:    "failed",
		Amount:    10.0,
		Currency:  "EUR",
		Timestamp: "2026-05-24T10:00:00Z",
	}

	err := eb.Publish(context.Background(), event)
	require.NoError(t, err)

	// Channel should be closed
	_, ok := <-ch
	assert.False(t, ok, "channel should be closed after unsubscribe")
}

func TestEventBus_Close(t *testing.T) {
	eb, _ := newTestEventBus(t)

	ch, unsubscribe := eb.Subscribe()
	defer unsubscribe()

	time.Sleep(200 * time.Millisecond)

	eb.Close()

	// Channel should be closed after Close
	_, ok := <-ch
	assert.False(t, ok, "channel should be closed after EventBus.Close")
}

func TestEventBus_ConcurrentSubscribeUnsubscribe(t *testing.T) {
	eb, _ := newTestEventBus(t)
	defer eb.Close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, unsubscribe := eb.Subscribe()
			time.Sleep(time.Millisecond)
			unsubscribe()
		}()
	}

	// Publish concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			event := &models.PaymentEvent{
				PaymentID: "race-test",
				Status:    "confirmed",
				Amount:    float64(i),
				Currency:  "USD",
				Timestamp: "2026-05-24T10:00:00Z",
			}
			_ = eb.Publish(context.Background(), event)
		}(i)
	}

	wg.Wait()
	// No panic = test passes
}
