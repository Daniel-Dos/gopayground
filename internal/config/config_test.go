package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/config"

	"github.com/stretchr/testify/assert"
)

func TestConfigDefaults(t *testing.T) {
	// Garante que nenhuma env var interfira — testa os valores do config.yaml.
	for _, key := range []string{
		"KAFKA_BROKERS", "KAFKA_TOPIC", "KAFKA_DLQ_TOPIC", "KAFKA_CONSUMER_GROUP",
		"REDIS_ADDR", "REDIS_PASSWORD",
		"DYNAMODB_ENDPOINT", "DYNAMODB_TABLE",
		"WORKER_COUNT", "WORKER_IDEMPOTENCY_TTL_HOURS", "WORKER_STATUS_TTL_HOURS",
		"RETRY_MAX_ATTEMPTS", "RETRY_BASE_DELAY_MS",
		"OTEL_ENDPOINT", "OTEL_SERVICE_NAME",
		"SERVER_GRACEFUL_SHUTDOWN_TIMEOUT",
		"UI_PORT", "UI_EVENT_BUS_BUFFER", "UI_READ_TIMEOUT", "UI_WRITE_TIMEOUT",
	} {
		os.Unsetenv(key)
	}

	cfg := config.NewConfig()

	// Kafka
	assert.Equal(t, "localhost:9092", cfg.Kafka.Brokers)
	assert.Equal(t, "payment.events", cfg.Kafka.Topic)
	assert.Equal(t, "payment.events.dlq", cfg.Kafka.DLQTopic)
	assert.Equal(t, "payment-consumer-group", cfg.Kafka.ConsumerGroup)

	// Redis
	assert.Equal(t, "localhost:6379", cfg.Redis.Addr)
	assert.Equal(t, "", cfg.Redis.Password)

	// DynamoDB
	assert.Equal(t, "http://localhost:4566", cfg.DynamoDB.Endpoint)
	assert.Equal(t, "payment_history", cfg.DynamoDB.Table)

	// Worker
	assert.Equal(t, 10, cfg.Worker.Count)
	assert.Equal(t, 24, cfg.Worker.IdempotencyTTLHours)
	assert.Equal(t, 168, cfg.Worker.StatusTTLHours)

	// Retry
	assert.Equal(t, 3, cfg.Retry.MaxAttempts)
	assert.Equal(t, 100, cfg.Retry.BaseDelayMs)

	// OTel
	assert.Equal(t, "localhost:4317", cfg.OTel.Endpoint)
	assert.Equal(t, "payment-consumer", cfg.OTel.ServiceName)

	// Server
	assert.Equal(t, 30*time.Second, cfg.Server.GracefulShutdownTimeout)

	// UI
	assert.Equal(t, "8081", cfg.UI.Port)
	assert.Equal(t, 256, cfg.UI.EventBusBuffer)
	assert.Equal(t, 10*time.Second, cfg.UI.ReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.UI.WriteTimeout)
}

