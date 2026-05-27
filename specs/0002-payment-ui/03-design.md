# 03 — Design

## Arquitetura Geral

```
┌─────────────────────────────────────────────────────────────────┐
│                         Browser (Cliente)                       │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                    Dashboard HTML/CSS/JS                   │  │
│  │                                                           │  │
│  │  ┌──────────────────┐  ┌────────────────────────────┐     │  │
│  │  │  Live Feed (SSE)  │  │  Payments List (tabela)    │     │  │
│  │  │  ┌──────────────┐ │  │  ID    │ Status │ Updated │     │  │
│  │  │  │ event 1      │ │  │  p1    │ ✅ conf│ 10:30   │     │  │
│  │  │  │ event 2      │ │  │  p2    │ ❌ fail│ 10:31   │     │  │
│  │  │  └──────────────┘ │  │  p3    │ ⏳ pend│ 10:32   │     │  │
│  │  └──────────────────┘  └────────────────────────────┘     │  │
│  │                                                           │  │
│  │  ┌────────────────────────────────────────────────────┐   │  │
│  │  │  Payment History Modal/Panel                       │   │  │
│  │  │  Timestamp │ Status │ Amount │ Currency │ Desc    │   │  │
│  │  └────────────────────────────────────────────────────┘   │  │
│  │                                                           │  │
│  │  ┌────────────────────────────────────────────────────┐   │  │
│  │  │  Metrics: Total: 150 │ Success: 95% │ DLQ: 3      │   │  │
│  │  └────────────────────────────────────────────────────┘   │  │
│  │                                                           │  │
│  │  ┌────────────────────────────────────────────────────┐   │  │
│  │  │  Filters: [payment_id input] [status dropdown]     │   │  │
│  │  └────────────────────────────────────────────────────┘   │  │
│  └───────────────────────────────────────────────────────────┘  │
└────────────────────────────┬────────────────────────────────────┘
                             │ HTTP (mesma origem)
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Go HTTP Server (:8081)                        │
│                                                                  │
│  ┌──────────────────┐  ┌──────────────┐  ┌──────────────────────┐   │
│  │  Static          │  │  SSE Handler │  │  REST API Handlers   │   │
│  │  File Server     │  │  /api/events │  │  /api/payments       │   │
│  │  (index.html,    │  │              │  │  /api/payments/:id/  │   │
│  │   app.js,        │  │              │  │    history           │   │
│  │   style.css,     │  │              │  │  /api/metrics        │   │
│  │   docs/)         │  │              │  │  /healthz            │   │
│  └──────────────┘  └──────┬───────┘  └──────────┬───────────┘   │
│                           │                     │               │
│                           ▼                     ▼               │
│                    ┌──────────────┐     ┌──────────────┐        │
│                    │  Event Bus   │     │  Redis       │        │
│                    │  (in-memory  │     │  Reader      │        │
│                    │   channel)   │     │              │        │
│                    └──────┬───────┘     └──────────────┘        │
│                           │                    │                │
│                           │                    ▼                │
│                           │             ┌──────────────┐        │
│                           │             │  DynamoDB    │        │
│                           │             │  Reader      │        │
│                           │             └──────────────┘        │
└───────────────────────────┼─────────────────────────────────────┘
                            │
                            │ Eventos publicados pelo consumer
                            ▼
                    ┌──────────────┐
                    │  Kafka       │
                    │  Payment     │
                    │  Consumer    │
                    └──────────────┘
```

## Endpoints da API

| Método | Rota                       | Descrição                                    | Formato Resposta                         |
|--------|----------------------------|----------------------------------------------|------------------------------------------|
| GET    | `/`                        | Serve `index.html` (arquivo embutido)        | `text/html`                              |
| GET    | `/api/events`              | SSE stream de eventos em tempo real          | `text/event-stream`                      |
| GET    | `/api/payments`            | Lista todos os pagamentos com status atual   | `application/json` (array)               |
| GET    | `/api/payments/{id}/history`| Histórico completo de um pagamento           | `application/json` (array)               |
| GET    | `/api/metrics`             | Métricas agregadas                           | `application/json` (objeto)              |
| GET    | `/healthz`                 | Health check                                 | `application/json`                       |

### GET /api/payments

Retorna a lista de todos os pagamentos com status atual, lida do Redis.

**Query params**:
- `payment_id` (opcional) — filtra por payment_id (LIKE)
- `status` (opcional) — filtra por status exato

**Resposta**:
```json
[
  {
    "payment_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "status": "confirmed",
    "updated_at": "2026-05-24T10:30:01Z"
  }
]
```

**Implementação**: Usa `SCAN 0 MATCH payment:*` no Redis para listar chaves,
depois `HGETALL` para cada chave.

### GET /api/payments/{id}/history

Retorna o histórico completo de um pagamento, lido do DynamoDB.

**Resposta**:
```json
[
  {
    "payment_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "status": "pending",
    "amount": 150.00,
    "currency": "BRL",
    "description": "Pedido #12345",
    "timestamp": "2026-05-24T10:30:00Z",
    "processed_at": "2026-05-24T10:30:01.123Z",
    "trace_id": "0af7651916cd43dd8448eb211c80319c"
  }
]
```

