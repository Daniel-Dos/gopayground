package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/IBM/sarama"
)

// SyncProducer define a interface mínima para produzir mensagens no Kafka.
// Esta é a definição canônica compartilhada em todo o código.
type SyncProducer interface {
	SendMessage(msg *sarama.ProducerMessage) (partition int32, offset int64, err error)
}

// ProducerConfig contém parâmetros ajustáveis do produtor Sarama.
type ProducerConfig struct {
	Timeout         time.Duration
	RequiredAcks    sarama.RequiredAcks
	Idempotent      bool
	MaxMessageBytes int
}

// ConsumerConfig contém parâmetros ajustáveis do consumidor Sarama.
type ConsumerConfig struct {
	Version           sarama.KafkaVersion
	InitialOffset     int64
	MaxProcessingTime time.Duration
	RebalanceStrategy sarama.BalanceStrategy
	DialTimeout       time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
}

// DefaultProducerConfig retorna um ProducerConfig com valores padrão sensatos.
func DefaultProducerConfig() ProducerConfig {
	return ProducerConfig{
		Timeout:         10 * time.Second,
		RequiredAcks:    sarama.WaitForLocal,
		MaxMessageBytes: 100 * 1024,
	}
}

// DefaultConsumerConfig retorna um ConsumerConfig com valores padrão sensatos.
func DefaultConsumerConfig() ConsumerConfig {
	return ConsumerConfig{
		Version:           sarama.V2_6_0_0,
		InitialOffset:     sarama.OffsetOldest,
		MaxProcessingTime: 30 * time.Second,
		RebalanceStrategy: sarama.BalanceStrategyRoundRobin,
		DialTimeout:       10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
}

// NewProducerSaramaConfig cria um *sarama.Config para uso como produtor.
func NewProducerSaramaConfig(cfg ProducerConfig) *sarama.Config {
	saramaCfg := sarama.NewConfig()
	saramaCfg.Producer.Return.Successes = true
	saramaCfg.Producer.Timeout = cfg.Timeout
	saramaCfg.Producer.RequiredAcks = cfg.RequiredAcks
	saramaCfg.Producer.Idempotent = cfg.Idempotent
	if cfg.MaxMessageBytes > 0 {
		saramaCfg.Producer.MaxMessageBytes = cfg.MaxMessageBytes
	}
	saramaCfg.Net.DialTimeout = 5 * time.Second
	saramaCfg.Net.WriteTimeout = 5 * time.Second
	return saramaCfg
}

// NewConsumerSaramaConfig cria um *sarama.Config para uso como grupo de consumidores.
func NewConsumerSaramaConfig(cfg ConsumerConfig) *sarama.Config {
	saramaCfg := sarama.NewConfig()
	saramaCfg.Version = cfg.Version
	saramaCfg.Consumer.Return.Errors = true
	saramaCfg.Consumer.Offsets.Initial = cfg.InitialOffset
	saramaCfg.Consumer.Group.Rebalance.Strategy = cfg.RebalanceStrategy
	saramaCfg.Consumer.MaxProcessingTime = cfg.MaxProcessingTime
	saramaCfg.Producer.Return.Successes = true
	saramaCfg.Producer.Return.Errors = true
	saramaCfg.Producer.RequiredAcks = sarama.WaitForAll
	saramaCfg.Net.DialTimeout = cfg.DialTimeout
	saramaCfg.Net.ReadTimeout = cfg.ReadTimeout
	saramaCfg.Net.WriteTimeout = cfg.WriteTimeout
	saramaCfg.Consumer.Offsets.AutoCommit.Enable = true
	return saramaCfg
}

// NewSyncProducerWithRetry cria um sarama.SyncProducer com retry exponencial,
// respeitando cancelamento de contexto.
// Sequência: 500ms, 1s, 2s, 4s, 8s, 8s, ... (limitado em 8s)
// Timeout total: ~30 segundos.
func NewSyncProducerWithRetry(ctx context.Context, brokers []string, cfg *sarama.Config) (sarama.SyncProducer, error) {
	backoff := 500 * time.Millisecond
	const maxBackoff = 8 * time.Second
	const maxElapsed = 30 * time.Second
	deadline := time.Now().Add(maxElapsed)

	for {
		producer, err := sarama.NewSyncProducer(brokers, cfg)
		if err == nil {
			return producer, nil
		}

		if ctx.Err() != nil {
			return nil, fmt.Errorf("canceled while waiting for Kafka: %w", ctx.Err())
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out after %v connecting to Kafka: %w", maxElapsed, err)
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

// NewConsumerGroup cria um sarama.ConsumerGroup.
func NewConsumerGroup(brokers []string, groupID string, cfg *sarama.Config) (sarama.ConsumerGroup, error) {
	return sarama.NewConsumerGroup(brokers, groupID, cfg)
}
