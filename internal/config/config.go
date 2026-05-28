package config

import (
	"log"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// ServerConfig contém configurações do servidor HTTP.
type ServerConfig struct {
	Port                   int           `mapstructure:"port"`
	GracefulShutdownTimeout time.Duration `mapstructure:"graceful_shutdown_timeout"`
}

// KafkaConfig contém configurações do Apache Kafka (produtor e consumidor).
type KafkaConfig struct {
	Brokers         string        `mapstructure:"brokers"`
	Topic           string        `mapstructure:"topic"`
	DLQTopic        string        `mapstructure:"dlq_topic"`
	ConsumerGroup   string        `mapstructure:"consumer_group"`
	ProducerTimeout time.Duration `mapstructure:"producer_timeout"`
	DialTimeout     time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	Idempotent      bool          `mapstructure:"idempotent"`
	RequiredAcks    string        `mapstructure:"required_acks"`
	MaxMessageBytes int           `mapstructure:"max_message_bytes"`
	Version         string        `mapstructure:"version"`
}

// RedisConfig contém configurações do Redis.
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
}

// DynamoDBConfig contém configurações do DynamoDB (AWS ou localstack).
type DynamoDBConfig struct {
	Endpoint  string `mapstructure:"endpoint"`
	Table     string `mapstructure:"table"`
	Region    string `mapstructure:"region"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
}

// UIConfig contém configurações do servidor HTTP da UI.
type UIConfig struct {
	Port           string        `mapstructure:"port"`
	ProducerURL    string        `mapstructure:"producer_url"`
	EventBusBuffer int           `mapstructure:"event_bus_buffer"`
	ReadTimeout    time.Duration `mapstructure:"read_timeout"`
	WriteTimeout   time.Duration `mapstructure:"write_timeout"`
}

// WorkerConfig contém configurações dos workers de processamento.
type WorkerConfig struct {
	Count               int `mapstructure:"count"`
	IdempotencyTTLHours int `mapstructure:"idempotency_ttl_hours"`
	StatusTTLHours      int `mapstructure:"status_ttl_hours"`
}

// RetryConfig contém configurações de retry (backoff exponencial).
type RetryConfig struct {
	MaxAttempts int `mapstructure:"max_attempts"`
	BaseDelayMs int `mapstructure:"base_delay_ms"`
}

// OTelConfig contém configurações do OpenTelemetry (tracing e métricas).
type OTelConfig struct {
	Endpoint    string `mapstructure:"endpoint"`
	ServiceName string `mapstructure:"service_name"`
}

// Config agrupa toda a configuração da aplicação por domínio.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Kafka    KafkaConfig    `mapstructure:"kafka"`
	Redis    RedisConfig    `mapstructure:"redis"`
	DynamoDB DynamoDBConfig `mapstructure:"dynamodb"`
	UI       UIConfig       `mapstructure:"ui"`
	Worker   WorkerConfig   `mapstructure:"worker"`
	Retry    RetryConfig    `mapstructure:"retry"`
	OTel     OTelConfig     `mapstructure:"otel"`
}

// NewConfig carrega configurações a partir do arquivo config.yaml
// e variáveis de ambiente (12-Factor). O arquivo YAML é obrigatório
// e serve como fonte primária de defaults. Variáveis de ambiente
// sobrescrevem os valores do YAML.
func NewConfig() Config {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("..")
	viper.AddConfigPath("../..")

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// O arquivo config.yaml é obrigatório — contém todos os defaults.
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Failed to read config.yaml: %v", err)
	}

	var config Config
	if err := viper.Unmarshal(&config, viper.DecodeHook(mapstructure.StringToTimeDurationHookFunc())); err != nil {
		log.Fatalf("Unable to decode config: %v", err)
	}

	return config
}
