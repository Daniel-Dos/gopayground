package main

import (
	"os"
	"testing"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigLoading(t *testing.T) {
	os.Setenv("KAFKA_BROKERS", "localhost:9092")
	os.Setenv("REDIS_ADDR", "localhost:6379")
	os.Setenv("DYNAMODB_ENDPOINT", "http://localhost:8000")
	os.Setenv("OTEL_ENDPOINT", "localhost:0")
	defer func() {
		os.Unsetenv("KAFKA_BROKERS")
		os.Unsetenv("REDIS_ADDR")
		os.Unsetenv("DYNAMODB_ENDPOINT")
		os.Unsetenv("OTEL_ENDPOINT")
	}()

	cfg := config.NewConfig()
	assert.Equal(t, "localhost:9092", cfg.Kafka.Brokers)
	assert.Equal(t, "payment.events", cfg.Kafka.Topic)
	assert.Equal(t, 10, cfg.Worker.Count)
	assert.Equal(t, 30*time.Second, cfg.Server.GracefulShutdownTimeout)
}

func TestHealthEndpoint(t *testing.T) {
	cfg := config.NewConfig()
	cfg.Server.Port = 0

	require.NotEmpty(t, cfg.Kafka.Brokers)
	require.NotEmpty(t, cfg.Redis.Addr)
}