**Implementação**: Usa `Query` no DynamoDB com `KeyConditionExpression:
payment_id = :id`, ordenado por `timestamp` ascendente.

### GET /api/metrics

Retorna métricas agregadas.

**Resposta**:
```json
{
  "total_processed": 150,
  "by_status": {
    "confirmed": 100,
    "failed": 30,
    "pending": 15,
    "refunded": 5
  },
  "success_rate": 66.67,
  "dlq_count": 3
}
```

**Implementação**:
- `total_processed` e `by_status`: scan no Redis, conta por status.
- `dlq_count`: consulta o offset mais recente do tópico DLQ via Kafka client
  (ou retorna 0 se Kafka não estiver disponível).

### GET /api/events (SSE)

Formato SSE:
```
event: payment
data: {"payment_id":"...","status":"confirmed","amount":150.00,...}

event: heartbeat
data: {}
```

- **heartbeat**: enviado a cada 30s para manter a conexão viva.
- **payment**: enviado quando um novo evento é consumido.

## Componentes Internos

### 1. Event Bus (`internal/ui/events.go`)

O Event Bus é o mecanismo que conecta o consumer Kafka à UI. Quando o consumer
processa um pagamento, ele publica o evento no bus. A UI consome desse bus e
transmite via SSE.

```go
// Publisher is the interface the consumer uses to publish events
type Publisher interface {
    Publish(ctx context.Context, event *models.PaymentEvent) error
}

// Subscriber is the interface the SSE handler uses to receive events
type Subscriber interface {
    Subscribe() (<-chan *models.PaymentEvent, func())
    Close()
}
```

**Implementação**: Canal com buffer configurável (default 256). Se o buffer
estiver cheio, descarta eventos mais antigos (dropping policy — para não
bloquear o consumer).

### 2. HTTP Server (`internal/ui/handler.go`)

Handler HTTP que configura as rotas e serve os arquivos estáticos.

```go
type Server struct {
    mux        *http.ServeMux
    redis      redis.UniversalClient
    dynamo     *dynamodb.Client
    eventBus   *EventBus
    cfg        Config
    logger     *slog.Logger
}
```

### 3. SSE Handler (`internal/ui/events.go`)

```go
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming not supported", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    ch, cancel := s.eventBus.Subscribe()
    defer cancel()

    for {
        select {
        case <-r.Context().Done():
            return
        case event, ok := <-ch:
            if !ok {
                return
            }
            data, _ := json.Marshal(event)
            fmt.Fprintf(w, "event: payment\ndata: %s\n\n", data)
            flusher.Flush()
        case <-time.After(30 * time.Second):
            fmt.Fprintf(w, "event: heartbeat\ndata: {}\n\n")
            flusher.Flush()
        }
    }
}
```

### 4. Static File Embedding (`internal/ui/server.go`)

Todo o conteúdo da pasta `static/` é embutido no binário Go via `//go:embed`:

```go
//go:embed static/*
var staticFiles embed.FS
```

**Documentação**: Os arquivos de documentação (`/docs/*`) são copiados para
`internal/ui/static/docs/` durante o build (via `Makefile` ou `Dockerfile`)
**antes** da compilação. Como a diretiva `//go:embed static/*` captura
recursivamente todo o conteúdo abaixo de `static/`, a documentação é
automaticamente incluída no binário sem necessidade de alterar a diretiva
`embed`.

O servidor HTTP serve os arquivos estáticos via `http.FileServer`, que
também expõe `/docs/`:

```go
staticFS, _ := fs.Sub(staticFiles, "static")
mux.Handle("GET /", http.FileServer(http.FS(staticFS)))
```

## Estrutura de Pastas

```
cmd/
  ui/
    main.go                          # Entry point, DI, inicialização
    main_test.go                     # Teste de integração

internal/
  ui/
    server.go                        # Server struct, construtor, rotas
    server_test.go                   # Testes do servidor
    events.go                        # Event Bus (Publisher/Subscriber)
    events_test.go                   # Testes do Event Bus
    handlers.go                      # Handlers HTTP (payments, history, metrics, sse, health)
    handlers_test.go                 # Testes dos handlers
    static/                          # Arquivos embutidos via embed
      index.html                     # Dashboard HTML
      app.js                         # Lógica JS (SSE, fetch, render)
      style.css                      # Estilos CSS
      docs/                          # Documentação do projeto (copiada de /docs/ no build)
        index.html                   # Página inicial da documentação
        setup.html                   # Guia de setup
        architecture.html            # Arquitetura do sistema
        observability.html           # Observabilidade
        producer-guide.html          # Guia do producer
        features/                    # Documentação de features
        diagrams/                    # Diagramas arquiteturais

config/
  config.go                          # Extensão do config existente (+ UI config)
  config_test.go                     # Testes de config
```

## Configuração

Todas as configurações via variáveis de ambiente (adicionais ao config do consumer):

