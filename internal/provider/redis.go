package provider

import (
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisConfig contém parâmetros de conexão para o cliente Redis.
type RedisConfig struct {
	Addr         string
	Password     string
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// DefaultRedisConfig retorna uma RedisConfig com timeouts padrão.
func DefaultRedisConfig(addr, password string) RedisConfig {
	return RedisConfig{
		Addr:         addr,
		Password:     password,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}
}

// NewRedisClient cria um cliente Redis a partir de uma RedisConfig.
// Todos os serviços DEVEM usar esta função em vez de redis.NewClient diretamente.
func NewRedisClient(cfg RedisConfig) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})
}