func TestConfigEnvOverrides(t *testing.T) {
	// Configura env vars para sobrescrever os valores do config.yaml.
	os.Setenv("KAFKA_BROKERS", "broker1:9092,broker2:9092")
	os.Setenv("KAFKA_TOPIC", "custom.events")
	os.Setenv("KAFKA_DLQ_TOPIC", "custom.dlq")
	os.Setenv("KAFKA_CONSUMER_GROUP", "custom-group")
	os.Setenv("REDIS_ADDR", "redis:6379")
	os.Setenv("REDIS_PASSWORD", "secret123")
	os.Setenv("DYNAMODB_ENDPOINT", "http://dynamo:8000")
	os.Setenv("DYNAMODB_TABLE", "custom_history")
	os.Setenv("WORKER_COUNT", "5")
	os.Setenv("WORKER_IDEMPOTENCY_TTL_HOURS", "48")
	os.Setenv("WORKER_STATUS_TTL_HOURS", "72")
	os.Setenv("RETRY_MAX_ATTEMPTS", "5")
	os.Setenv("RETRY_BASE_DELAY_MS", "200")
	os.Setenv("OTEL_ENDPOINT", "otel:4317")
	os.Setenv("OTEL_SERVICE_NAME", "custom-service")
	os.Setenv("SERVER_GRACEFUL_SHUTDOWN_TIMEOUT", "10s")
	os.Setenv("UI_PORT", "9090")
	os.Setenv("UI_EVENT_BUS_BUFFER", "512")
	os.Setenv("UI_READ_TIMEOUT", "5s")
	os.Setenv("UI_WRITE_TIMEOUT", "60s")

	defer func() {
		os.Unsetenv("KAFKA_BROKERS")
		os.Unsetenv("KAFKA_TOPIC")
		os.Unsetenv("KAFKA_DLQ_TOPIC")
		os.Unsetenv("KAFKA_CONSUMER_GROUP")
		os.Unsetenv("REDIS_ADDR")
		os.Unsetenv("REDIS_PASSWORD")
		os.Unsetenv("DYNAMODB_ENDPOINT")
		os.Unsetenv("DYNAMODB_TABLE")
		os.Unsetenv("WORKER_COUNT")
		os.Unsetenv("WORKER_IDEMPOTENCY_TTL_HOURS")
		os.Unsetenv("WORKER_STATUS_TTL_HOURS")
		os.Unsetenv("RETRY_MAX_ATTEMPTS")
		os.Unsetenv("RETRY_BASE_DELAY_MS")
		os.Unsetenv("OTEL_ENDPOINT")
		os.Unsetenv("OTEL_SERVICE_NAME")
		os.Unsetenv("SERVER_GRACEFUL_SHUTDOWN_TIMEOUT")
		os.Unsetenv("UI_PORT")
		os.Unsetenv("UI_EVENT_BUS_BUFFER")
		os.Unsetenv("UI_READ_TIMEOUT")
		os.Unsetenv("UI_WRITE_TIMEOUT")
	}()

	cfg := config.NewConfig()

	assert.Equal(t, "broker1:9092,broker2:9092", cfg.Kafka.Brokers)
	assert.Equal(t, "custom.events", cfg.Kafka.Topic)
	assert.Equal(t, "custom.dlq", cfg.Kafka.DLQTopic)
	assert.Equal(t, "custom-group", cfg.Kafka.ConsumerGroup)
	assert.Equal(t, "redis:6379", cfg.Redis.Addr)
	assert.Equal(t, "secret123", cfg.Redis.Password)
	assert.Equal(t, "http://dynamo:8000", cfg.DynamoDB.Endpoint)
	assert.Equal(t, "custom_history", cfg.DynamoDB.Table)
	assert.Equal(t, 5, cfg.Worker.Count)
	assert.Equal(t, 48, cfg.Worker.IdempotencyTTLHours)
	assert.Equal(t, 72, cfg.Worker.StatusTTLHours)
	assert.Equal(t, 5, cfg.Retry.MaxAttempts)
	assert.Equal(t, 200, cfg.Retry.BaseDelayMs)
	assert.Equal(t, "otel:4317", cfg.OTel.Endpoint)
	assert.Equal(t, "custom-service", cfg.OTel.ServiceName)
	assert.Equal(t, 10*time.Second, cfg.Server.GracefulShutdownTimeout)

	// UI overrides
	assert.Equal(t, "9090", cfg.UI.Port)
	assert.Equal(t, 512, cfg.UI.EventBusBuffer)
	assert.Equal(t, 5*time.Second, cfg.UI.ReadTimeout)
	assert.Equal(t, 60*time.Second, cfg.UI.WriteTimeout)
}

func TestConfigServerPortDefault(t *testing.T) {
	os.Unsetenv("SERVER_PORT")
	cfg := config.NewConfig()
	assert.Equal(t, 8080, cfg.Server.Port)
}
