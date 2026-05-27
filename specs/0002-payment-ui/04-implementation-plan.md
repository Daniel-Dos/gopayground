# 04 — Plano de Implementação

## Ordem de Implementação

```
 1. Config (extensão)   → Adicionar configurações da UI
 2. Event Bus           → Mecanismo de publicação/assinatura de eventos
 3. Static files        → HTML, CSS, JS da dashboard
 4. Handlers HTTP       → Endpoints REST + SSE
 5. Server              → Montagem das rotas, inicialização HTTP
 6. Main                → Entry point, DI, inicialização
 7. docker-compose      → Adicionar serviço da UI
 8. Makefile            → Atualizar com comandos da UI
 9. Testes              → Unitários e integração
```

---

## Passo 1 — Config (Extensão)

**Arquivos**: `config/config.go`, `config/config_test.go`

### O que implementar

Adicionar à struct `Config` existente:

```go
type Config struct {
    // ... config existente do consumer ...

    // UI-specific config
    UIPort            string        `envconfig:"UI_PORT" default:"8081"`
    UIEventBusBuffer  int           `envconfig:"UI_EVENT_BUS_BUFFER" default:"256"`
    UIReadTimeout     time.Duration `envconfig:"UI_READ_TIMEOUT" default:"10s"`
    UIWriteTimeout    time.Duration `envconfig:"UI_WRITE_TIMEOUT" default:"30s"`
}
```

### Testes

- [ ] Valores default aplicados quando env vars não estão setadas.
- [ ] Override via variáveis de ambiente.

---

## Passo 2 — Event Bus

**Arquivos**: `internal/ui/events.go`, `internal/ui/events_test.go`

### O que implementar

```go
// EventBus distribui eventos do consumer para a UI via Redis Pub/Sub.
type EventBus struct {
    redis    *redis.Client
    channel  string
    subscribers map[string]chan *models.PaymentEvent
    mu        sync.RWMutex
    logger    *slog.Logger
}

func NewEventBus(rdb *redis.Client, channel string, logger *slog.Logger) *EventBus {
    eb := &EventBus{
        redis:       rdb,
        channel:     channel,
        subscribers: make(map[string]chan *models.PaymentEvent),
        logger:      logger,
    }
    go eb.listenRedis()
    return eb
}

// Publish publica um evento no canal Redis.
func (eb *EventBus) Publish(ctx context.Context, event *models.PaymentEvent) error {
    data, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("marshal event: %w", err)
    }
    return eb.redis.Publish(ctx, eb.channel, string(data)).Err()
}

// Subscribe registra um subscriber e retorna um canal de eventos.
func (eb *EventBus) Subscribe() (<-chan *models.PaymentEvent, func()) {
    eb.mu.Lock()
    defer eb.mu.Unlock()

    id := uuid.New().String()
    ch := make(chan *models.PaymentEvent, 64)
    eb.subscribers[id] = ch

    unsubscribe := func() {
        eb.mu.Lock()
        delete(eb.subscribers, id)
        close(ch)
        eb.mu.Unlock()
    }

    return ch, unsubscribe
}

// listenRedis escuta o canal Redis e distribui para subscribers locais.
func (eb *EventBus) listenRedis() {
    pubsub := eb.redis.Subscribe(context.Background(), eb.channel)
    defer pubsub.Close()

    ch := pubsub.Channel()
    for msg := range ch {
        var event models.PaymentEvent
        if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
            eb.logger.Error("failed to unmarshal event from Redis", "error", err)
            continue
        }

        eb.mu.RLock()
        for id, sub := range eb.subscribers {
            select {
            case sub <- &event:
            default:
                // Subscriber lento, descarta evento
                eb.logger.Warn("dropping event for slow subscriber", "subscriber_id", id)
            }
        }
        eb.mu.RUnlock()
    }
}

// Close finaliza o EventBus.
func (eb *EventBus) Close() {
    eb.mu.Lock()
    defer eb.mu.Unlock()
    for id, ch := range eb.subscribers {
        close(ch)
        delete(eb.subscribers, id)
    }
}
```

