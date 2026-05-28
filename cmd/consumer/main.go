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
	"github.com/Daniel-Dos/gopayground/internal/consumer"
	"github.com/Daniel-Dos/gopayground/internal/dlq"
	"github.com/Daniel-Dos/gopayground/internal/events"
	"github.com/Daniel-Dos/gopayground/internal/history"
	"github.com/Daniel-Dos/gopayground/internal/idempotency"
	"github.com/Daniel-Dos/gopayground/internal/retry"
	"github.com/Daniel-Dos/gopayground/internal/status"
	"github.com/Daniel-Dos/gopayground/internal/validator"
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

	// 2. Configure logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	logger.Info("starting payment consumer", "service", cfg.OTelServiceName)

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

	// Verify Redis connection
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Warn("redis not available at startup, will retry", "error", err)
	}

	// 5. Connect DynamoDB
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

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		logger.Error("failed to load AWS config", "error", err)
		os.Exit(1)
	}

	dynamoClient := dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
		if cfg.DynamoDBEndpoint != "" {
			o.BaseEndpoint = aws.String(cfg.DynamoDBEndpoint)
		}
	})

	// 5b. Ensure DynamoDB table exists (auto-create in dev/local environments)
	{
		tableCtx, tableCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer tableCancel()
		if err := history.EnsureTable(tableCtx, dynamoClient, cfg.DynamoDBTable, logger); err != nil {
			logger.Error("failed to ensure dynamodb table", "error", err)
			os.Exit(1)
		}
	}

	// 6. Configure Kafka
	kafkaCfg := sarama.NewConfig()
	kafkaCfg.Version = sarama.V2_6_0_0
	kafkaCfg.Consumer.Return.Errors = true
	kafkaCfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	kafkaCfg.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	kafkaCfg.Consumer.MaxProcessingTime = 30 * time.Second
	kafkaCfg.Producer.Return.Successes = true
	kafkaCfg.Producer.Return.Errors = true
	kafkaCfg.Producer.RequiredAcks = sarama.WaitForAll
	kafkaCfg.Producer.Idempotent = false
	kafkaCfg.Net.DialTimeout = 10 * time.Second
	kafkaCfg.Net.ReadTimeout = 30 * time.Second
	kafkaCfg.Net.WriteTimeout = 10 * time.Second

	brokers := strings.Split(cfg.KafkaBrokers, ",")

	consumerGroup, err := sarama.NewConsumerGroup(brokers, cfg.KafkaConsumerGroup, kafkaCfg)
	if err != nil {
		logger.Error("failed to create consumer group", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := consumerGroup.Close(); err != nil {
			logger.Error("consumer group close error", "error", err)
		}
	}()

	syncProducer, err := sarama.NewSyncProducer(brokers, kafkaCfg)
	if err != nil {
		logger.Error("failed to create sync producer", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := syncProducer.Close(); err != nil {
			logger.Error("sync producer close error", "error", err)
		}
	}()

	// 7. Instantiate components
	validator := validator.New()
	idempotencyChecker := idempotency.NewChecker(rdb, cfg.IdempotencyTTLHours)
	statusUpdater := status.NewUpdater(rdb, cfg.StatusTTLHours)
	historyRecorder := history.NewRecorder(dynamoClient, cfg.DynamoDBTable)
	retryHandler := retry.NewHandler(cfg.RetryMaxAttempts, cfg.RetryBaseDelayMs)
	dlqProducer := dlq.NewProducer(syncProducer, cfg.KafkaDLQTopic)
	eventPublisher := events.NewRedisPublisher(rdb, "payment:events")

	// 8. Create handler
	handler := consumer.NewHandler(
		validator,
		idempotencyChecker,
		statusUpdater,
		historyRecorder,
		retryHandler,
		dlqProducer,
		rdb,
		eventPublisher,
		cfg.WorkerCount,
		meter,
		tracer,
	)

	// 9. Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 10. HTTP health endpoint (in a goroutine)
	healthServer := startHealthServer(cfg.Server.Port, rdb, dynamoClient, cfg.DynamoDBTable, logger)

	// 11. Consumer loop
	topics := []string{cfg.KafkaTopic}
	consumeCtx, consumeCancel := context.WithCancel(context.Background())
	defer consumeCancel()

	go func() {
		select {
		case <-sigCh:
			logger.Info("shutdown signal received")
			consumeCancel()
		case <-consumeCtx.Done():
		}
	}()

	// Ensure health server is shut down when consumer exits
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := healthServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("health server shutdown error", "error", err)
		}
		logger.Info("health server stopped")
	}()

	logger.Info("starting consumer loop",
		"topics", topics,
		"group", cfg.KafkaConsumerGroup,
		"workers", cfg.WorkerCount,
	)

	for {
		select {
		case <-consumeCtx.Done():
			logger.Info("consumer loop exiting")
			return
		default:
			if err := consumerGroup.Consume(consumeCtx, topics, handler); err != nil {
				logger.Error("consume error", "error", err)
				// Wait a bit before retrying to avoid tight loop
				time.Sleep(1 * time.Second)
			}
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

func startHealthServer(port int, rdb *redis.Client, dynamoClient *dynamodb.Client, table string, logger *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check Redis
		if err := rdb.Ping(r.Context()).Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"status":"redis_down"}`)
			return
		}

		// Check DynamoDB
		if _, err := dynamoClient.DescribeTable(r.Context(), &dynamodb.DescribeTableInput{
			TableName: aws.String(table),
		}); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"status":"dynamodb_down"}`)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{
		Addr:        addr,
		Handler:     mux,
		ReadTimeout: 10 * time.Second,
	}

	logger.Info("starting health server", "addr", addr)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("health server error", "error", err)
		}
	}()

	return srv
}
