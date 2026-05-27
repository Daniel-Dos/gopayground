package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/config"

	"github.com/stretchr/testify/assert"
)

func TestConfigDefaults(t *testing.T) {
	// Unset relevant env vars to test defaults
	for _, key := range []string{
		"KAFKA_BROKERS", "KAFKA_TOPIC", "KAFKA_DLQ_TOPIC", "KAFKA_CONSUMER_GROUP",
		"REDIS_ADDR", "REDIS_PASSWORD",
		"DYNAMODB_ENDPOINT", "DYNAMODB_TABLE",
		"WORKER_COUNT", "IDEMPOTENCY_TTL_HOURS", "STATUS_TTL_HOURS",
		"RETRY_MAX_ATTEMPTS", "RETRY_BASE_DELAY_MS",
		"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME",
		"GRACEFUL_SHUTDOWN_TIMEOUT",
		"UI_PORT", "UI_EVENT_BUS_BUFFER", "UI_READ_TIMEOUT", "UI_WRITE_TIMEOUT",
	} {
		os.Unsetenv(key)
	}

	cfg := config.NewConfig()

	assert.Equal(t, "localhost:9092", cfg.KafkaBrokers)
	assert.Equal(t, "payment.events", cfg.KafkaTopic)
	assert.Equal(t, "payment.events.dlq", cfg.KafkaDLQTopic)
	assert.Equal(t, "payment-consumer-group", cfg.KafkaConsumerGroup)
	assert.Equal(t, "localhost:6379", cfg.RedisAddr)
	assert.Equal(t, "", cfg.RedisPassword)
	assert.Equal(t, "http://localhost:4566", cfg.DynamoDBEndpoint)
	assert.Equal(t, "payment_history", cfg.DynamoDBTable)
	assert.Equal(t, 10, cfg.WorkerCount)
	assert.Equal(t, 24, cfg.IdempotencyTTLHours)
	assert.Equal(t, 168, cfg.StatusTTLHours)
	assert.Equal(t, 3, cfg.RetryMaxAttempts)
	assert.Equal(t, 100, cfg.RetryBaseDelayMs)
	assert.Equal(t, "localhost:4317", cfg.OTelEndpoint)
	assert.Equal(t, "payment-consumer", cfg.OTelServiceName)
	assert.Equal(t, 30*time.Second, cfg.GracefulShutdownTimeout)

	// UI defaults
	assert.Equal(t, "8081", cfg.UIPort)
	assert.Equal(t, 256, cfg.UIEventBusBuffer)
	assert.Equal(t, 10*time.Second, cfg.UIReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.UIWriteTimeout)
}

func TestConfigEnvOverrides(t *testing.T) {
	// Set env vars
	os.Setenv("KAFKA_BROKERS", "broker1:9092,broker2:9092")
	os.Setenv("KAFKA_TOPIC", "custom.events")
	os.Setenv("KAFKA_DLQ_TOPIC", "custom.dlq")
	os.Setenv("KAFKA_CONSUMER_GROUP", "custom-group")
	os.Setenv("REDIS_ADDR", "redis:6379")
	os.Setenv("REDIS_PASSWORD", "secret123")
	os.Setenv("DYNAMODB_ENDPOINT", "http://dynamo:8000")
	os.Setenv("DYNAMODB_TABLE", "custom_history")
	os.Setenv("WORKER_COUNT", "5")
	os.Setenv("IDEMPOTENCY_TTL_HOURS", "48")
	os.Setenv("STATUS_TTL_HOURS", "72")
	os.Setenv("RETRY_MAX_ATTEMPTS", "5")
	os.Setenv("RETRY_BASE_DELAY_MS", "200")
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "otel:4317")
	os.Setenv("OTEL_SERVICE_NAME", "custom-service")
	os.Setenv("GRACEFUL_SHUTDOWN_TIMEOUT", "10s")
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
		os.Unsetenv("IDEMPOTENCY_TTL_HOURS")
		os.Unsetenv("STATUS_TTL_HOURS")
		os.Unsetenv("RETRY_MAX_ATTEMPTS")
		os.Unsetenv("RETRY_BASE_DELAY_MS")
		os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		os.Unsetenv("OTEL_SERVICE_NAME")
		os.Unsetenv("GRACEFUL_SHUTDOWN_TIMEOUT")
		os.Unsetenv("UI_PORT")
		os.Unsetenv("UI_EVENT_BUS_BUFFER")
		os.Unsetenv("UI_READ_TIMEOUT")
		os.Unsetenv("UI_WRITE_TIMEOUT")
	}()

	cfg := config.NewConfig()

	assert.Equal(t, "broker1:9092,broker2:9092", cfg.KafkaBrokers)
	assert.Equal(t, "custom.events", cfg.KafkaTopic)
	assert.Equal(t, "custom.dlq", cfg.KafkaDLQTopic)
	assert.Equal(t, "custom-group", cfg.KafkaConsumerGroup)
	assert.Equal(t, "redis:6379", cfg.RedisAddr)
	assert.Equal(t, "secret123", cfg.RedisPassword)
	assert.Equal(t, "http://dynamo:8000", cfg.DynamoDBEndpoint)
	assert.Equal(t, "custom_history", cfg.DynamoDBTable)
	assert.Equal(t, 5, cfg.WorkerCount)
	assert.Equal(t, 48, cfg.IdempotencyTTLHours)
	assert.Equal(t, 72, cfg.StatusTTLHours)
	assert.Equal(t, 5, cfg.RetryMaxAttempts)
	assert.Equal(t, 200, cfg.RetryBaseDelayMs)
	assert.Equal(t, "otel:4317", cfg.OTelEndpoint)
	assert.Equal(t, "custom-service", cfg.OTelServiceName)
	assert.Equal(t, 10*time.Second, cfg.GracefulShutdownTimeout)

	// UI overrides
	assert.Equal(t, "9090", cfg.UIPort)
	assert.Equal(t, 512, cfg.UIEventBusBuffer)
	assert.Equal(t, 5*time.Second, cfg.UIReadTimeout)
	assert.Equal(t, 60*time.Second, cfg.UIWriteTimeout)
}

func TestConfigServerPortDefault(t *testing.T) {
	os.Unsetenv("SERVER_PORT")
	cfg := config.NewConfig()
	assert.Equal(t, 8080, cfg.Server.Port)
}
