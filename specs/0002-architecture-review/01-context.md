# Architecture Review — 0002

## Date
2026-05-25

## Scope
Full project architecture analysis after session changes covering:
- Infrastructure (Docker, Compose, containerization)
- Code changes (producer, consumer, UI, DynamoDB init)
- Observability (OpenTelemetry)
- Architectural decisions and trade-offs

---

## 1. Current Project Structure Overview

```
app/
├── .opencode/                   # OpenCode agent/skill configuration
├── adrs/                        # Architecture Decision Records
├── cmd/
│   ├── consumer/main.go         # Kafka consumer service
│   ├── producer/main.go         # CLI producer tool
│   └── ui/main.go               # Web UI server
├── config.yaml                  # Static config (only server.port)
├── data/                        # Local data volume (for Floci)
├── docker-compose.yml           # Infra-only stack (Kafka, Redis, Floci, OTel, UI)
├── docker-compose-events.yml    # Full stack (infra + consumer + producer + UI)
├── Dockerfile                   # Root app (legacy — builds cmd? main.go)
├── Dockerfile.consumer          # Consumer service image
├── Dockerfile.producer          # Producer CLI image
├── Dockerfile.ui                # UI server image
├── docs/
├── go.mod / go.sum
├── internal/
│   ├── config/config.go         # Viper-based configuration
│   ├── config/config_test.go
│   ├── consumer/
│   │   ├── handler.go           # Sarama ConsumerGroupHandler
│   │   └── handler_test.go
│   ├── dlq/
│   │   ├── producer.go          # Dead Letter Queue producer
│   │   └── producer_test.go
│   ├── history/
│   │   ├── dynamodb.go          # PaymentHistory recorder (DynamoDB PutItem)
│   │   ├── dynamodb_init.go     # Auto-create DynamoDB table (EnsureTable)
│   │   └── dynamodb_test.go
│   ├── idempotency/
│   │   ├── redis.go             # Idempotency check via Redis SETNX
│   │   └── redis_test.go
│   ├── main.go                  # Legacy/exploratory: Ollama + LangChain test
│   ├── models/
│   │   ├── payment.go           # PaymentEvent, PaymentStatus, PaymentHistory
│   │   └── payment_test.go
│   ├── producer/
│   │   ├── producer.go          # Event publishing logic + generators
│   │   └── producer_test.go
│   ├── retry/
│   │   ├── handler.go           # Retry with exponential backoff + jitter
│   │   └── handler_test.go
│   ├── status/
│   │   ├── redis.go             # Payment status update via Redis HSet
│   │   └── redis_test.go
│   ├── ui/
│   │   ├── events.go            # EventBus (Redis Pub/Sub + local subscribers)
│   │   ├── events_test.go
│   │   ├── handlers.go          # HTTP handlers (SSE, payments, history, metrics)
│   │   ├── handlers_test.go
│   │   ├── helpers_test.go
│   │   ├── server.go            # HTTP server setup + middleware
│   │   ├── server_test.go
│   │   └── static/              # Frontend assets
│   │       ├── app.js
│   │       ├── index.html
│   │       └── style.css
│   └── validator/
│       ├── validator.go         # Payload validation (go-playground)
│       └── validator_test.go
├── main.go                      # Legacy HTTP server (old entry point)
├── Makefile
├── node_modules/
├── otel-collector-config.yaml   # OTel collector pipeline config
├── package.json / package-lock.json
├── pkg/
│   ├── model/user.go            # Legacy model
│   └── telemetry/
│       ├── telemetry.go         # OTLP tracer + meter init
│       └── telemetry_test.go
├── README.md
└── specs/                       # Spec-driven development docs
    ├── 0001-kafka-payment-consumer/
    ├── 0002-payment-ui/
    ├── 0003-cli-producer/
    └── 0002-architecture-review/  ← THIS FILE
```

---

## 2. Component Diagram (ASCII)

