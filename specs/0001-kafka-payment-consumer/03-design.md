# 03 — Design

## Arquitetura

### Diagrama de Fluxo (Texto)

```
 ┌─────────────────────────────────────────────────────────────────────┐
 │                         Kafka Cluster                              │
 │  ┌──────────────────────────────┐   ┌──────────────────────────┐   │
 │  │   payment.events (3 part)    │   │ payment.events.dlq (1p)  │   │
 │  └────────────┬─────────────────┘   └────────────▲─────────────┘   │
 │               │                                   │                 │
 └───────────────│───────────────────────────────────│─────────────────┘
                 │                                   │
                 │ Consome                           │ Publica
                 ▼                                   │
 ┌──────────────────────────────┐                    │
 │       Kafka Consumer         │                    │
 │    (sarama consumer group)   │                    │
 │  payment-consumer-group      │                    │
 └──────────────┬───────────────┘                    │
                │                                    │
                ▼                                    │
 ┌──────────────────────────────┐                    │
 │     Payload Validator        │                    │
 │   (go-playground/validator)  │                    │
 │   valida schema + regras     │                    │
 └──────────────┬───────────────┘                    │
                │                                    │
                ▼                                    │
 ┌──────────────────────────────┐                    │
 │   Idempotency Checker        │                    │
 │   Redis: idempotency:<id>    │                    │
 │   SET NX + TTL 24h           │                    │
 └──────────────┬───────────────┘                    │
                │                                    │
                ▼ (já processado = skip)             │
 ┌──────────────────────────────┐                    │
 │  Payment Status Updater      │                    │
 │  Redis: payment:<id>         │                    │
 │  SET com TTL configurável    │                    │
 └──────────────┬───────────────┘                    │
                │                                    │
                ▼                                    │
 ┌──────────────────────────────┐                    │
 │    History Recorder          │                    │
 │   DynamoDB: payment_history  │                    │
 │   PutItem com retry          │                    │
 └──────────────┬───────────────┘                    │
                │                                    │
                ▼                                    │
 ┌──────────────────────────────┐                    │
 │      Retry Handler           │                    │
 │   backoff: 100ms→300ms→900ms │                    │
 │   max 3 tentativas           │                    │
 └──────┬───────────────┬───────┘                    │
        │               │                            │
     Sucesso          Falha                          │
        │               │                            │
        ▼               ▼────────────────────────────┘
   Commit offset      DLQ Producer
                      publica com headers
```

### Componentes Detalhados

#### 1. Kafka Consumer (`internal/consumer/`)

- Utiliza `sarama.ConsumerGroup` com consumer group `payment-consumer-group`.
- Implementa `sarama.ConsumerGroupHandler` interface.
- Configuração de sessão com `session.Context()` para graceful shutdown.
- Worker pool: cada mensagem é processada em uma goroutine separada, limitada
  por semáforo (buffered channel).
- Fetch de mensagens em lote (configurável via `Fetch.Max`), processa em paralelo.

```go
type Handler struct {
    validator    *validator.Validator
    idempotency  *idempotency.Checker
    status       *status.Updater
    history      *history.Recorder
    retry        *retry.Handler
    dlq          *dlq.Producer
    semaphore    chan struct{}
    logger       *slog.Logger
    tracer       trace.Tracer
}
```

#### 2. Payload Validator (`internal/validator/`)

- Struct `PaymentEvent` com tags `validate`.
- Validação com `go-playground/validator`.
- Função customizada para validar ISO 4217 e UUID v4.

```go
type PaymentEvent struct {
    PaymentID   string  `validate:"required,uuid4"`
    Status      string  `validate:"required,oneof=pending confirmed failed refunded"`
    Amount      float64 `validate:"required,gt=0"`
    Currency    string  `validate:"required,len=3,uppercase"`
    Description string  `validate:"omitempty,max=255,ascii"`
    Timestamp   string  `validate:"required,rfc3339"`
}
```

#### 3. Idempotency Checker (`internal/idempotency/`)

- Interface para facilitar testes com mock.

```go
type Checker interface {
    IsProcessed(ctx context.Context, paymentID string) (bool, error)
    MarkProcessed(ctx context.Context, paymentID string) error
}
```

- Implementação concreta com Redis:
  - `IsProcessed`: `GET idempotency:<payment_id>` → se existir, já processado.
  - `MarkProcessed`: `SET idempotency:<payment_id> "1" NX EX 86400` (24h).

#### 4. Payment Status Updater (`internal/status/`)

- Interface:

```go
type Updater interface {
    UpdateStatus(ctx context.Context, paymentID string, status string) error
}
```

- Implementação Redis:
  - `HSET payment:<payment_id> status <status> updated_at <timestamp>`.
  - `EXPIRE payment:<payment_id> <ttl>` (TTL configurável, default 7 dias).

