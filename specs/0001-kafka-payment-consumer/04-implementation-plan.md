# 04 — Plano de Implementação

## Ordem de Implementação

A implementação deve seguir a ordem abaixo para garantir que cada componente
possa ser testado isoladamente antes de ser integrado.

```
 1. models           → Structs de domínio
 2. config           → Carregamento de configuração
 3. validator        → Validação de payload
 4. idempotency      → Checador de idempotência (Redis)
 5. status           → Atualizador de status (Redis)
 6. history          → Gravador de histórico (DynamoDB)
 7. retry            → Handler de retry com backoff
 8. dlq              → Produtor de DLQ (Kafka)
 9. telemetry        → Inicialização OTel
10. consumer         → ConsumerGroupHandler + integração
11. main             → Entry point, DI, inicialização
12. docker-compose   → Stack local para desenvolvimento
```

---

## Passo 1 — Models

**Arquivos**: `internal/models/payment.go`, `internal/models/payment_test.go`

### O que implementar

```go
// PaymentEvent representa o evento recebido do Kafka
type PaymentEvent struct {
    PaymentID   string  `json:"payment_id"   validate:"required,uuid4"`
    Status      string  `json:"status"       validate:"required,oneof=pending confirmed failed refunded"`
    Amount      float64 `json:"amount"       validate:"required,gt=0"`
    Currency    string  `json:"currency"     validate:"required,len=3,uppercase"`
    Description string  `json:"description"  validate:"omitempty,max=255,ascii"`
    Timestamp   string  `json:"timestamp"    validate:"required,rfc3339"`
}

// PaymentStatus representa o status atual no Redis
type PaymentStatus struct {
    PaymentID string `json:"payment_id"`
    Status    string `json:"status"`
    UpdatedAt string `json:"updated_at"`
}

// PaymentHistory representa o registro no DynamoDB
type PaymentHistory struct {
    PaymentID   string    `dynamodbav:"payment_id"`
    Status      string    `dynamodbav:"status"`
    Amount      float64   `dynamodbav:"amount"`
    Currency    string    `dynamodbav:"currency"`
    Description string    `dynamodbav:"description"`
    Timestamp   string    `dynamodbav:"timestamp"`
    ProcessedAt time.Time `dynamodbav:"processed_at"`
    TraceID     string    `dynamodbav:"trace_id"`
}
```

### Testes

- Testar criação de `PaymentHistory` a partir de `PaymentEvent`.
- Verificar tags de validação e serialização.

### Dependências

- `encoding/json` (stdlib)

---

## Passo 2 — Config

**Arquivos**: `config/config.go`, `config/config_test.go`

### O que implementar

Struct `Config` com todas as variáveis de ambiente mapeadas.
Método `Load()` que lê de env vars com defaults.

```go
type Config struct {
    KafkaBrokers          string        `envconfig:"KAFKA_BROKERS" default:"localhost:9092"`
    KafkaTopic            string        `envconfig:"KAFKA_TOPIC" default:"payment.events"`
    KafkaDLQTopic         string        `envconfig:"KAFKA_DLQ_TOPIC" default:"payment.events.dlq"`
    KafkaConsumerGroup    string        `envconfig:"KAFKA_CONSUMER_GROUP" default:"payment-consumer-group"`
    RedisAddr             string        `envconfig:"REDIS_ADDR" default:"localhost:6379"`
    RedisPassword         string        `envconfig:"REDIS_PASSWORD" default:""`
	DynamoDBEndpoint      string        `envconfig:"DYNAMODB_ENDPOINT" default:"http://localhost:4566"`
    DynamoDBTable         string        `envconfig:"DYNAMODB_TABLE" default:"payment_history"`
    WorkerCount           int           `envconfig:"WORKER_COUNT" default:"10"`
    IdempotencyTTLHours   int           `envconfig:"IDEMPOTENCY_TTL_HOURS" default:"24"`
    StatusTTLHours        int           `envconfig:"STATUS_TTL_HOURS" default:"168"`
    RetryMaxAttempts      int           `envconfig:"RETRY_MAX_ATTEMPTS" default:"3"`
    RetryBaseDelayMs      int           `envconfig:"RETRY_BASE_DELAY_MS" default:"100"`
    OTelEndpoint          string        `envconfig:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"localhost:4317"`
    OTelServiceName       string        `envconfig:"OTEL_SERVICE_NAME" default:"payment-consumer"`
    GracefulShutdownTimeout time.Duration `envconfig:"GRACEFUL_SHUTDOWN_TIMEOUT" default:"30s"`
}
```