```
┌──────────────────────────────────────────────────────────────────────────┐
│                          Docker Network: payment-network                 │
│                                                                          │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐               │
│  │   Producer   │    │  Consumer    │    │     UI       │               │
│  │ (CLI / once) │───▶│ (always-on)  │◀───│ (always-on)  │               │
│  └──────┬───────┘    └──────┬───────┘    └──────┬───────┘               │
│         │                   │                    │                        │
│         │  publish          │                    │                        │
│         ▼                   │                    │                        │
│  ┌──────────────┐           │              ┌─────┴──────┐               │
│  │    Kafka     │◀──────────│──────────────│  Redis     │               │
│  │ payment.events│          │ Pub/Sub      │  (status,  │               │
│  │ payment.events│──────────│──────────────│ idempot.,  │               │
│  │    .dlq      │           │              │  events)   │               │
│  └──────────────┘           │              └────────────┘               │
│         │                   │                                            │
│         │  DLQ publish      │  RecordHistory                             │
│         ▼                   ▼                                            │
│  ┌─────────────────────────────────────────┐                            │
│  │              DynamoDB (Floci)            │                            │
│  │         ┌──────────────────────┐        │                            │
│  │         │  payment_history     │        │                            │
│  │         │  PK: payment_id      │        │                            │
│  │         │  SK: timestamp       │        │                            │
│  │         └──────────────────────┘        │                            │
│  └─────────────────────────────────────────┘                            │
│         │                                                                │
│         ▼                                                                │
│  ┌─────────────────────────────────────────┐                            │
│  │        OpenTelemetry Collector          │                            │
│  │  OTLP gRPC:4317 / OTLP HTTP:4318        │                            │
│  │  Exporters: debug, otlp (jaeger)        │                            │
│  └─────────────────────────────────────────┘                            │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Data Flow

### 3.1 Happy Path: Payment Event → Consumer → Storage

```
1. Producer publishes to Kafka topic "payment.events"
     └─ Event: { payment_id, status, amount, currency, description, timestamp }
     └─ Key: payment_id (partitioning)
     └─ Headers: source, timestamp

2. Consumer receives via Sarama ConsumerGroup "payment-consumer-group"
     └─ Validates payload (JSON schema + go-playground rules)
     └─ Idempotency check via Redis SETNX (key: "idempotency:<payment_id>")
     └─ If duplicate → skip (mark message, return nil)

3. Processing (inside retry handler, max 3 attempts):
     └─ Status update → Redis HSET "payment:<payment_id>" {status, updated_at}
     └─ History record → DynamoDB PutItem (payment_history table)
          └─ Condition: attribute_not_exists(payment_id) AND attribute_not_exists(timestamp)
          └─ Fields: payment_id, status, amount, currency, description, timestamp, processed_at, trace_id

4. Consumer marks message as processed (sarama session.MarkMessage)

5. Consumer publishes event to Redis Pub/Sub channel "payment:events"
     └─ UI EventBus subscribes → distributes to SSE clients
```

### 3.2 Failure Paths

```
Validation failure
  └─ Publish original message to DLQ topic "payment.events.dlq"
  └─ DLQ headers: original_topic, original_partition, original_offset, error_count, last_error

Processing failure (retry exhausted)
  └─ Publish to DLQ with enriched headers (trace_id, dlq_timestamp)

Redis unavailable
  └─ Consumer logs warning but continues (optimistic processing)
  └─ UI health check returns 503

DynamoDB unavailable
  └─ Consumer retry will fail → eventually DLQ
  └─ Health check /readyz returns 503

Kafka unavailable at startup
  └─ Producer uses connectProducerWithRetry (exponential backoff: 500ms→8s, ~30s timeout)
  └─ Consumer fails fast (no retry at connection level)
```

### 3.3 Data Model

```
PaymentEvent (Kafka payload)
┌─────────────────────────────────────────────┐
│ payment_id  string  UUID v4 (required)      │
│ status      string  pending|confirmed|failed│
│                     |refunded               │
│ amount      float64 > 0                     │
│ currency    string  ISO 4217, 3 chars       │
│ description string  max 255, printable ASCII│
│ timestamp   string  RFC 3339 (≤5 min skew)  │
└─────────────────────────────────────────────┘

