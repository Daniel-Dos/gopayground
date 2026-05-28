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
	"github.com/Daniel-Dos/gopayground/internal/kafka"
	"github.com/Daniel-Dos/gopayground/internal/provider"
	"github.com/Daniel-Dos/gopayground/internal/retry"
	"github.com/Daniel-Dos/gopayground/internal/status"
	"github.com/Daniel-Dos/gopayground/internal/validator"
	"github.com/Daniel-Dos/gopayground/pkg/telemetry"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
)

func main() {
	// 1. Carregar configurações
	cfg := config.NewConfig()

	// 2. Configurar logger estruturado
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	logger.Info("iniciando consumidor de pagamentos", "service", cfg.OTel.ServiceName)

	// 3. Inicializar OpenTelemetry
	ctx := context.Background()

	tp, err := telemetry.InitTracerProvider(ctx, cfg)
	if err != nil {
		logger.Error("falha ao inicializar tracer provider", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
		defer cancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			logger.Error("erro ao desligar tracer provider", "error", err)
		}
	}()

	mp, err := telemetry.InitMeterProvider(ctx, cfg)
	if err != nil {
		logger.Error("falha ao inicializar meter provider", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
		defer cancel()
		if err := mp.Shutdown(shutdownCtx); err != nil {
			logger.Error("erro ao desligar meter provider", "error", err)
		}
	}()

	meter := otel.Meter(cfg.OTel.ServiceName)
	tracer := otel.Tracer(cfg.OTel.ServiceName)

	// 4. Conectar ao Redis (cliente centralizado via provider)
	rdb := provider.NewRedisClient(provider.DefaultRedisConfig(cfg.Redis.Addr, cfg.Redis.Password))
	defer func() {
		if err := rdb.Close(); err != nil {
			logger.Error("erro ao fechar Redis", "error", err)
		}
	}()

	// Verifica conexão Redis (não bloqueante — aviso apenas)
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Warn("redis não disponível na inicialização, tentará novamente", "error", err)
	}

	// 5. Conectar ao DynamoDB (cliente centralizado via provider)
	dynamoClient, err := provider.NewDynamoDBClient(ctx, provider.DynamoDBConfig{
		Endpoint:  cfg.DynamoDB.Endpoint,
		Region:    cfg.DynamoDB.Region,
		AccessKey: cfg.DynamoDB.AccessKey,
		SecretKey: cfg.DynamoDB.SecretKey,
	})
	if err != nil {
		logger.Error("falha ao conectar no DynamoDB", "error", err)
		os.Exit(1)
	}

	// 5b. Garantir que a tabela DynamoDB existe (criação automática em dev/local)
	{
		tableCtx, tableCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer tableCancel()
		if err := history.EnsureTable(tableCtx, dynamoClient, cfg.DynamoDB.Table, logger); err != nil {
			logger.Error("falha ao garantir tabela DynamoDB", "error", err)
			os.Exit(1)
		}
	}

	// 6. Configurar Kafka
	consumerCfg := kafka.NewConsumerSaramaConfig(kafka.DefaultConsumerConfig())
	brokers := strings.Split(cfg.Kafka.Brokers, ",")

	consumerGroup, err := kafka.NewConsumerGroup(brokers, cfg.Kafka.ConsumerGroup, consumerCfg)
	if err != nil {
		logger.Error("failed to create consumer group", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := consumerGroup.Close(); err != nil {
			logger.Error("consumer group close error", "error", err)
		}
	}()

	producerCfg := kafka.NewProducerSaramaConfig(kafka.DefaultProducerConfig())
	syncProducer, err := kafka.NewSyncProducerWithRetry(ctx, brokers, producerCfg)
	if err != nil {
		logger.Error("failed to create sync producer", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := syncProducer.Close(); err != nil {
			logger.Error("sync producer close error", "error", err)
		}
	}()

	// 7. Instanciar componentes
	payloadValidator := validator.New()
	idempotencyChecker := idempotency.NewChecker(rdb, cfg.Worker.IdempotencyTTLHours)
	statusUpdater := status.NewUpdater(rdb, cfg.Worker.StatusTTLHours)
	historyRecorder := history.NewRecorder(dynamoClient, cfg.DynamoDB.Table)
	retryHandler := retry.NewHandler(cfg.Retry.MaxAttempts, cfg.Retry.BaseDelayMs)
	dlqProducer := dlq.NewProducer(syncProducer, cfg.Kafka.DLQTopic)
	eventPublisher := events.NewRedisPublisher(rdb, "payment:events")

	// 8. Criar handler do consumidor
	handler := consumer.NewHandler(
		payloadValidator,
		idempotencyChecker,
		statusUpdater,
		historyRecorder,
		retryHandler,
		dlqProducer,
		rdb,
		eventPublisher,
		cfg.Worker.Count,
		meter,
		tracer,
	)

	// 9. Configurar desligamento gracioso (SIGINT/SIGTERM)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 10. Servidor HTTP de health check (em goroutine separada)
	healthServer := startHealthServer(cfg.Server.Port, rdb, dynamoClient, cfg.DynamoDB.Table, logger)

	// 11. Loop principal do consumidor Kafka
	topics := []string{cfg.Kafka.Topic}
	consumeCtx, consumeCancel := context.WithCancel(context.Background())
	defer consumeCancel()

	go func() {
		select {
		case <-sigCh:
			logger.Info("sinal de desligamento recebido")
			consumeCancel()
		case <-consumeCtx.Done():
		}
	}()

	// Garantir que o servidor de health check seja desligado ao sair
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := healthServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("erro ao desligar servidor de health check", "error", err)
		}
		logger.Info("servidor de health check parado")
	}()

	logger.Info("iniciando loop do consumidor",
		"topics", topics,
		"group", cfg.Kafka.ConsumerGroup,
		"workers", cfg.Worker.Count,
	)

	for {
		select {
		case <-consumeCtx.Done():
			logger.Info("loop do consumidor finalizado")
			return
		default:
			if err := consumerGroup.Consume(consumeCtx, topics, handler); err != nil {
				logger.Error("erro no consumo", "error", err)
				// Pequena pausa antes de tentar novamente para evitar loop intenso
				time.Sleep(1 * time.Second)
			}
		}
	}
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