**Alternativa para integração em processo único**: Se o consumer e a UI
rodarem no mesmo processo, usar um canal Go diretamente (sem Redis Pub/Sub).
A interface deve ser a mesma para permitir ambas as estratégias.

### Testes

- [ ] Publish distribui evento para subscribers ativos.
- [ ] Subscribe retorna canal que recebe eventos.
- [ ] Unsubscribe remove subscriber e fecha canal.
- [ ] Subscriber lento não bloqueia Publish.
- [ ] Múltiplos subscribers recebem o mesmo evento.
- [ ] Close limpa todos os subscribers.

---

## Passo 3 — Static Files

**Arquivos**: `internal/ui/static/index.html`, `internal/ui/static/app.js`, `internal/ui/static/style.css`

### index.html

Criar conforme design da seção 03. Estrutura semântica com seções para:
- Header com título e status da conexão.
- Seção de métricas.
- Seção de filtros.
- Feed em tempo real.
- Tabela de pagamentos.
- Modal de histórico.

### app.js

Implementar:

```javascript
// 1. Conexão SSE
const eventSource = new EventSource('/api/events');

eventSource.addEventListener('payment', (e) => {
    const event = JSON.parse(e.data);
    addToFeed(event);
    updatePayment(event);
    refreshMetrics();
});

eventSource.addEventListener('heartbeat', () => {
    updateConnectionStatus(true);
});

eventSource.onerror = () => {
    updateConnectionStatus(false);
};

// 2. Load inicial
async function loadInitialData() {
    const [payments, metrics] = await Promise.all([
        fetch('/api/payments').then(r => r.json()),
        fetch('/api/metrics').then(r => r.json()),
    ]);
    renderPaymentsTable(payments);
    renderMetrics(metrics);
}

// 3. Filtros
document.getElementById('filter-payment-id').addEventListener('input', applyFilters);
document.getElementById('filter-status').addEventListener('change', applyFilters);

async function applyFilters() {
    const paymentId = document.getElementById('filter-payment-id').value;
    const status = document.getElementById('filter-status').value;
    const params = new URLSearchParams();
    if (paymentId) params.set('payment_id', paymentId);
    if (status) params.set('status', status);
    const payments = await fetch(`/api/payments?${params}`).then(r => r.json());
    renderPaymentsTable(payments);
}

// 4. Histórico
async function showHistory(paymentId) {
    document.getElementById('modal-payment-id').textContent =
        `History: ${paymentId}`;
    const history = await fetch(`/api/payments/${paymentId}/history`)
        .then(r => r.json());
    renderHistoryTable(history);
    document.getElementById('history-modal').classList.remove('hidden');
}

// 5. Render helpers: addToFeed, renderPaymentsTable, renderMetrics,
//    renderHistoryTable, updateConnectionStatus
```

### style.css

Diretrizes:
- CSS custom properties para cores (tema claro).
- Layout flexbox/responsivo.
- Max-width 1200px centralizado.
- Badges com `border-radius` para status.
- Feed com altura máxima 300px e overflow-y scroll.
- Modal com position fixed, overlay semi-transparente.
- Transições suaves para abertura/fechamento do modal.

---

## Passo 4 — Handlers HTTP

**Arquivos**: `internal/ui/handlers.go`, `internal/ui/handlers_test.go`

### O que implementar

```go
type Handlers struct {
    redis    *redis.Client
    dynamo   *dynamodb.Client
    dynamoTbl string
    eventBus *EventBus
    logger   *slog.Logger
}

// handleSSE: transmite eventos em tempo real
func (h *Handlers) handleSSE(w http.ResponseWriter, r *http.Request)

// handleListPayments: lista pagamentos do Redis (com filtros)
func (h *Handlers) handleListPayments(w http.ResponseWriter, r *http.Request)

// handlePaymentHistory: retorna histórico do DynamoDB
func (h *Handlers) handlePaymentHistory(w http.ResponseWriter, r *http.Request)

// handleMetrics: retorna métricas agregadas
func (h *Handlers) handleMetrics(w http.ResponseWriter, r *http.Request)

// handleHealth: health check
func (h *Handlers) handleHealth(w http.ResponseWriter, r *http.Request)
```

### Detalhamento dos Handlers

#### handleListPayments