### Testes

- Testar valores default.
- Testar override via variáveis de ambiente.

---

## Passo 3 — Validator

**Arquivos**: `internal/validator/validator.go`, `internal/validator/validator_test.go`

### O que implementar

```go
// Validator interface
type Validator interface {
    Validate(ctx context.Context, data []byte) (*models.PaymentEvent, error)
}

type paymentValidator struct {
    validate *validator.Validate
}

func New() Validator {
    v := validator.New()
    // Registra validação customizada para RFC3339
    v.RegisterValidation("rfc3339", validateRFC3339)
    return &paymentValidator{validate: v}
}

func (pv *paymentValidator) Validate(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
    var event models.PaymentEvent
    if err := json.Unmarshal(data, &event); err != nil {
        return nil, fmt.Errorf("unmarshal error: %w", err)
    }
    if err := pv.validate.StructCtx(ctx, event); err != nil {
        return nil, fmt.Errorf("validation error: %w", err)
    }
    return &event, nil
}
```

### Regras Customizadas

- `rfc3339`: valida se string é timestamp RFC3339 válido e não é futuro (> 5 min skew).
- `uppercase`: já nativo do validator, mas deve ser configurado.

### Testes

- Payload válido.
- Campo ausente.
- Campo com tipo errado.
- UUID inválido.
- Status inválido.
- Amount zero/negativo.
- Moeda inválida (não ISO 4217).
- Timestamp futuro.
- Timestamp mal formatado.
- Description com mais de 255 chars.
- Description com caracteres de controle.

---

## Passo 4 — Idempotency

**Arquivos**: `internal/idempotency/redis.go`, `internal/idempotency/redis_test.go`

### O que implementar

```go
type Checker interface {
    IsProcessed(ctx context.Context, paymentID string) (bool, error)
    MarkProcessed(ctx context.Context, paymentID string) error
}

type redisChecker struct {
    client *redis.Client
    ttl    time.Duration
}

func NewChecker(client *redis.Client, ttlHours int) Checker {
    return &redisChecker{
        client: client,
        ttl:    time.Duration(ttlHours) * time.Hour,
    }
}

func (rc *redisChecker) IsProcessed(ctx context.Context, paymentID string) (bool, error) {
    key := fmt.Sprintf("idempotency:%s", paymentID)
    exists, err := rc.client.Exists(ctx, key).Result()
    if err != nil {
        return false, fmt.Errorf("redis exists error: %w", err)
    }
    return exists == 1, nil
}

func (rc *redisChecker) MarkProcessed(ctx context.Context, paymentID string) error {
    key := fmt.Sprintf("idempotency:%s", paymentID)
    ok, err := rc.client.SetNX(ctx, key, "1", rc.ttl).Result()
    if err != nil {
        return fmt.Errorf("redis setnx error: %w", err)
    }
    if !ok {
        return fmt.Errorf("idempotency key already exists: %s", paymentID)
    }
    return nil
}
```

### Testes

- `IsProcessed` retorna `false` para chave inexistente.
- `IsProcessed` retorna `true` após `MarkProcessed`.
- `MarkProcessed` falha se chave já existe (simular corrida).
- Timeout de contexto.
- Redis indisponível (erro propagado).

---

## Passo 5 — Status Updater