PaymentStatus (Redis Hash "payment:<payment_id>")
┌─────────────────────────────────────────────┐
│ payment_id  string                          │
│ status      string                          │
│ updated_at  string  RFC 3339                │
└─────────────────────────────────────────────┘

PaymentHistory (DynamoDB "payment_history")
┌─────────────────────────────────────────────┐
│ payment_id   string  PK (Hash)              │
│ timestamp    string  SK (Range)             │
│ status       string                         │
│ amount       float64                        │
│ currency     string                         │
│ description  string                         │
│ processed_at time.Time                      │
│ trace_id     string  (OTel trace)           │
└─────────────────────────────────────────────┘
  Billing: PAY_PER_REQUEST
```

---

## 4. All Architectural Decisions Made

### ADR-001: Kafka over ZooKeeper (KRaft mode)
- **Decision**: Single-node Kafka with KRaft (no ZooKeeper)
- **Rationale**: Simpler deployment, no external dependency on ZK
- **Trade-off**: Limited to single-broker in development; production would need multi-broker KRaft

### ADR-002: DynamoDB via AWS SDK v2 with Floci emulator
- **Decision**: Use AWS SDK v2 DynamoDB with conditional writes; local Floci container for dev
- **Rationale**: Production-grade SDK; Floci provides AWS-compatible local endpoint
- **Trade-off**: AWS SDK adds dependency weight; Floci is not 100% feature-complete

### ADR-003: Static credentials for local DynamoDB
- **Decision**: `isLocalEndpoint()` helper detects local/floci/localstack endpoints and uses `credentials.NewStaticCredentialsProvider("test", "test", "")`
- **Rationale**: Avoids EC2 IMDS lookup failures in non-EC2 environments
- **Trade-off**: Duplicated logic in consumer and UI cmd packages; acceptable for dev

### ADR-004: Auto-create DynamoDB table
- **Decision**: `history.EnsureTable()` called at startup in both consumer and UI
- **Rationale**: Zero-config local development; table created if absent
- **Trade-off**: Extra startup dependency; could mask provisioning issues in production

### ADR-005: Idempotency via Redis SETNX
- **Decision**: Redis key `idempotency:<payment_id>` with TTL; best-effort marking
- **Rationale**: Fast, distributed, TTL-based expiry
- **Trade-off**: Not transactional; window between check and mark could allow duplicates

### ADR-006: Retry with exponential backoff + jitter
- **Decision**: `retry.Handler` with delays: 1x, 3x, 9x base (not binary); ±25% jitter
- **Rationale**: 3x multiplier spreads retries faster than pure exponential; jitter prevents thundering herd
- **Trade-off**: Not configurable per-operation; fixed per-service config

### ADR-007: DLQ with enriched headers
- **Decision**: DLQ preserves original message + adds headers: original_topic, original_partition, original_offset, error_count, last_error, trace_id
- **Rationale**: Full traceability for DLQ reprocessing

### ADR-008: Consumer worker pool via semaphore channel
- **Decision**: `semaphore chan struct{}` of size `workerCount` (default 10)
- **Rationale**: Simple bounded concurrency without external library
- **Trade-off**: No dynamic scaling; fixed pool size at startup

### ADR-009: UI EventBus via Redis Pub/Sub + in-memory fan-out
- **Decision**: Consumer publishes to Redis Pub/Sub; UI subscribes and fans out to SSE clients
- **Rationale**: Decouples consumer from UI; Redis Pub/Sub is simple and well-understood
- **Trade-off**: Redis Pub/Sub is fire-and-forget (no message persistence); slow SSE subscribers drop events

### ADR-010: OpenTelemetry for observability
- **Decision**: OTLP gRPC exporter for traces and metrics; AlwaysSample() sampler; batch processing
- **Rationale**: Vendor-neutral, industry-standard observability
- **Trade-off**: AlwaysSample() generates high volume of traces (~100% sampling)

### ADR-011: Viper configuration
- **Decision**: Viper with YAML config file + environment variable override; automatic env binding
- **Rationale**: Flexible, hierarchical config; standard Go approach
- **Trade-off**: Viper adds dependency; env-to-config mapping can be error-prone

### ADR-012: Producer flag parsing with subcommand stripping
- **Decision**: `parseFlags()` strips `"publish"` positional arg before calling `flag.Parse()`
- **Rationale**: Docker Compose command passes `["publish", "--brokers", "kafka:9092", ...]`; without stripping, `flag.Parse()` stops at the first non-flag arg
- **Trade-off**: Non-standard; goes against Go's `flag` conventions but solves Docker Compose interop

### ADR-013: Producer retry on Kafka connection
- **Decision**: `connectProducerWithRetry()` with exponential backoff 500ms→8s, ~30s total timeout; respects context cancellation
- **Rationale**: Producer container may start before Kafka is ready; retry is the only reliable startup strategy
- **Trade-off**: Blocks startup for up to 30s; simple but not configurable

### ADR-014: Dockerfiles updated to golang:1.26-alpine
- **Decision**: All four Dockerfiles use `golang:1.26-alpine` as builder; alpine:3.19 for runtime
- **Rationale**: Go 1.26 required (from go.mod); Alpine minimizes image size
- **Trade-off**: CGO disabled; no C extensions

### ADR-015: Binary files removed from project root
- **Decision**: Compiled `./consumer`, `./producer`, `./ui` binaries deleted from repo
- **Rationale**: Build artifacts should not be versioned; binaries go to `bin/` via Makefile
- **Trade-off**: Requires `go build` before running locally; CI must build explicitly

### ADR-016: AWS env vars added to Compose services
- **Decision**: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION` added to consumer and UI services in docker-compose-events.yml
- **Rationale**: Required for local DynamoDB (Floci) authentication; static credentials to bypass IMDS

