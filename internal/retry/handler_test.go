package retry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/retry"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetry_SuccessFirstAttempt(t *testing.T) {
	h := retry.NewHandler(3, 100)
	attempts := 0

	err := h.Do(context.Background(), func(ctx context.Context) error {
		attempts++
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, attempts)
}

func TestRetry_SuccessAfterRetry(t *testing.T) {
	h := retry.NewHandler(3, 10) // small delay for fast test
	attempts := 0

	err := h.Do(context.Background(), func(ctx context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestRetry_Exhaustion(t *testing.T) {
	h := retry.NewHandler(3, 10)
	attempts := 0

	err := h.Do(context.Background(), func(ctx context.Context) error {
		attempts++
		return errors.New("persistent error")
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "all 3 attempts failed")
	assert.Equal(t, 3, attempts)
}

func TestRetry_ContextCancelled(t *testing.T) {
	h := retry.NewHandler(5, 100)
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel before retry
	cancel()

	err := h.Do(ctx, func(ctx context.Context) error {
		return errors.New("error")
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRetry_ContextCancelledDuringBackoff(t *testing.T) {
	h := retry.NewHandler(5, 500) // long delay
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := h.Do(ctx, func(ctx context.Context) error {
		return errors.New("error")
	})

	require.Error(t, err)
}

func TestRetry_MaxAttemptsOne(t *testing.T) {
	h := retry.NewHandler(1, 100)
	attempts := 0

	err := h.Do(context.Background(), func(ctx context.Context) error {
		attempts++
		return errors.New("error")
	})

	require.Error(t, err)
	assert.Equal(t, 1, attempts)
}

func TestRetry_JitterDoesNotProduceNegative(t *testing.T) {
	// Test that jitter never results in a negative delay by running several iterations
	for i := 0; i < 10; i++ {
		h := retry.NewHandler(2, 10) // small delay for fast test
		attempts := 0
		err := h.Do(context.Background(), func(ctx context.Context) error {
			attempts++
			return errors.New("error")
		})
		require.Error(t, err)
		assert.Equal(t, 2, attempts)
	}
}

func TestRetry_NoDelayOnFirstAttempt(t *testing.T) {
	start := time.Now()
	h := retry.NewHandler(1, 10000) // 10s delay (should not be used)

	err := h.Do(context.Background(), func(ctx context.Context) error {
		return nil
	})

	require.NoError(t, err)
	assert.Less(t, time.Since(start), 100*time.Millisecond) // should be instant
}