**Arquivos**: `internal/status/redis.go`, `internal/status/redis_test.go`

### O que implementar

```go
type Updater interface {
    UpdateStatus(ctx context.Context, paymentID string, status string) error
}

type redisUpdater struct {
    client *redis.Client
    ttl    time.Duration
}

func NewUpdater(client *redis.Client, ttlHours int) Updater {
    return &redisUpdater{
        client: client,
        ttl:    time.Duration(ttlHours) * time.Hour,
    }
}

func (ru *redisUpdater) UpdateStatus(ctx context.Context, paymentID string, status string) error {
    key := fmt.Sprintf("payment:%s", paymentID)
    pipe := ru.client.Pipeline()
    pipe.HSet(ctx, key, map[string]interface{}{
        "payment_id": paymentID,
        "status":     status,
        "updated_at": time.Now().UTC().Format(time.RFC3339),
    })
    pipe.Expire(ctx, key, ru.ttl)
    _, err := pipe.Exec(ctx)
    if err != nil {
        return fmt.Errorf("redis pipeline error: %w", err)
    }
    return nil
}
```

### Testes

- `UpdateStatus` cria chave com campos corretos.
- `UpdateStatus` sobrescreve status anterior.
- TTL é configurado corretamente.
- Timeout/erro do Redis.

---

## Passo 6 — History Recorder

**Arquivos**: `internal/history/dynamodb.go`, `internal/history/dynamodb_test.go`

### O que implementar

```go
type Recorder interface {
    RecordHistory(ctx context.Context, event *models.PaymentEvent) error
}

type dynamoRecorder struct {
    client *dynamodb.Client
    table  string
}

func NewRecorder(client *dynamodb.Client, table string) Recorder {
    return &dynamoRecorder{client: client, table: table}
}

func (dr *dynamoRecorder) RecordHistory(ctx context.Context, event *models.PaymentEvent) error {
    history := models.PaymentHistory{
        PaymentID:   event.PaymentID,
        Status:      event.Status,
        Amount:      event.Amount,
        Currency:    event.Currency,
        Description: event.Description,
        Timestamp:   event.Timestamp,
        ProcessedAt: time.Now().UTC(),
        TraceID:     trace.SpanFromContext(ctx).SpanContext().TraceID().String(),
    }

    item, err := dynamodbav.MarshalMap(history)
    if err != nil {
        return fmt.Errorf("marshal error: %w", err)
    }

    _, err = dr.client.PutItem(ctx, &dynamodb.PutItemInput{
        TableName: aws.String(dr.table),
        Item:      item,
        // Evita duplicatas: se já existe payment_id + timestamp, falha
        ConditionExpression: aws.String("attribute_not_exists(payment_id) AND attribute_not_exists(#ts)"),
        ExpressionAttributeNames: map[string]string{
            "#ts": "timestamp",
        },
    })
    if err != nil {
        // Condição falhou → já existe (idempotência do DynamoDB)
        var condErr *types.ConditionalCheckFailedException
        if errors.As(err, &condErr) {
            return nil // já registrado, não é erro
        }
        return fmt.Errorf("dynamodb put error: %w", err)
    }

    return nil
}
```

### Testes

- `RecordHistory` insere item com sucesso.
- `RecordHistory` não retorna erro se item já existe (ConditionalCheckFailed).
- Timeout/erro do DynamoDB.
- Tracing (trace_id) está presente no item.

---

## Passo 7 — Retry Handler

**Arquivos**: `internal/retry/handler.go`, `internal/retry/handler_test.go`

### O que implementar

