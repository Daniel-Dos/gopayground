package status_test

import (
	"context"
	"testing"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/status"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestUpdater(t *testing.T) (status.Updater, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() { client.Close() })

	updater := status.NewUpdater(client, 168) // 7 days
	return updater, mr
}

func TestUpdateStatus_CreatesKey(t *testing.T) {
	updater, mr := setupTestUpdater(t)
	paymentID := "test-payment-1"

	err := updater.UpdateStatus(context.Background(), paymentID, "confirmed")
	require.NoError(t, err)

	key := "payment:" + paymentID
	assert.True(t, mr.Exists(key))

	// Verify fields
	assert.Equal(t, paymentID, mr.HGet(key, "payment_id"))
	assert.Equal(t, "confirmed", mr.HGet(key, "status"))
	assert.NotEmpty(t, mr.HGet(key, "updated_at"))
}

func TestUpdateStatus_Overwrites(t *testing.T) {
	updater, mr := setupTestUpdater(t)
	paymentID := "test-payment-2"

	err := updater.UpdateStatus(context.Background(), paymentID, "pending")
	require.NoError(t, err)

	err = updater.UpdateStatus(context.Background(), paymentID, "confirmed")
	require.NoError(t, err)

	key := "payment:" + paymentID
	assert.Equal(t, "confirmed", mr.HGet(key, "status"))
}

func TestUpdateStatus_SetsTTL(t *testing.T) {
	updater, mr := setupTestUpdater(t)
	paymentID := "test-payment-3"

	err := updater.UpdateStatus(context.Background(), paymentID, "failed")
	require.NoError(t, err)

	key := "payment:" + paymentID
	ttl := mr.TTL(key)
	assert.Greater(t, ttl, 167*time.Hour)
	assert.LessOrEqual(t, ttl, 168*time.Hour)
}

func TestUpdateStatus_ContextCancelled(t *testing.T) {
	updater, _ := setupTestUpdater(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := updater.UpdateStatus(ctx, "test-payment-4", "refunded")
	assert.Error(t, err)
}

func TestUpdateStatus_RedisDown(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:16379", // wrong port
	})
	updater := status.NewUpdater(client, 168)

	err := updater.UpdateStatus(context.Background(), "test-payment-5", "pending")
	assert.Error(t, err)
}