---

## 5. Dependencies

### Go Runtime Dependencies

| Dependency | Version | Purpose |
|---|---|---|
| `github.com/IBM/sarama` | v1.49.0 | Kafka producer/consumer |
| `github.com/aws/aws-sdk-go-v2` | v1.41.7 | AWS SDK core |
| `github.com/aws/aws-sdk-go-v2/config` | v1.32.18 | AWS config loader |
| `github.com/aws/aws-sdk-go-v2/credentials` | v1.19.17 | Static credentials for local DynamoDB |
| `github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue` | v1.20.40 | DynamoDB marshaling |
| `github.com/aws/aws-sdk-go-v2/service/dynamodb` | v1.57.4 | DynamoDB client |
| `github.com/go-playground/validator/v10` | v10.30.2 | Payload validation |
| `github.com/google/uuid` | v1.6.0 | UUID generation |
| `github.com/redis/go-redis/v9` | v9.19.0 | Redis client |
| `github.com/spf13/viper` | v1.21.0 | Configuration management |
| `go.opentelemetry.io/otel` | v1.43.0 | OpenTelemetry API |
| `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc` | v1.43.0 | OTLP metric exporter |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` | v1.43.0 | OTLP trace exporter |
| `go.opentelemetry.io/otel/sdk` | v1.43.0 | OTel SDK |
| `go.opentelemetry.io/otel/sdk/metric` | v1.43.0 | OTel metrics SDK |
| `github.com/stretchr/testify` | v1.11.1 | Test assertions |
| `github.com/alicebob/miniredis/v2` | v2.38.0 | Redis mock for tests |
| `github.com/ollama/ollama` | v0.24.0 | Legacy/exploratory (internal/main.go) |
| `github.com/tmc/langchaingo` | v0.1.14 | Legacy/exploratory (internal/main.go) |

### Infrastructure Dependencies

| Service | Image | Port | Purpose |
|---|---|---|---|
| Kafka | `apache/kafka:latest` | 9092, 9093 | Event stream |
| Redis | `redis:7-alpine` | 6379 | Status, idempotency, event bus |
| Floci (DynamoDB) | `floci/floci:latest` | 4566 | Local DynamoDB-compatible |
| OTel Collector | `otel/opentelemetry-collector-contrib:latest` | 4317, 4318, 8888, 8889 | Telemetry pipeline |

---

## 6. Configuration Reference

| Variable | Default | Services | Description |
|---|---|---|---|
| `KAFKA_BROKERS` | `localhost:9092` | consumer, producer | Kafka broker addresses (comma-sep) |
| `KAFKA_TOPIC` | `payment.events` | consumer, producer | Main events topic |
| `KAFKA_DLQ_TOPIC` | `payment.events.dlq` | consumer | Dead letter queue topic |
| `KAFKA_CONSUMER_GROUP` | `payment-consumer-group` | consumer | Consumer group ID |
| `REDIS_ADDR` | `localhost:6379` | consumer, UI | Redis address |
| `REDIS_PASSWORD` | `` | consumer, UI | Redis password |
| `DYNAMODB_ENDPOINT` | `http://localhost:4566` | consumer, UI | DynamoDB endpoint |
| `DYNAMODB_TABLE` | `payment_history` | consumer, UI | DynamoDB table name |
| `AWS_ACCESS_KEY_ID` | — | consumer, UI | AWS access key (static for local) |
| `AWS_SECRET_ACCESS_KEY` | — | consumer, UI | AWS secret key (static for local) |
| `AWS_REGION` | — | consumer, UI | AWS region (static for local) |
| `WORKER_COUNT` | `10` | consumer | Concurrent message workers |
| `IDEMPOTENCY_TTL_HOURS` | `24` | consumer | Idempotency key TTL |
| `STATUS_TTL_HOURS` | `168` | consumer | Payment status TTL |
| `RETRY_MAX_ATTEMPTS` | `3` | consumer | Max retry attempts |
| `RETRY_BASE_DELAY_MS` | `100` | consumer | Base retry delay (ms) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | consumer | OTLP endpoint |
| `OTEL_SERVICE_NAME` | `payment-consumer` | consumer | Service name for telemetry |
| `GRACEFUL_SHUTDOWN_TIMEOUT` | `30s` | consumer | Shutdown drain timeout |
| `UI_PORT` | `8081` | UI | HTTP server port |
| `UI_EVENT_BUS_BUFFER` | `256` | UI | EventBus channel buffer |
| `UI_READ_TIMEOUT` | `10s` | UI | HTTP read timeout |
| `UI_WRITE_TIMEOUT` | `30s` | UI | HTTP write timeout |
| `server.port` | `8080` | — | Legacy config (config.yaml) |

