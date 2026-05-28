package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/config"
	"github.com/Daniel-Dos/gopayground/internal/history"
	"github.com/Daniel-Dos/gopayground/internal/ui"
	"github.com/Daniel-Dos/gopayground/pkg/telemetry"

	"github.com/IBM/sarama"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
)

func main() {
	// 1. Load config
	cfg := config.NewConfig()

	// 2. Logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout,
		&slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	logger.Info("starting payment UI server")

	// 3. Initialize OTel
	ctx := context.Background()

	tp, err := telemetry.InitTracerProvider(ctx, cfg)
	if err != nil {
		logger.Error("failed to initialize tracer provider", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.GracefulShutdownTimeout)
		defer cancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			logger.Error("tracer provider shutdown error", "error", err)
		}
	}()

	mp, err := telemetry.InitMeterProvider(ctx, cfg)
	if err != nil {
		logger.Error("failed to initialize meter provider", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.GracefulShutdownTimeout)
		defer cancel()
		if err := mp.Shutdown(shutdownCtx); err != nil {
			logger.Error("meter provider shutdown error", "error", err)
		}
	}()

	meter := otel.Meter(cfg.OTelServiceName)
	tracer := otel.Tracer(cfg.OTelServiceName)
	_ = tracer // reserved for future HTTP span creation

	// 4. Connect Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	defer func() {
		if err := rdb.Close(); err != nil {
			logger.Error("redis close error", "error", err)
		}
	}()

	// 4. Connect DynamoDB
	var loadOpts []func(*awsconfig.LoadOptions) error

	// When using local DynamoDB (Floci/LocalStack), use static credentials
	// to avoid EC2 IMDS lookup failures in non-EC2 environments.
	if isLocalEndpoint(cfg.DynamoDBEndpoint) {
		logger.Info("using static AWS credentials for local DynamoDB endpoint",
			"endpoint", cfg.DynamoDBEndpoint)
		loadOpts = append(loadOpts,
			awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider("test", "test", ""),
			),
			awsconfig.WithRegion("us-east-1"),
		)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		logger.Error("failed to load AWS config", "error", err)
		os.Exit(1)
	}

	dynamoClient := dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
		if cfg.DynamoDBEndpoint != "" {
			o.BaseEndpoint = aws.String(cfg.DynamoDBEndpoint)
		}
	})

	// 4b. Ensure DynamoDB table exists (auto-create in dev/local environments)
	{
		tableCtx, tableCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer tableCancel()
		if err := history.EnsureTable(tableCtx, dynamoClient, cfg.DynamoDBTable, logger); err != nil {
			logger.Error("failed to ensure dynamodb table", "error", err)
			os.Exit(1)
		}
	}

	// 5. Create Kafka sync producer (optional — UI works without it)
	kafkaProducer := createKafkaProducer(cfg, logger)

	// 6. Create UI server
	server := ui.NewServer(cfg, rdb, dynamoClient, kafkaProducer, logger, meter)

	// 6. Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("shutdown signal received", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Error("shutdown error", "error", err)
		}
		if kafkaProducer != nil {
			if err := kafkaProducer.Close(); err != nil {
				logger.Error("kafka producer close error", "error", err)
			}
			logger.Info("kafka producer closed")
		}
	}()

	// 7. Start server
	logger.Info("UI server ready", "port", cfg.UIPort)
	if err := server.Start(); err != nil && err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}

	logger.Info("UI server stopped")
}

// createKafkaProducer creates a sync Kafka producer with retry.
// Retries with exponential backoff for up to ~30 seconds.
// Returns nil only if all retries are exhausted (UI still works without it).
func createKafkaProducer(cfg config.Config, logger *slog.Logger) sarama.SyncProducer {
	kafkaBrokers := strings.Split(cfg.KafkaBrokers, ",")

	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Producer.Return.Successes = true
	kafkaConfig.Producer.Timeout = 10 * time.Second
	kafkaConfig.Producer.RequiredAcks = sarama.WaitForLocal
	kafkaConfig.Net.DialTimeout = 5 * time.Second
	kafkaConfig.Net.WriteTimeout = 5 * time.Second
	kafkaConfig.Producer.MaxMessageBytes = 100 * 1024

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	producer, err := connectProducerWithRetry(ctx, kafkaBrokers, kafkaConfig)
	if err != nil {
		logger.Warn("kafka not available after retries, producer UI will be disabled", "error", err)
		return nil
	}

	logger.Info("kafka producer connected", "brokers", cfg.KafkaBrokers, "topic", cfg.KafkaTopic)
	return producer
}

// connectProducerWithRetry tries to create a Sarama SyncProducer with
// exponential backoff, respecting context cancellation.
// Sequence: 500ms, 1s, 2s, 4s, 8s, 8s, ... (capped at 8s)
// Total wall-clock timeout: ~30 seconds.
func connectProducerWithRetry(ctx context.Context, brokers []string, config *sarama.Config) (sarama.SyncProducer, error) {
	backoff := 500 * time.Millisecond
	const maxBackoff = 8 * time.Second

	for {
		producer, err := sarama.NewSyncProducer(brokers, config)
		if err == nil {
			return producer, nil
		}

		// Prefer context cancellation over deadline checks.
		if ctx.Err() != nil {
			return nil, fmt.Errorf("canceled while waiting for Kafka: %w", ctx.Err())
		}

		slog.Warn("kafka not ready, retrying...", "backoff", backoff.String(), "error", err)

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("canceled while waiting for Kafka: %w", ctx.Err())
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// isLocalEndpoint checks whether the given DynamoDB endpoint points to a
// local/test container (Floci, LocalStack, localhost) that accepts any
// static credentials, avoiding the need for EC2 IMDS or real AWS credentials.
func isLocalEndpoint(endpoint string) bool {
	lower := strings.ToLower(endpoint)
	return strings.Contains(lower, "localhost") ||
		strings.Contains(lower, "127.0.0.1") ||
		strings.Contains(lower, "floci") ||
		strings.Contains(lower, "localstack")
}