#### 5. History Recorder (`internal/history/`)

- Interface:

```go
type Recorder interface {
    RecordHistory(ctx context.Context, event *models.PaymentEvent) error
}
```

- Implementação DynamoDB:
  - Tabela: `payment_history`
  - Chave primária: `payment_id` (String) + `timestamp` (String, sort key).
  - Atributos: `payment_id`, `status`, `amount`, `currency`, `description`, `timestamp`, `processed_at`, `trace_id`.

#### 6. Retry Handler (`internal/retry/`)

- Interface:

```go
type Handler interface {
    Do(ctx context.Context, fn func(context.Context) error) error
}
```

- Implementação:
  - 3 tentativas com backoff exponencial: 100ms, 300ms, 900ms.
  - Usa `math/rand` + jitter (±25%) para evitar thundering herd.
  - Contexto com timeout total de 30s.
  - Se todas falharem → retorna `ErrExhausted` para o consumer.

#### 7. DLQ Producer (`internal/dlq/`)

- Interface:

```go
type Producer interface {
    Publish(ctx context.Context, msg *sarama.ConsumerMessage, err error) error
}
```

- Implementação:
  - Publica no tópico `payment.events.dlq` usando `sarama.SyncProducer`.
  - Headers da mensagem original preservados + novos headers:
    - `original_topic`
    - `error_count`
    - `last_error`
    - `dlq_timestamp`

#### 8. Models (`internal/models/`)

```go
type PaymentEvent struct {
    PaymentID   string  `json:"payment_id"   validate:"required,uuid4"`
    Status      string  `json:"status"       validate:"required,oneof=pending confirmed failed refunded"`
    Amount      float64 `json:"amount"       validate:"required,gt=0"`
    Currency    string  `json:"currency"     validate:"required,len=3,uppercase"`
    Description string  `json:"description"  validate:"omitempty,max=255,ascii"`
    Timestamp   string  `json:"timestamp"    validate:"required,rfc3339"`
}

type PaymentStatus struct {
    PaymentID string `json:"payment_id"`
    Status    string `json:"status"`
    UpdatedAt string `json:"updated_at"`
}

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

#### 9. Telemetry (`pkg/telemetry/`)

- Inicialização do OpenTelemetry SDK.
- Provider de tracer com exportador OTLP.
- Provider de métricas com exportador OTLP.
- Middleware para injeção de contexto de tracing.
- Helper para criar spans com atributos padronizados.

### Estrutura de Pastas

```
cmd/
  consumer/
    main.go                          # Entry point, DI, inicialização
    main_test.go                     # Teste de integração do main

internal/
  consumer/
    handler.go                       # ConsumerGroupHandler
    handler_test.go                  # Testes unitários do handler
  validator/
    validator.go                     # Validação de payload
    validator_test.go                # Testes de validação
  idempotency/
    redis.go                         # Checador de idempotência (Redis)
    redis_test.go                    # Testes do idempotency
  status/
    redis.go                         # Atualizador de status (Redis)
    redis_test.go                    # Testes do status updater
  history/
    dynamodb.go                      # Gravador de histórico (DynamoDB)
    dynamodb_test.go                 # Testes do history recorder
  retry/
    handler.go                       # Lógica de retry com backoff
    handler_test.go                  # Testes do retry handler
  dlq/
    producer.go                      # Produtor de DLQ (Kafka)
    producer_test.go                 # Testes do DLQ producer
  models/
    payment.go                       # Structs compartilhadas
    payment_test.go                  # Testes dos models

pkg/
  telemetry/
    telemetry.go                     # Inicialização OTel
    telemetry_test.go                # Testes do telemetry

config/
  config.go                          # Carregamento de configuração
  config_test.go                     # Testes de config

.golangci.yml                        # Linter config
Dockerfile                           # Imagem Docker
docker-compose.yml                   # Stack local (Kafka, Redis, DynamoDB Local)
Makefile                             # Comandos de build, test, lint
```

### Contratos de Interface (Resumo)

```go
// Idempotency checker
type IdempotencyChecker interface {
    IsProcessed(ctx context.Context, paymentID string) (bool, error)
    MarkProcessed(ctx context.Context, paymentID string) error
}

// Status updater
type StatusUpdater interface {
    UpdateStatus(ctx context.Context, paymentID string, status string) error
}

// History recorder
type HistoryRecorder interface {
    RecordHistory(ctx context.Context, event *models.PaymentEvent) error
}

// Retry handler
type RetryHandler interface {
    Do(ctx context.Context, fn func(context.Context) error) error
}

// DLQ producer
type DLQProducer interface {
    Publish(ctx context.Context, msg *sarama.ConsumerMessage, err error) error
}