---

## 7. API Endpoints

### Consumer Service (port 8080)

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/healthz` | Liveness probe — always returns `{"status":"ok"}` |
| `GET` | `/readyz` | Readiness probe — checks Redis + DynamoDB; returns 503 if down |

### UI Service (port 8081)

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/` | Static files (index.html, app.js, style.css) |
| `GET` | `/healthz` | Health check — pings Redis |
| `GET` | `/api/events` | Server-Sent Events stream for real-time payment events |
| `GET` | `/api/payments` | List payments from Redis; optional `?payment_id=` and `?status=` filters |
| `GET` | `/api/payments/{id}/history` | Payment history from DynamoDB |
| `GET` | `/api/metrics` | Aggregated metrics (total, by_status, success_rate, dlq_count) |

### SSE Event Format

```
event: payment
data: {"payment_id":"...","status":"confirmed","amount":100,"currency":"BRL","description":"...","timestamp":"2026-05-25T12:00:00Z"}

event: heartbeat
data: {}
```

### API Response Formats

**List Payments:**
```json
[
  {"payment_id": "uuid", "status": "confirmed", "updated_at": "2026-05-25T12:00:00Z"}
]
```

**Payment History:**
```json
[
  {
    "payment_id": "uuid",
    "status": "confirmed",
    "amount": 100.0,
    "currency": "BRL",
    "description": "",
    "timestamp": "2026-05-25T12:00:00Z",
    "processed_at": "2026-05-25T12:00:01Z",
    "trace_id": "abc123"
  }
]
```