| Variável                    | Default                     | Descrição                            |
|-----------------------------|-----------------------------|--------------------------------------|
| `UI_PORT`                   | `8081`                      | Porta do servidor HTTP               |
| `UI_EVENT_BUS_BUFFER`       | `256`                       | Tamanho do buffer do Event Bus       |
| `REDIS_ADDR`                | `localhost:6379`            | Endereço Redis (compartilhado)       |
| `REDIS_PASSWORD`            | ``                          | Senha Redis (compartilhado)          |
| `DYNAMODB_ENDPOINT`         | `http://localhost:4566`     | Endpoint DynamoDB (compartilhado)    |
| `DYNAMODB_TABLE`            | `payment_history`           | Tabela DynamoDB (compartilhado)      |
| `UI_READ_TIMEOUT`           | `10s`                       | Timeout de leitura HTTP              |
| `UI_WRITE_TIMEOUT`          | `30s`                       | Timeout de escrita HTTP (SSE longo)  |

---

## Frontend (HTML/CSS/JS)

### index.html

Estrutura da página:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Payment Consumer UI</title>
    <link rel="stylesheet" href="style.css">
</head>
<body>
    <header>
        <h1>Payment Consumer Dashboard</h1>
        <div id="connection-status">🔴 Disconnected</div>
    </header>

    <section id="metrics">
        <!-- Métricas renderizadas pelo JS -->
    </section>

    <section id="filters">
        <input type="text" id="filter-payment-id" placeholder="Filter by Payment ID...">
        <select id="filter-status">
            <option value="">All Statuses</option>
            <option value="pending">Pending</option>
            <option value="confirmed">Confirmed</option>
            <option value="failed">Failed</option>
            <option value="refunded">Refunded</option>
        </select>
    </section>

    <section id="feed">
        <h2>Live Feed</h2>
        <div id="feed-container"><!-- Eventos inseridos aqui --></div>
    </section>

    <section id="payments">
        <h2>Payments</h2>
        <table id="payments-table">
            <thead>
                <tr>
                    <th>Payment ID</th>
                    <th>Status</th>
                    <th>Updated At</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody><!-- Linhas renderizadas pelo JS --></tbody>
        </table>
    </section>

    <!-- Modal de Histórico -->
    <div id="history-modal" class="modal hidden">
        <div class="modal-content">
            <span class="close">&times;</span>
            <h2 id="modal-payment-id">Payment History</h2>
            <table id="history-table">
                <thead>
                    <tr>
                        <th>Timestamp</th>
                        <th>Status</th>
                        <th>Amount</th>
                        <th>Currency</th>
                        <th>Description</th>
                        <th>Processed At</th>
                        <th>Trace ID</th>
                    </tr>
                </thead>
                <tbody><!-- Histórico renderizado pelo JS --></tbody>
            </table>
        </div>
    </div>

    <script src="app.js"></script>
</body>
</html>
```

### app.js — Responsabilidades

1. Conectar ao SSE endpoint (`/api/events`).
2. Receber eventos SSE e:
   - Adicionar ao feed em tempo real (topo da lista).
   - Atualizar métricas.
   - Atualizar a linha correspondente na tabela de pagamentos (ou adicionar nova).
3. Fazer fetch inicial de `/api/payments` e `/api/metrics` para popular a UI.
4. Filtrar a tabela de pagamentos conforme inputs do usuário.
5. Ao clicar em "View History" em um pagamento:
   - Fazer fetch de `/api/payments/{id}/history`.
   - Abrir modal com os dados.
6. Indicar status da conexão SSE (conectado/desconectado/reconectando).

### style.css — Diretrizes

- Design limpo, mobile-responsive.
- Badges coloridos para status:
  - `pending`: 🟡 amarelo
  - `confirmed`: 🟢 verde
  - `failed`: 🔴 vermelho
  - `refunded`: 🔵 azul
- Feed em tempo real em uma div scrollável com altura máxima.
- Tabela de pagamentos com linhas clicáveis.
- Modal de histórico com overlay escuro.

---

## Integração com o Consumer

O consumer existente (spec `0001-kafka-payment-consumer`) deve publicar eventos
no Event Bus da UI. A forma de integração é:

1. O `cmd/ui/main.go` cria uma instância do `EventBus`.
2. O Event Bus é passado para o consumer via injeção de dependência.
3. Ao processar uma mensagem com sucesso, o consumer chama
   `eventBus.Publish(ctx, event)`.
4. Se o buffer do Event Bus estiver cheio, a publicação não bloqueia — o evento
   é descartado (log warning).

**Alternativa**: Se a UI rodar como processo separado (recomendado), o
consumer pode publicar eventos em um tópico Kafka separado
(`payment.ui.events`) que a UI consome. **Decisão**: para simplicidade,
usar Event Bus em memória com a UI rodando como parte do mesmo processo ou
como um sidecar que compartilha o Event Bus via canal Go. Em cenários onde a
UI é um processo separado, usar um canal HTTP interno ou Redis Pub/Sub.

**Decisão final**: Usar **Redis Pub/Sub** como mecanismo de integração entre
consumer e UI quando rodam em processos separados. O consumer publica no
canal `payment:events` do Redis, e a UI se inscreve nesse canal. Isso
elimina o acoplamento direto e permite que a UI seja um processo
independente.

---