```go
type Handler interface {
    Do(ctx context.Context, fn func(context.Context) error) error
}

type retryHandler struct {
    maxAttempts int
    baseDelay   time.Duration
    jitter      float64 // ±25%
}

func NewHandler(maxAttempts int, baseDelayMs int) Handler {
    return &retryHandler{
        maxAttempts: maxAttempts,
        baseDelay:   time.Duration(baseDelayMs) * time.Millisecond,
        jitter:      0.25,
    }
}

func (rh *retryHandler) Do(ctx context.Context, fn func(context.Context) error) error {
    var lastErr error

    for attempt := 0; attempt < rh.maxAttempts; attempt++ {
        if attempt > 0 {
            delay := rh.baseDelay * time.Duration(1<<(attempt-1)) // exp: 1x, 2x, 4x
            delay = addJitter(delay, rh.jitter)

            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(delay):
            }
        }

        err := fn(ctx)
        if err == nil {
            return nil
        }

        lastErr = err
        slog.Warn("retry attempt failed",
            "attempt", attempt+1,
            "max_attempts", rh.maxAttempts,
            "error", err,
        )
    }

    return fmt.Errorf("all %d attempts failed: %w", rh.maxAttempts, lastErr)
}
```

### Jitter

```go
func addJitter(d time.Duration, jitterPct float64) time.Duration {
    jitter := time.Duration(float64(d) * jitterPct * (rand.Float64()*2 - 1))
    return d + jitter
}
```

### Testes

- Sucesso na primeira tentativa.
- Sucesso após retry.
- Falha após exaustão de tentativas.
- Contexto cancelado durante backoff.
- Jitter não produz delays negativos.

---

## Passo 8 — DLQ Producer

**Arquivos**: `internal/dlq/producer.go`, `internal/dlq/producer_test.go`

### O que implementar

```go
type Producer interface {
    Publish(ctx context.Context, msg *sarama.ConsumerMessage, err error) error
}

type kafkaDLQProducer struct {
    producer sarama.SyncProducer
    topic    string
}

func NewProducer(producer sarama.SyncProducer, topic string) Producer {
    return &kafkaDLQProducer{producer: producer, topic: topic}
}

func (kp *kafkaDLQProducer) Publish(ctx context.Context, msg *sarama.ConsumerMessage, err error) error {
    headers := []sarama.RecordHeader{
        {Key: []byte("original_topic"), Value: []byte(msg.Topic)},
        {Key: []byte("original_partition"), Value: []byte(strconv.Itoa(int(msg.Partition)))},
        {Key: []byte("original_offset"), Value: []byte(strconv.FormatInt(msg.Offset, 10))},
        {Key: []byte("error_count"), Value: []byte("3")},
        {Key: []byte("last_error"), Value: []byte(err.Error())},
        {Key: []byte("dlq_timestamp"), Value: []byte(time.Now().UTC().Format(time.RFC3339))},
    }

    dlqMsg := &sarama.ProducerMessage{
        Topic:   kp.topic,
        Key:     msg.Key,
        Value:   msg.Value,
        Headers: append(msg.Headers, headers...),
    }

    _, _, err = kp.producer.SendMessage(dlqMsg)
    if err != nil {
        return fmt.Errorf("dlq publish error: %w", err)
    }
    return nil
}
```

### Testes

- Mensagem publicada com headers corretos.
- Erro ao publicar propagado.
- Key original preservada.

---

## Passo 9 — Telemetry

**Arquivos**: `pkg/telemetry/telemetry.go`, `pkg/telemetry/telemetry_test.go`

### O que implementar

```go
func InitTracerProvider(ctx context.Context, cfg config.Config) (*sdktrace.TracerProvider, error)
func InitMeterProvider(ctx context.Context, cfg config.Config) (*sdkmetric.MeterProvider, error)
```

- Configura OTLP exporter via gRPC.
- Configura TracerProvider com `AlwaysSample` sampler (desenvolvimento),
  `ParentBased` sampler (produção).
- Configura MeterProvider com leitor periódico.
- Register global providers.

---

## Passo 10 — Consumer Handler

**Arquivos**: `internal/consumer/handler.go`, `internal/consumer/handler_test.go`

### O que implementar

