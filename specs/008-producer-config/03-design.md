# 03 — Design

## Diagrama de Arquitetura (Configuração)

```
┌─────────────────────────────────────────────────────────┐
│                    Ordem de Precedência                  │
├─────────────────────────────────────────────────────────┤
│  1. Flag --port (se passada explicitamente)              │
│     └── parseServeFlagsArgs → serveFlags.apply()         │
│                                                          │
│  2. config.yaml → producer.port: 8082                    │
│     └── config.NewConfig() → cfg.Producer.Port           │
│                                                          │
│  3. Env var PRODUCER_PORT (Viper mapeia automaticamente) │
│     └── Viper: PRODUCER_PORT → producer_port →          │
│         producer.port → cfg.Producer.Port                │
│                                                          │
│  4. Fallback hardcoded 8082                              │
│     └── Se cfg.Producer.Port == 0, usar 8082             │
└─────────────────────────────────────────────────────────┘
```

## Fluxo de Dados (runServe)

```
runServe(args)
    │
    ├─ 1. cfg = config.NewConfig()         ← carrega YAML + env vars
    │         └── cfg.Producer.Port = 8082  (do producer.port no YAML)
    │
    ├─ 2. f = parseServeFlagsArgs(args)    ← parseia --port (default "8082")
    │
    ├─ 3. f.apply(&cfg)                    ← se --port != "", sobrescreve
    │         └── cfg.Producer.Port = X     (NÃO cfg.Server.Port)
    │
    ├─ 4. valida fallback:
    │         if cfg.Producer.Port == 0 → cfg.Producer.Port = 8082
    │
    └─ 5. port = fmt.Sprintf("%d", cfg.Producer.Port)
              usado no http.Server.Addr
```

## Contratos

### Config Struct (antes)

```go
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
```

### Config Struct (depois)

```go
type ProducerConfig struct {
    Port int `mapstructure:"port"`
}

type Config struct {
    Server   ServerConfig   `mapstructure:"server"`
    Producer ProducerConfig `mapstructure:"producer"`  // NOVO
    Kafka    KafkaConfig    `mapstructure:"kafka"`
    // ... demais campos inalterados
}
```

### config.yaml (depois)

```yaml
producer:
  port: 8082
```

### serveFlags.apply() (antes)

```go
func (f *serveFlags) apply(cfg *config.Config) {
    if f.port != "" {
        port, err := strconv.Atoi(f.port)
        if err == nil {
            cfg.Server.Port = port   // ← PROBLEMA: sobrescreve porta do consumer
        }
    }
}
```

### serveFlags.apply() (depois)

```go
func (f *serveFlags) apply(cfg *config.Config) {
    if f.port != "" {
        port, err := strconv.Atoi(f.port)
        if err == nil {
            cfg.Producer.Port = port  // ← CORRETO: usa campo do producer
        }
    }
}
```

## Decisões Arquiteturais

| Decisão | Alternativa | Justificativa |
|---------|-------------|---------------|
| Novo `ProducerConfig` em vez de reaproveitar `ServerConfig` | Usar `ServerConfig` para ambos | `ServerConfig` é semanticamente do consumer; misturar causaria confusão e impossibilitaria ter configurações diferentes no futuro |
| `mapstructure:"producer"` seguindo padrão existente | Nome diferente | Consistência com os outros blocos (`server`, `kafka`, `redis`, etc.) |
| Flag `--port` mantida | Remover flag | Compatibilidade retroativa; scripts existentes podem usar `--port` |
| Fallback `8082` se `cfg.Producer.Port == 0` | Falhar com erro | Tolerância a configuração parcial; o valor zero indica ausência |