// Validator
type Validator interface {
    Validate(ctx context.Context, data []byte) (*models.PaymentEvent, error)
}
```

---

## Configuração

Todas as configurações via variáveis de ambiente:

| Variável                            | Default                     | Descrição                            |
|-------------------------------------|-----------------------------|--------------------------------------|
| `KAFKA_BROKERS`                     | `localhost:9092`            | Lista de brokers Kafka               |
| `KAFKA_TOPIC`                       | `payment.events`            | Tópico de consumo                    |
| `KAFKA_DLQ_TOPIC`                   | `payment.events.dlq`        | Tópico DLQ                           |
| `KAFKA_CONSUMER_GROUP`              | `payment-consumer-group`    | Consumer group ID                    |
| `REDIS_ADDR`                        | `localhost:6379`            | Endereço Redis                        |
| `REDIS_PASSWORD`                    | ``                          | Senha Redis                          |
| `DYNAMODB_ENDPOINT`                 | `http://localhost:4566`     | Endpoint DynamoDB (local)            |
| `DYNAMODB_TABLE`                    | `payment_history`           | Tabela DynamoDB                      |
| `WORKER_COUNT`                      | `10`                        | Número de workers concorrentes       |
| `IDEMPOTENCY_TTL_HOURS`             | `24`                        | TTL da chave de idempotência (horas) |
| `STATUS_TTL_HOURS`                  | `168`                       | TTL do status no Redis (7 dias)      |
| `RETRY_MAX_ATTEMPTS`                | `3`                         | Máximo de tentativas de retry        |
| `RETRY_BASE_DELAY_MS`               | `100`                       | Delay base do backoff (ms)           |
| `OTEL_EXPORTER_OTLP_ENDPOINT`       | `localhost:4317`            | Endpoint OTel collector              |
| `OTEL_SERVICE_NAME`                 | `payment-consumer`          | Nome do serviço para tracing         |
| `GRACEFUL_SHUTDOWN_TIMEOUT`         | `30s`                       | Timeout máximo para graceful shutdown|

---

## Fluxo Detalhado de Processamento de Mensagem

```
1. Handler.ConsumeClaim(session, claim)
   ├─ 2. Para cada mensagem em claim.Messages():
   │   ├─ 3. Adquire semáforo (worker pool)
   │   ├─ 4. Cria span filho com payment_id como atributo
   │   ├─ 5. Chama processMessage(ctx, msg)
   │   │   ├─ 6. validator.Validate(msg.Value)
   │   │   │   ├─ Sucesso → continua
   │   │   │   └─ Erro → publishDLQ + skip (erro permanente)
   │   │   ├─ 7. idempotency.IsProcessed(payment_id)
   │   │   │   ├─ True → log + commit + continua (próxima mensagem)
   │   │   │   └─ False → idempotency.MarkProcessed(payment_id)
   │   │   ├─ 8. retry.Do(ctx, func() {
   │   │   │   ├─ status.UpdateStatus(payment_id, status)
   │   │   │   └─ history.RecordHistory(event)
   │   │   │})
   │   │   ├─ 9. Se retry exaurido → dlq.Publish(msg, err)
   │   │   └─ 10. Se sucesso → commit + log
   │   └─ 11. Libera semáforo
   └─ 12. Sessão finalizada → cleanup
```

### Tratamento de Erros por Componente

| Componente       | Erro                    | Ação                                    |
|------------------|-------------------------|-----------------------------------------|
| Validator        | Payload inválido        | DLQ (erro permanente, sem retry)        |
| Idempotency      | Redis down              | Fallback para processamento otimista    |
| Status Updater   | Redis down              | Log erro, continua (não bloqueante)     |
| History Recorder | DynamoDB timeout/erro   | Retry (até 3x), se falhar → DLQ         |
| Retry            | Exaustão                | DLQ                                     |
| DLQ Producer     | Kafka down              | Log crítico, perda da mensagem          |

---

## Considerações de Concorrência

- **Semáforo**: `chan struct{}` com buffer de `WORKER_COUNT` tamanho.
- **Contexto**: cada mensagem recebe um context derivado do `session.Context()`.
- **Cancelamento**: ao receber SIGTERM, o contexto da sessão é cancelado → workers
  em andamento têm `GRACEFUL_SHUTDOWN_TIMEOUT` para finalizar.
- **Race conditions**: uso de Redis com `SET NX` garante atomicidade da
  idempotência. DynamoDB `PutItem` condicional (ConditionExpression) para
  evitar duplicatas na tabela de histórico.
- **Goroutine leaks**: todas as goroutines são criadas com `ctx` monitorado.
  Uso de `sync.WaitGroup` para garantir que todas finalizem no shutdown.