**Metrics:**
```json
{
  "total_processed": 150,
  "by_status": {"confirmed": 100, "failed": 30, "refunded": 20},
  "success_rate": 66.67,
  "dlq_count": 5
}
```

**Healthz (UI):**
```json
{"status": "ok", "redis": "connected"}
```

---

## 8. Key Changes Summary (This Session)

| Area | Change | Motivation |
|---|---|---|
| **Dockerfiles** | `golang:1.25-alpine` → `golang:1.26-alpine` | Match Go 1.26 in go.mod |
| **Kafka controller** | `KAFKA_CONTROLLER_QUORUM_VOTERS: 0@kafka:9093` → `1@kafka:9093` | Fix KRaft controller config; node ID must match |
| **Binary cleanup** | Removed `./consumer`, `./producer`, `./ui` from project root | Build artifacts should not be versioned |
| **Script** | Created `scripts/run-producer.sh` | Convenience for local producer testing |
| **AWS env vars** | Added `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION` to consumer + UI Compose services | Required for local Floci DynamoDB authentication |
| **Producer flag parsing** | Strip `"publish"` subcommand before `flag.Parse()` | Fix Docker Compose command arg interop |
| **Producer retry** | Added `connectProducerWithRetry()` with exponential backoff | Graceful startup when Kafka is not yet ready |
| **DynamoDB credentials** | `isLocalEndpoint()` helper + static credentials for Floci | Avoid EC2 IMDS lookup failures in dev |
| **DynamoDB auto-create** | `history.EnsureTable()` — DescribeTable + CreateTable + waiter | Zero-config local dev experience |

---

## 9. Risk Assessment

| Risk | Impact | Likelihood | Mitigation |
|---|---|---|---|
| Redis down at startup | High — idempotency/skip/status fail | Medium | Graceful warning; optimistic processing continues |
| DynamoDB auto-create fails | High — service won't start | Low | 30s timeout; clear error message |
| Kafka partition rebalance | Medium — brief processing pause | High (with consumer group) | RoundRobin strategy; graceful session handling |
| EventBus subscriber slow | Low — events dropped | Medium | Bounded channel; log warning on drop |
| DLQ infinite growth | Medium — storage | High | Manual monitoring; no automated cleanup yet |
| AlwaysSample() in production | Medium — high trace volume | Depends on throughput | Should use probabilistic sampling for production |
| Floci not 100% DynamoDB compatible | Medium — conditional writes may differ | Low | Test coverage with miniredis + DynamoDB mocks |

---

## 10. Production Readiness Gaps

1. **No automated DLQ reprocessing** — messages accumulate on `payment.events.dlq`
2. **No circuit breaker** — consumer retries indefinitely on Redis/DynamoDB failure
3. **No rate limiting on API endpoints** — SSE semaphore (100) is the only throttle
4. **No authentication/authorization** — endpoints are fully open
5. **No persisted configuration for Floci** — `FLOCI_STORAGE_MODE: memory` means data loss on restart
6. **Legacy code** — `internal/main.go` (Ollama) and root `main.go` (old HTTP server) are not part of the core flow but add build weight
7. **No producer healthcheck** — producer runs once and exits; no monitoring of publish success
8. **Single Kafka broker** — no HA; `KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1`
9. **No secrets management** — AWS credentials are plaintext in docker-compose

---

## 11. UI Frontend Architecture

### 11.1 Overview

The UI frontend is a **vanilla single-page application (SPA)** served by the Go HTTP server at port 8081. All assets are embedded into the Go binary via `//go:embed static/*` and served as static files:

- `index.html` — Dashboard layout, semantic HTML structure
- `style.css` — Dark-themed, responsive design (~1086 lines)
- `app.js` — Application logic: SSE, REST fetches, DOM rendering (~380 lines)

A secondary inline `<script>` block in `index.html` handles non-logic DOM enhancements (icon mapping on metric cards, feed timestamp extraction, payment row counter via `MutationObserver`).

### 11.2 Static File Embedding