```go
func (h *Handlers) handleListPayments(w http.ResponseWriter, r *http.Request) {
    // 1. Parse query params: payment_id, status
    filterID := r.URL.Query().Get("payment_id")
    filterStatus := r.URL.Query().Get("status")

    // 2. SCAN Redis para chaves payment:*
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()

    var cursor uint64
    var payments []models.PaymentStatus

    for {
        keys, nextCursor, err := h.redis.Scan(ctx, cursor, "payment:*", 100).Result()
        if err != nil {
            h.logger.ErrorContext(ctx, "redis scan error", "error", err)
            writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "redis scan failed"})
            return
        }

        for _, key := range keys {
            paymentID := strings.TrimPrefix(key, "payment:")

            // Filtro por payment_id
            if filterID != "" && !strings.Contains(paymentID, filterID) {
                continue
            }

            fields, err := h.redis.HGetAll(ctx, key).Result()
            if err != nil {
                h.logger.WarnContext(ctx, "redis hgetall error", "key", key, "error", err)
                continue
            }

            status := fields["status"]
            // Filtro por status
            if filterStatus != "" && status != filterStatus {
                continue
            }

            payments = append(payments, models.PaymentStatus{
                PaymentID: paymentID,
                Status:    status,
                UpdatedAt: fields["updated_at"],
            })
        }

        if nextCursor == 0 {
            break
        }
        cursor = nextCursor
    }

    writeJSON(w, http.StatusOK, payments)
}
```

#### handlePaymentHistory

```go
func (h *Handlers) handlePaymentHistory(w http.ResponseWriter, r *http.Request) {
    paymentID := r.PathValue("id")
    if paymentID == "" {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payment_id required"})
        return
    }

    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()

    var history []models.PaymentHistory

    result, err := h.dynamo.Query(ctx, &dynamodb.QueryInput{
        TableName: aws.String(h.dynamoTbl),
        KeyConditions: map[string]types.Condition{
            "payment_id": {
                ComparisonOperator: types.ComparisonOperatorEq,
                AttributeValueList: []types.AttributeValue{
                    &types.AttributeValueMemberS{Value: paymentID},
                },
            },
        },
    })
    if err != nil {
        h.logger.ErrorContext(ctx, "dynamodb query error", "payment_id", paymentID, "error", err)
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "dynamodb query failed"})
        return
    }

    for _, item := range result.Items {
        var h models.PaymentHistory
        if err := dynamodbav.UnmarshalMap(item, &h); err != nil {
            h.logger.WarnContext(ctx, "dynamodb unmarshal error", "payment_id", paymentID, "error", err)
            continue
        }
        history = append(history, h)
    }

    // Ordenar por timestamp ascendente
    sort.Slice(history, func(i, j int) bool {
        return history[i].Timestamp < history[j].Timestamp
    })

    writeJSON(w, http.StatusOK, history)
}
```

#### handleMetrics

```go
func (h *Handlers) handleMetrics(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()

    metrics := Metrics{
        ByStatus: make(map[string]int),
    }

    var cursor uint64
    for {
        keys, nextCursor, err := h.redis.Scan(ctx, cursor, "payment:*", 100).Result()
        if err != nil {
            break
        }
        for _, key := range keys {
            metrics.TotalProcessed++
            status, err := h.redis.HGet(ctx, key, "status").Result()
            if err == nil {
                metrics.ByStatus[status]++
            }
        }
        if nextCursor == 0 {
            break
        }
        cursor = nextCursor
    }

    total := metrics.ByStatus["confirmed"] + metrics.ByStatus["failed"] + metrics.ByStatus["refunded"]
    if total > 0 {
        metrics.SuccessRate = float64(metrics.ByStatus["confirmed"]) / float64(total) * 100
    }

    writeJSON(w, http.StatusOK, metrics)
}
```

#### handleHealth

```go
func (h *Handlers) handleHealth(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
    defer cancel()

    // Verifica Redis
    if err := h.redis.Ping(ctx).Err(); err != nil {
        writeJSON(w, http.StatusServiceUnavailable, map[string]string{
            "status": "unhealthy",
            "redis":  "down",
        })
        return
    }

    writeJSON(w, http.StatusOK, map[string]string{
        "status": "ok",
        "redis":  "connected",
    })
}
```

