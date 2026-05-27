package idempotency_test

import (
	"context"
	"testing"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/idempotency"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestChecker(t *testing.T) (idempotency.Checker, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() { client.Close() })

	checker := idempotency.NewChecker(client, 24)
	return checker, mr
}

func TestIsProcessed_KeyNotExists(t *testing.T) {
	checker, _ := setupTestChecker(t)
	processed, err := checker.IsProcessed(context.Background(), "nonexistent-id")
	require.NoError(t, err)
	assert.False(t, processed)
}

func TestMarkAndIsProcessed(t *testing.T) {
	checker, mr := setupTestChecker(t)
	paymentID := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	err := checker.MarkProcessed(context.Background(), paymentID)
	require.NoError(t, err)

	// Verify key exists in miniredis
	assert.True(t, mr.Exists("idempotency:"+paymentID))

	processed, err := checker.IsProcessed(context.Background(), paymentID)
	require.NoError(t, err)
	assert.True(t, processed)
}

func TestMarkProcessed_Duplicate(t *testing.T) {
	checker, _ := setupTestChecker(t)
	paymentID := "dup-payment-id"

	err := checker.MarkProcessed(context.Background(), paymentID)
	require.NoError(t, err)

	err = checker.MarkProcessed(context.Background(), paymentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestMarkProcessed_TTL(t *testing.T) {
	checker, mr := setupTestChecker(t)
	paymentID := "ttl-test-id"

	err := checker.MarkProcessed(context.Background(), paymentID)
	require.NoError(t, err)

	ttl := mr.TTL("idempotency:" + paymentID)
	assert.Greater(t, ttl, 23*time.Hour)
	assert.LessOrEqual(t, ttl, 24*time.Hour)
}

func TestContextCancelled(t *testing.T) {
	checker, _ := setupTestChecker(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := checker.IsProcessed(ctx, "test-id")
	assert.Error(t, err)
}

func TestIsProcessed_RedisDown(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:16379", // wrong port
	})
	checker := idempotency.NewChecker(client, 24)

	_, err := checker.IsProcessed(context.Background(), "test-id")
	assert.Error(t, err)
}