```go
//go:embed static/*
var staticFiles embed.FS

staticFS, err := fs.Sub(staticFiles, "static")
// serves index.html at GET /
```

**File**: `internal/ui/server.go:17-18`, `internal/ui/server.go:37-44`

### 11.3 SSE Real-Time Event Flow

```
Backend (Go)                            Frontend (Browser)
┌─────────────────┐                    ┌────────────────────┐
│  HandleSSE()     │  event: heartbeat  │  EventSource       │
│  Ticker 30s      │ ─────────────────▶ │  /api/events       │
│                  │                    │                    │
│  EventBus        │  event: payment    │  addEventListener  │
│  (Redis Pub/Sub) │ ─────────────────▶ │  ('payment', fn)   │
└─────────────────┘                    └────────────────────┘
```

**Heartbeat**: Server sends `event: heartbeat\ndata: {}\n\n` every 30 seconds via `time.NewTicker(30 * time.Second)` — `handlers.go:102,120-122`.

**Connection limit**: Semaphore channel of 100 slots (`var sseSemaphore = make(chan struct{}, 100)`) — `handlers.go:24`.

### 11.4 Data Flow: REST API

| Endpoint | Purpose | Source |
|---|---|---|
| `GET /api/payments` | List all payments (optional `?payment_id=` and `?status=` filters) | Redis SCAN + HGETALL |
| `GET /api/payments/{id}/history` | Full history of a payment | DynamoDB Query |
| `GET /api/metrics` | Aggregated metrics (total, by_status, success_rate, dlq_count) | Redis SCAN |
| `GET /healthz` | Health check (pings Redis) | Redis PING |

### 11.5 Portuguese-Brazil Localization (ADR-017)

**Decision date**: 2026-05-25

All static UI files were translated to pt-BR. This is a **full localization without an i18n framework** — all strings are hardcoded in Portuguese in the source files.

**Changes applied**:

| File | Change |
|---|---|
| `index.html` | `<html lang="en">` → `<html lang="pt-BR">` |
| `index.html` | Title "Payment Monitor" → "Monitor de Pagamentos" |
| `index.html` | Subtitle "Real-time Payment Processing" → "Processamento de Pagamentos em Tempo Real" |
| `index.html` | "● Disconnected" → "● Desconectado" (initial state) |
| `index.html` | "Payment ID" → "ID do Pagamento" |
| `index.html` | "Status" → "Status" (unchanged, same word in pt-BR) |
| `index.html` | "All Statuses" → "Todos os Status" |
| `index.html` | "Pending" → "Pendente" |
| `index.html` | "Confirmed" → "Confirmado" |
| `index.html` | "Failed" → "Falhou" |
| `index.html` | "Refunded" → "Reembolsado" |
| `index.html` | "Live Event Feed" → "Feed de Eventos" |
| `index.html` | "Real-time" → "Tempo Real" |
| `index.html` | "Payments" → "Pagamentos" |
| `index.html` | "Search by ID..." → "Buscar por ID..." |
| `index.html` | "Updated At" → "Atualizado Em" |
| `index.html` | "Actions" → "Ações" |
| `index.html` | "View History" → "Ver Histórico" |
| `index.html` | "Payment History" → "Histórico do Pagamento" |
| `index.html` | Column headers: "Timestamp"→"Timestamp", "Status"→"Status", "Amount"→"Valor", "Currency"→"Moeda", "Description"→"Descrição", "Processed At"→"Processado Em", "Trace ID"→"ID do Traço" |
| `index.html` | "No payments found." → "Nenhum pagamento encontrado." |
| `index.html` | "No history records found." → "Nenhum registro de histórico encontrado." |
| `index.html` | "Loading..." → "Carregando..." |
| `index.html` | "Close" → "Fechar" |
| `app.js` | `'● Connected'` → `'Conectado'` with emoji |
| `app.js` | `'● Disconnected'` → `'Desconectado'` with emoji |
| `app.js` | Status display: `pending` → `Pendente`, `confirmed` → `Confirmado`, `failed` → `Falhou`, `refunded` → `Reembolsado` |