### Testes

- [ ] `handleListPayments` retorna lista vazia quando não há chaves.
- [ ] `handleListPayments` filtra corretamente por `payment_id`.
- [ ] `handleListPayments` filtra corretamente por `status`.
- [ ] `handleListPayments` retorna 500 se Redis falha.
- [ ] `handlePaymentHistory` retorna histórico para payment_id válido.
- [ ] `handlePaymentHistory` retorna array vazio se payment_id não existe.
- [ ] `handlePaymentHistory` retorna 400 se payment_id vazio.
- [ ] `handleMetrics` retorna métricas corretas.
- [ ] `handleHealth` retorna 200 quando Redis está ok.
- [ ] `handleHealth` retorna 503 quando Redis está down.
- [ ] SSE handler transmite eventos e heartbeat.

---

## Passo 5 — Server

**Arquivos**: `internal/ui/server.go`, `internal/ui/server_test.go`

### O que implementar

```go
type Server struct {
    httpServer *http.Server
    handlers   *Handlers
    eventBus   *EventBus
    logger     *slog.Logger
}

func NewServer(cfg Config, rdb *redis.Client, dynamoClient *dynamodb.Client, logger *slog.Logger) *Server {
    eventBus := NewEventBus(rdb, "payment:events", logger)

    handlers := &Handlers{
        redis:     rdb,
        dynamo:    dynamoClient,
        dynamoTbl: cfg.DynamoDBTable,
        eventBus:  eventBus,
        logger:    logger,
    }

    mux := http.NewServeMux()

    // Static files
    staticFS, _ := fs.Sub(staticFiles, "static")
    mux.Handle("GET /", http.FileServer(http.FS(staticFS)))

    // API
    mux.HandleFunc("GET /api/events", handlers.handleSSE)
    mux.HandleFunc("GET /api/payments", handlers.handleListPayments)
    mux.HandleFunc("GET /api/payments/{id}/history", handlers.handlePaymentHistory)
    mux.HandleFunc("GET /api/metrics", handlers.handleMetrics)
    mux.HandleFunc("GET /healthz", handlers.handleHealth)

    httpServer := &http.Server{
        Addr:         ":" + cfg.UIPort,
        Handler:      middleware(mux, logger),
        ReadTimeout:  cfg.UIReadTimeout,
        WriteTimeout: cfg.UIWriteTimeout,
    }

    return &Server{
        httpServer: httpServer,
        eventBus:   eventBus,
        logger:     logger,
    }
}

func (s *Server) Start() error {
    s.logger.Info("starting UI server", "port", s.httpServer.Addr)
    return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
    s.eventBus.Close()
    return s.httpServer.Shutdown(ctx)
}

// middleware aplica logging e recovery
func middleware(next http.Handler, logger *slog.Logger) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}
        next.ServeHTTP(wrapped, r)
        logger.Info("request",
            "method", r.Method,
            "path", r.URL.Path,
            "status", wrapped.status,
            "duration", time.Since(start).String(),
        )
    })
}
```

### Testes

- [ ] Server inicializa rotas corretamente.
- [ ] Root path serve index.html.
- [ ] Paths desconhecidos retornam 404.
- [ ] Middleware de logging não afeta resposta.
- [ ] Graceful shutdown finaliza conexões abertas.

---

## Passo 6 — Main

**Arquivos**: `cmd/ui/main.go`

### O que implementar

