package ui_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// discardWriter discards all log output.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

func setupMiniredis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	return client, mr
}

func setupRedisPayment(t *testing.T, mr *miniredis.Miniredis, paymentID, status string) {
	t.Helper()
	key := "payment:" + paymentID
	mr.HSet(key, "payment_id", paymentID)
	mr.HSet(key, "status", status)
	mr.HSet(key, "updated_at", time.Now().UTC().Format(time.RFC3339))
}