**Rationale**: The system is developed for a Portuguese-speaking team in Brazil. Adding an i18n framework (go-i18n, react-i18next, etc.) would introduce unnecessary complexity for a development tool used by a single-language team.

**Trade-off**: Adding support for additional languages in the future would require extracting all strings from the static files into an i18n system. This is acceptable given the tool's status as a development UI.

**Emoji simplification**: Unicode escape sequences (`\u{1F7E2}`) were replaced with literal emoji characters (`🟢`) for readability and maintainability.

### 11.6 SSE Connection Status Reliability Fix (ADR-018)

**Problem**: The connection status indicator (`#connection-status`) never changed from "Disconnected" because the frontend relied solely on `EventSource.onopen` and `EventSource.onerror` events, which are unreliable in certain scenarios:
- Browser `onerror` fires immediately, but auto-reconnect `onopen` may not fire if the underlying TCP connection reestablishment is not detected as a new "open" event
- Some browsers fire `onerror` and never fire `onopen` again for the reconnected connection
- Silent connection drops (proxy timeout, half-open TCP) are not detected by `EventSource`

**Original unreliable pattern** (`app.js` lines 26-61):
```javascript
eventSource.onopen = function () {
    updateConnectionStatus(true);
};
eventSource.onerror = function () {
    updateConnectionStatus(false);
};
```

**Fix — Heartbeat Timeout Pattern (45s)**:

The fix adds a **client-side heartbeat timeout** that monitors incoming data on the SSE connection. Instead of relying on `onopen`/`onerror`, the connection status is determined by whether data (heartbeat or payment event) has been received within the last 45 seconds.

**Implementation** (`app.js`):

```javascript
// Heartbeat watchdog timer
let heartbeatTimer = null;

function scheduleHeartbeatTimeout() {
    if (heartbeatTimer) clearTimeout(heartbeatTimer);
    heartbeatTimer = setTimeout(function () {
        updateConnectionStatus(false);
    }, 45000); // 45s — 1.5x the server heartbeat interval (30s)
}

function resetHeartbeatTimeout() {
    updateConnectionStatus(true);
    scheduleHeartbeatTimeout();
}
```

The `resetHeartbeatTimeout()` function is called on:
- `eventSource.onopen` — initial connection established
- `eventSource.addEventListener('payment', ...)` — any payment event received
- `eventSource.addEventListener('heartbeat', ...)` — heartbeat received

The `scheduleHeartbeatTimeout()` sets a 45-second timer. If no data arrives within 45 seconds, the connection is considered dead and the status changes to "Desconectado". If data arrives, the timer resets.

**Why 45s?** The server sends heartbeats every 30s. Setting the timeout to 45s (1.5× interval) provides tolerance for:
- Network jitter (up to ~10s delay)
- Server-side GC pauses
- Browser EventSource reconnection delay (~3s)

**Status value mapping** (frontend display):

| Internal value | Display (pt-BR) | Emoji |
|---|---|---|
| `pending` | Pendente | 🟡 |
| `confirmed` | Confirmado | 🟢 |
| `failed` | Falhou | 🔴 |
| `refunded` | Reembolsado | 🔵 |

**Edge cases covered**:

1. **Initial connection never established**: No heartbeat fires → `updateConnectionStatus(false)` remains → status stays "Desconectado" (correct)
2. **Connection drops silently**: Last heartbeat times out after 45s → status changes to "Desconectado" → browser auto-reconnects → next event resets timeout → status changes to "Conectado"
3. **Browser tab backgrounded**: `setTimeout` is throttled by browsers to ≥1s in background tabs, but the factor is still small relative to 45s
4. **Server restart**: Heartbeats stop → 45s timeout expires → status "Desconectado" → server restarts → `onopen` fires → status "Conectado"
5. **Rapid reconnect**: `clearTimeout` in `scheduleHeartbeatTimeout()` prevents stale timer from firing after reconnect

---

*Document generated by Architect agent during architecture review session on 2026-05-25.*