```go
package main

import (
    "context"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/redis/go-redis/v9"

    "github.com/seuorg/payment-consumer/config"
    "github.com/seuorg/payment-consumer/internal/ui"
)

//go:embed static/*
var staticFiles embed.FS

func main() {
    // 1. Config
    cfg := config.Load()

    // 2. Logger
    logger := slog.New(slog.NewJSONHandler(os.Stdout,
        &slog.HandlerOptions{Level: slog.LevelInfo}))

    // 3. Redis
    rdb := redis.NewClient(&redis.Options{
        Addr:     cfg.RedisAddr,
        Password: cfg.RedisPassword,
    })
    defer rdb.Close()

    // 4. DynamoDB
    awsCfg, err := config.LoadDefaultConfig(context.Background())
    if err != nil {
        logger.Error("failed to load AWS config", "error", err)
        os.Exit(1)
    }
    dynamoClient := dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
        o.BaseEndpoint = aws.String(cfg.DynamoDBEndpoint)
    })

    // 5. UI Server
    server := ui.NewServer(cfg, rdb, dynamoClient, logger)

    // 6. Graceful shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        <-sigCh
        logger.Info("shutting down UI server...")
        ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer cancel()
        if err := server.Shutdown(ctx); err != nil {
            logger.Error("shutdown error", "error", err)
        }
    }()

    // 7. Start
    if err := server.Start(); err != nil && err != http.ErrServerClosed {
        logger.Error("server error", "error", err)
        os.Exit(1)
    }
}
```

---

## Passo 7 — Docker Compose

**Arquivo**: `docker-compose.yml`

Adicionar serviço:

```yaml
services:
  # ... serviços existentes (kafka, redis, dynamodb, consumer) ...

  payment-ui:
    build:
      context: .
      dockerfile: Dockerfile.ui
    ports:
      - "8081:8081"
    environment:
      - UI_PORT=8081
      - REDIS_ADDR=redis:6379
      - DYNAMODB_ENDPOINT=http://dynamodb:8000
      - DYNAMODB_TABLE=payment_history
    depends_on:
      - redis
      - dynamodb
```

Criar `Dockerfile.ui`:

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Copy docs into static/ for embedding in the Go binary
RUN mkdir -p internal/ui/static/docs && cp -r docs/* internal/ui/static/docs/ 2>/dev/null || true

RUN go build -o /app/ui ./cmd/ui

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/ui /app/ui
EXPOSE 8081
CMD ["/app/ui"]
```

**Nota**: A documentação é copiada para `internal/ui/static/docs/` antes do
build Go, garantindo que seja incluída pelo `//go:embed static/*`.

---

## Passo 8 — Makefile

Adicionar comandos:

```makefile
.PHONY: run-ui build-ui copy-docs clean

copy-docs:
	@mkdir -p internal/ui/static/docs
	@cp -r docs/* internal/ui/static/docs/ 2>/dev/null || true

build-ui: copy-docs
	go build -o bin/ui ./cmd/ui

run-ui: copy-docs
	go run ./cmd/ui

clean:
	rm -rf bin/ internal/ui/static/docs/
```

**Nota**: O target `copy-docs` deve ser executado antes de qualquer build
ou execução da UI. O target `clean` remove o diretório copiado para evitar
arquivos residuais.

---

## Passo 9 — Testes

### Testes Unitários

| Pacote            | O que testar                                              |
|-------------------|-----------------------------------------------------------|
| `internal/ui`     | EventBus: publish, subscribe, unsubscribe, close          |
| `internal/ui`     | Handlers: cada handler com request/response HTTP          |
| `internal/ui`     | Server: roteamento, middleware, graceful shutdown         |
| `config`          | UI config values (defaults, env override)                 |

### Testes de Integração

| Teste                                            | Descrição                                  |
|--------------------------------------------------|--------------------------------------------|
| Dashboard carrega no browser                     | GET `/` retorna 200 + HTML                 |
| SSE conecta e recebe eventos                     | Abrir SSE e publicar evento via EventBus   |
| Lista de pagamentos reflete dados do Redis       | Inserir dados no Redis, verificar resposta |
| Histórico de pagamento carrega do DynamoDB       | Inserir dados no DynamoDB, verificar modal |
| Métricas refletem contagens corretas             | Verificar response da API de métricas      |
| Filtros funcionam                                | Testar query params                        |
| Health check retorna 200 quando Redis está ok    | `/healthz` deve retornar 200               |

---

## Dependências

Nenhuma nova dependência Go além das já existentes no módulo:

```
github.com/redis/go-redis/v9
github.com/aws/aws-sdk-go-v2
github.com/aws/aws-sdk-go-v2/service/dynamodb
github.com/aws/aws-sdk-go-v2/config
log/slog (stdlib)
embed (stdlib)
net/http (stdlib)
```

---