```go
type Handler struct {
    validator   validator.Validator
    idempotency idempotency.Checker
    status      status.Updater
    history     history.Recorder
    retry       retry.Handler
    dlq         dlq.Producer
    semaphore   chan struct{}
    logger      *slog.Logger
    tracer      trace.Tracer

    // Métricas
    messagesReceived   instrument.Int64Counter
    messagesProcessed  instrument.Int64Counter
    processingDuration instrument.Float64Histogram
    retryAttempts      instrument.Int64Counter
    dlqPublished       instrument.Int64Counter
    idempotencyHits    instrument.Int64Counter
}

func NewHandler(
    validator validator.Validator,
    idempotency idempotency.Checker,
    status status.Updater,
    history history.Recorder,
    retry retry.Handler,
    dlq dlq.Producer,
    workerCount int,
    meter metric.Meter,
    tracer trace.Tracer,
) *Handler { ... }

// Setup implementa sarama.ConsumerGroupHandler.Setup
func (h *Handler) Setup(session sarama.ConsumerGroupSession) error { ... }

// Cleanup implementa sarama.ConsumerGroupHandler.Cleanup
func (h *Handler) Cleanup(session sarama.ConsumerGroupSession) error { ... }

// ConsumeClaim implementa sarama.ConsumerGroupHandler.ConsumeClaim
func (h *Handler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error { ... }
```

### Fluxo do `processMessage`

```go
func (h *Handler) processMessage(ctx context.Context, msg *sarama.ConsumerMessage) error {
    // 1. Validar payload
    event, err := h.validator.Validate(ctx, msg.Value)
    if err != nil {
        // Erro permanente → DLQ direto
        if dlqErr := h.dlq.Publish(ctx, msg, err); dlqErr != nil {
            h.logger.ErrorContext(ctx, "failed to publish to DLQ", "error", dlqErr)
        }
        return err
    }

    // 2. Idempotência
    processed, err := h.idempotency.IsProcessed(ctx, event.PaymentID)
    if err != nil {
        h.logger.WarnContext(ctx, "idempotency check failed, proceeding optimistically", "error", err)
    }
    if processed {
        h.idempotencyHits.Add(ctx, 1)
        h.logger.InfoContext(ctx, "message already processed, skipping", "payment_id", event.PaymentID)
        return nil
    }

    // 3. Marcar como processado (se falhar, next consumer pode reprocessar)
    if err := h.idempotency.MarkProcessed(ctx, event.PaymentID); err != nil {
        h.logger.WarnContext(ctx, "failed to mark idempotency", "error", err)
    }

    // 4. Executar com retry
    err = h.retry.Do(ctx, func(ctx context.Context) error {
        // Atualizar status no Redis
        if err := h.status.UpdateStatus(ctx, event.PaymentID, event.Status); err != nil {
            return fmt.Errorf("status update failed: %w", err)
        }
        // Gravar histórico no DynamoDB
        if err := h.history.RecordHistory(ctx, event); err != nil {
            return fmt.Errorf("history record failed: %w", err)
        }
        return nil
    })

    if err != nil {
        // Retry exaurido → DLQ
        if dlqErr := h.dlq.Publish(ctx, msg, err); dlqErr != nil {
            h.logger.ErrorContext(ctx, "failed to publish to DLQ", "error", dlqErr)
        }
        return err
    }

    return nil
}
```

### Testes

- Mensagem válida processada com sucesso.
- Mensagem duplicada ignorada (idempotência).
- Payload inválido vai para DLQ.
- Falha de DynamoDB aciona retry.
- Exaustão de retry vai para DLQ.
- Graceful shutdown: mensagens em andamento são finalizadas.

---

## Passo 11 — Main

**Arquivos**: `cmd/consumer/main.go`

### O que implementar

