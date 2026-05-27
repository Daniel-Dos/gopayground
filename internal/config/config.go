package config

import (
	"log"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application.
type Config struct {
	Server struct {
		Port int `mapstructure:"port"`
	} `mapstructure:"server"`

	KafkaBrokers          string `mapstructure:"kafka_brokers"`
	KafkaTopic            string `mapstructure:"kafka_topic"`
	KafkaDLQTopic         string `mapstructure:"kafka_dlq_topic"`
	KafkaConsumerGroup    string `mapstructure:"kafka_consumer_group"`
	RedisAddr             string `mapstructure:"redis_addr"`
	RedisPassword         string `mapstructure:"redis_password"`
	DynamoDBEndpoint      string `mapstructure:"dynamodb_endpoint"`
	DynamoDBTable         string `mapstructure:"dynamodb_table"`
	WorkerCount           int    `mapstructure:"worker_count"`
	IdempotencyTTLHours   int    `mapstructure:"idempotency_ttl_hours"`
	StatusTTLHours        int    `mapstructure:"status_ttl_hours"`
	RetryMaxAttempts      int    `mapstructure:"retry_max_attempts"`
	RetryBaseDelayMs      int    `mapstructure:"retry_base_delay_ms"`
	OTelEndpoint          string `mapstructure:"otel_exporter_otlp_endpoint"`
	OTelServiceName       string `mapstructure:"otel_service_name"`
	GracefulShutdownTimeout time.Duration `mapstructure:"graceful_shutdown_timeout"`

	// UI-specific config
	UIPort            string        `mapstructure:"ui_port"`
	UIEventBusBuffer  int           `mapstructure:"ui_event_bus_buffer"`
	UIReadTimeout     time.Duration `mapstructure:"ui_read_timeout"`
	UIWriteTimeout    time.Duration `mapstructure:"ui_write_timeout"`
}

// NewConfig loads configuration from config file and environment variables.
func NewConfig() Config {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Set defaults
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("kafka_brokers", "localhost:9092")
	viper.SetDefault("kafka_topic", "payment.events")
	viper.SetDefault("kafka_dlq_topic", "payment.events.dlq")
	viper.SetDefault("kafka_consumer_group", "payment-consumer-group")
	viper.SetDefault("redis_addr", "localhost:6379")
	viper.SetDefault("redis_password", "")
	viper.SetDefault("dynamodb_endpoint", "http://localhost:4566")
	viper.SetDefault("dynamodb_table", "payment_history")
	viper.SetDefault("worker_count", 10)
	viper.SetDefault("idempotency_ttl_hours", 24)
	viper.SetDefault("status_ttl_hours", 168)
	viper.SetDefault("retry_max_attempts", 3)
	viper.SetDefault("retry_base_delay_ms", 100)
	viper.SetDefault("otel_exporter_otlp_endpoint", "localhost:4317")
	viper.SetDefault("otel_service_name", "payment-consumer")
	viper.SetDefault("graceful_shutdown_timeout", "30s")

	// UI-specific defaults
	viper.SetDefault("ui_port", "8081")
	viper.SetDefault("ui_event_bus_buffer", 256)
	viper.SetDefault("ui_read_timeout", "10s")
	viper.SetDefault("ui_write_timeout", "30s")

	// Try to read config file (optional)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			log.Printf("Warning: error reading config file: %s", err)
		}
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		log.Fatalf("Unable to decode config: %v", err)
	}

	// Parse duration if zero (viper may not decode string to Duration)
	if config.GracefulShutdownTimeout == 0 {
		d, err := time.ParseDuration(viper.GetString("graceful_shutdown_timeout"))
		if err == nil {
			config.GracefulShutdownTimeout = d
		}
	}

	if config.UIReadTimeout == 0 {
		d, err := time.ParseDuration(viper.GetString("ui_read_timeout"))
		if err == nil {
			config.UIReadTimeout = d
		}
	}

	if config.UIWriteTimeout == 0 {
		d, err := time.ParseDuration(viper.GetString("ui_write_timeout"))
		if err == nil {
			config.UIWriteTimeout = d
		}
	}

	return config
}
