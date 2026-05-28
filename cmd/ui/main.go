package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/config"
	"github.com/Daniel-Dos/gopayground/internal/history"
	"github.com/Daniel-Dos/gopayground/internal/provider"
	"github.com/Daniel-Dos/gopayground/internal/ui"
	"github.com/Daniel-Dos/gopayground/pkg/telemetry"

	"go.opentelemetry.io/otel"
)

func main() {
	// 1. Carregar configurações
	cfg := config.NewConfig()

	// 2. Configurar logger estruturado
	logger := slog.New(slog.NewJSONHandler(os.Stdout,
		&slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	logger.Info("iniciando servidor UI de pagamentos")

	// 3. Inicializar OpenTelemetry
	ctx := context.Background()

	tp, err := telemetry.InitTracerProvider(ctx, cfg)
	if err != nil {
		logger.Error("failed to initialize tracer provider", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
		defer cancel()
		if err := mp.Shutdown(shutdownCtx); err != nil {
			logger.Error("meter provider shutdown error", "error", err)
		}
	}()

	meter := otel.Meter(cfg.OTel.ServiceName)

	// 4. Conectar ao Redis via provider centralizado
	rdb := provider.NewRedisClient(provider.DefaultRedisConfig(cfg.Redis.Addr, cfg.Redis.Password))
	defer func() {
		if err := rdb.Close(); err != nil {
			logger.Error("erro ao fechar Redis", "error", err)
		}
	}()

	// 5. Conectar ao DynamoDB via provider centralizado
	dynamoClient, err := provider.NewDynamoDBClient(context.Background(), provider.DynamoDBConfig{
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

	// 6. Criar servidor UI
	server := ui.NewServer(cfg, rdb, dynamoClient, logger, meter)

	// 7. Configurar desligamento gracioso (SIGINT/SIGTERM)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("sinal de desligamento recebido", "signal", sig.String())
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("erro no desligamento", "error", err)
		}
	}()

	// 9. Iniciar servidor
	logger.Info("servidor UI pronto", "port", cfg.UI.Port)
	if err := server.Start(); err != nil && err != http.ErrServerClosed {
		logger.Error("erro no servidor", "error", err)
		os.Exit(1)
	}

	logger.Info("servidor UI parado")
}