```go
func main() {
    // 1. Carregar config
    cfg := config.Load()

    // 2. Configurar logging
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

    // 3. Inicializar OTel
    ctx := context.Background()
    tp, err := telemetry.InitTracerProvider(ctx, cfg)
    // ...
    defer tp.Shutdown(ctx)
    mp, err := telemetry.InitMeterProvider(ctx, cfg)
    // ...
    defer mp.Shutdown(ctx)

    // 4. Conectar Redis
    rdb := redis.NewClient(&redis.Options{
        Addr:     cfg.RedisAddr,
        Password: cfg.RedisPassword,
    })
    defer rdb.Close()

    // 5. Conectar DynamoDB
    awsCfg, err := config.LoadDefaultConfig(ctx)
    dynamoClient := dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
        o.BaseEndpoint = aws.String(cfg.DynamoDBEndpoint)
    })

    // 6. Configurar Kafka consumer + producer
    kafkaCfg := sarama.NewConfig()
    // ... configurações de consumer, producer, versão
    consumerGroup, err := sarama.NewConsumerGroup(cfg.KafkaBrokers, cfg.KafkaConsumerGroup, kafkaCfg)
    defer consumerGroup.Close()
    syncProducer, err := sarama.NewSyncProducer(cfg.KafkaBrokers, kafkaCfg)
    defer syncProducer.Close()

    // 7. Instanciar componentes
    validator := validator.New()
    idempotencyChecker := idempotency.NewChecker(rdb, cfg.IdempotencyTTLHours)
    statusUpdater := status.NewUpdater(rdb, cfg.StatusTTLHours)
    historyRecorder := history.NewRecorder(dynamoClient, cfg.DynamoDBTable)
    retryHandler := retry.NewHandler(cfg.RetryMaxAttempts, cfg.RetryBaseDelayMs)
    dlqProducer := dlq.NewProducer(syncProducer, cfg.KafkaDLQTopic)

    // 8. Criar handler
    handler := consumer.NewHandler(
        validator, idempotencyChecker, statusUpdater,
        historyRecorder, retryHandler, dlqProducer,
        cfg.WorkerCount, meter, tracer,
    )

    // 9. Graceful shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    // 10. Loop de consumo
    for {
        select {
        case <-sigCh:
            logger.Info("shutting down...")
            return
        default:
            if err := consumerGroup.Consume(ctx, []string{cfg.KafkaTopic}, handler); err != nil {
                logger.Error("consume error", "error", err)
            }
        }
    }
}
```

---

## Passo 12 — Docker Compose

**Arquivo**: `docker-compose.yml`

Serviços:
- `kafka` (apache/kafka) em KRaft mode (sem ZooKeeper), com 3 partições para `payment.events`
- `redis` (redis:7-alpine)
- `floci` (floci/floci:latest)
- `otel-collector` (otel/opentelemetry-collector-contrib:latest)

---

## Dependências Go (go.mod)

```
module github.com/seuorg/payment-consumer

go 1.22

require (
    github.com/IBM/sarama v1.43.0
    github.com/redis/go-redis/v9 v9.5.1
    github.com/aws/aws-sdk-go-v2 v1.26.0
    github.com/aws/aws-sdk-go-v2/service/dynamodb v1.31.0
    github.com/aws/aws-sdk-go-v2/config v1.27.0
    github.com/go-playground/validator/v10 v10.19.0
    go.opentelemetry.io/otel v1.26.0
    go.opentelemetry.io/otel/sdk v1.26.0
    go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.26.0
    go.opentelemetry.io/otel/exporters/otlp/otlpmetric v1.26.0
    go.opentelemetry.io/otel/metric v1.26.0
    github.com/stretchr/testify v1.9.0
)
```

---

## Makefile (Comandos Essenciais)

```makefile
.PHONY: test lint build run docker-up

test:
    go test ./... -race -count=1 -timeout=60s

lint:
    golangci-lint run ./...

build:
    go build -o bin/consumer ./cmd/consumer

run:
    go run ./cmd/consumer

docker-up:
    docker-compose up -d

docker-down:
    docker-compose down
```
