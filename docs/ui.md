# Payment UI

## O que é

A **Payment UI** é um servidor web Go que fornece uma interface gráfica e APIs REST para monitoramento e operação do sistema de processamento de pagamentos. Ela consolida dados do Redis (status), DynamoDB (histórico) e Kafka (publicação) em um único ponto de acesso.

## Por que existe

- **Monitoramento visual**: desenvolvedores e operadores acompanham eventos de pagamento em tempo real sem acessar Redis/DynamoDB via CLI
- **Publicação manual**: permite publicar eventos de pagamento no Kafka diretamente pela interface web ou via API REST (útil para testes e depuração)
- **Centralização**: unifica consulta de status, histórico, métricas e publicação em um só lugar

## Como funciona

### Arquitetura

```
┌─────────────────────────────────────────────────────────┐
│                    Payment UI (:8081)                    │
│                                                         │
│  ┌──────────────┐    ┌──────────────────────────────┐   │
│  │  Static      │    │       REST + SSE API          │   │
│  │  Files       │    │                              │   │
│  │  /index.html │    │  GET  /api/events       (SSE) │   │
│  │  /dashboard  │    │  GET  /api/payments           │   │
│  │  /producer   │    │  GET  /api/payments/{id}/hist │   │
│  └──────────────┘    │  GET  /api/metrics            │   │
│                      │  GET  /healthz                │   │
│  ┌──────────────┐    │  POST /api/publish            │   │
│  │  Event Bus   │    │  POST /api/publish/bulk       │   │
│  │  (in-memory) │    └────────┬─────────────────────┘   │
│  └──────┬───────┘             │                         │
│         │                     │                         │
└─────────│─────────────────────│─────────────────────────┘
          │                     │
          ▼                     ▼
   ┌──────────┐         ┌──────────────┐
   │  Redis   │         │   DynamoDB   │
   │  (status │         │  (histórico) │
   │  + Pub/  │         └──────────────┘
   │   Sub)   │
   └──────────┘
```

### Event Bus (Redis Pub/Sub)

O consumer Kafka publica eventos no Redis Pub/Sub (canal `payment:events`). A UI se inscreve nesse canal e distribui os eventos para as conexões SSE ativas. O Event Bus é em memória com buffer (configurável via `UI_EVENT_BUS_BUFFER`, default 256).

## Páginas Web

| Rota | Arquivo | Descrição |
|------|---------|-----------|
| `GET /` | `static/index.html` | Dashboard principal: feed SSE em tempo real, tabela de pagamentos com filtros, métricas agregadas, modal de histórico |
| `GET /dashboard` | `static/dashboard.html` | Dashboard gráfico com visualizações interativas (Chart.js) |
| `GET /producer` | `static/producer.html` | Página para publicar eventos manualmente via formulário |

Os assets estáticos (CSS, JS) são embarcados no binário Go via `//go:embed static/*`.

## Endpoints da API

### `GET /api/events` — SSE Stream

Stream de eventos em tempo real via **Server-Sent Events**.

**Formato das mensagens:**

```
event: payment
data: {"payment_id":"...","status":"confirmed","amount":150.00,"currency":"BRL","description":"...","timestamp":"2026-05-24T10:30:00Z"}

event: heartbeat
data: {}
```

- **Heartbeat**: enviado a cada 30 segundos para manter a conexão ativa
- **Reconexão**: o navegador reconecta automaticamente via `EventSource`
- **Limite**: máximo de 100 conexões SSE simultâneas (semáforo)
- **Subscriber lento**: eventos são descartados (non-blocking) se o buffer do subscriber estiver cheio

---

### `GET /api/payments` — Listar Pagamentos

Retorna a lista de pagamentos com status atual, lida do Redis via SCAN.

**Parâmetros de query (opcionais):**

| Parâmetro | Tipo | Descrição |
|-----------|------|-----------|
| `payment_id` | string | Filtro por ID (LIKE, case insensitive, máx 64 caracteres) |
| `status` | string | Filtro por status exato (case insensitive, máx 16 caracteres) |

**Response (200):**

```json
[
  {
    "payment_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "status": "confirmed",
    "updated_at": "2026-05-24T10:30:01Z"
  }
]
```

**Edge cases:**
- Redis indisponível → retorna 500
- Nenhum pagamento encontrado → retorna array vazio `[]`
- `payment_id` > 64 caracteres → retorna 400
- `status` > 16 caracteres → retorna 400

---

### `GET /api/payments/{id}/history` — Histórico de Pagamento

Retorna o histórico completo de um pagamento, consultado no DynamoDB e ordenado por timestamp ascendente.

**Parâmetros de path:**

| Parâmetro | Descrição |
|-----------|-----------|
| `{id}` | ID do pagamento (máx 64 caracteres) |

**Response (200):**

```json
[
  {
    "payment_id": "a1b2c3d4-...",
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

**Edge cases:**
- DynamoDB indisponível → retorna 500
- Pagamento não encontrado → retorna array vazio `[]`
- `id` > 64 caracteres → retorna 400
- Timeout de 10 segundos na consulta DynamoDB

---

### `GET /api/metrics` — Métricas Agregadas

Calcula métricas a partir dos dados no Redis via SCAN.

**Response (200):**

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
  "dlq_count": 0
}
```

**Campos:**

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `total_processed` | int | Total de pagamentos encontrados no Redis |
| `by_status` | object | Contagem por status (`confirmed`, `failed`, `pending`, `refunded`) |
| `success_rate` | float | Percentual de `confirmed` sobre o total de status finais (confirmed + failed + refunded) |
| `dlq_count` | int | Sempre 0 (não implementado — reservado) |

**Edge cases:**
- Redis indisponível → retorna métricas parciais (o que foi lido antes da falha)
- Nenhum pagamento → `total_processed: 0`, `success_rate: 0`

---

### `GET /healthz` — Health Check

Verifica a conectividade com Redis.

**Response (200):**

```json
{
  "status": "ok",
  "redis": "connected"
}
```

**Response (503):**

```json
{
  "status": "unhealthy",
  "redis": "down"
}
```

---

### `POST /api/publish` — Publicar Evento

Publica **um** evento de pagamento no tópico Kafka `payment.events`.

**Headers:**
- `Content-Type: application/json` (obrigatório)

**Request body:**

```json
{
  "payment_id": "a1b2c3d4-...",
  "status": "confirmed",
  "amount": 150.00,
  "currency": "BRL",
  "description": "Pedido #12345"
}
```

Todos os campos são opcionais — valores default são aplicados:

| Campo | Default | Validação |
|-------|---------|-----------|
| `payment_id` | UUID v4 auto-gerado | — |
| `status` | `pending` | Deve ser um de: `pending`, `confirmed`, `failed`, `refunded` |
| `amount` | — (obrigatório) | Deve ser > 0 |
| `currency` | — (obrigatório) | Deve ter exatamente 3 letras (ISO 4217) |
| `description` | vazio | Máximo 255 caracteres |

**Response (200):**

```json
{
  "status": "published",
  "payment_id": "a1b2c3d4-...",
  "partition": 0,
  "offset": 42
}
```

**Response (400 — erro de validação):**

```json
{
  "error": "amount must be greater than zero"
}
```

**Response (502 — Kafka indisponível):**

```json
{
  "error": "kafka not available"
}
```

**Edge cases:**
- Kafka não conectado na inicialização → endpoint retorna 502
- Body > 64KB → rejeitado com 400 (MaxBytesReader)
- Content-Type ausente ou inválido → rejeitado com 415

---

### `POST /api/publish/bulk` — Publicar Lote

Gera e publica **N** eventos de pagamento automaticamente.

**Headers:**
- `Content-Type: application/json` (obrigatório)

**Request body:**

```json
{
  "count": 10
}
```

| Campo | Default | Validação |
|-------|---------|-----------|
| `count` | — (obrigatório) | Deve ser entre 1 e 100 |

Os eventos são gerados com:
- `payment_id`: UUID v4 único para cada evento
- `status`: `confirmed`
- `amount`: varia de 10 a N×10 (ex: count=10 → amounts 10, 20, 30...100)
- `currency`: `BRL`
- `description`: `"Bulk event X of N"`
- `timestamp`: atual (RFC3339)

**Response (200):**

```json
[
  {
    "payment_id": "uuid-1",
    "status": "confirmed",
    "partition": 0,
    "offset": 43
  },
  {
    "payment_id": "uuid-2",
    "status": "confirmed",
    "partition": 0,
    "offset": 44
  }
]
```

Se algum evento individual falhar, o item correspondente incluirá o campo `error`:

```json
[
  {
    "payment_id": "uuid-1",
    "status": "confirmed",
    "partition": 0,
    "offset": 43
  },
  {
    "payment_id": "uuid-2",
    "status": "confirmed",
    "error": "kafka publish failed"
  }
]
```

**Edge cases:**
- Kafka não conectado → retorna 502
- Body > 4KB → rejeitado com 400 (MaxBytesReader)
- `count` fora do range 1–100 → rejeitado com 400
- Falha parcial → response inclui campo `error` nos itens falhos
- Timeout de 30 segundos para operações bulk longas → itens pendentes retornam com erro `"request cancelled or timed out"`
- Cancelamento via contexto (ex: SIGTERM) → interrompe o bulk pendente

## Como Executar

### Desenvolvimento local

```bash
# Pré-requisitos: Redis + DynamoDB + Kafka rodando
docker-compose up -d redis floci kafka otel-collector

# Executar a UI (modo dev — copia docs automaticamente)
make run-ui

# Ou diretamente:
go run ./cmd/ui

# Acessar: http://localhost:8081
```

### Docker Compose

```bash
# Sobe todos os serviços incluindo a UI
make docker-up

# A UI estará em http://localhost:8081
```

### Compilação

```bash
# Compila o binário (copia docs/ para embed)
make build-ui

# O binário estará em bin/ui
```

## Configuração

| Variável | Default | Descrição |
|----------|---------|-----------|
| `UI_PORT` | `8081` | Porta do servidor HTTP |
| `UI_EVENT_BUS_BUFFER` | `256` | Tamanho do buffer do Event Bus (subscribers) |
| `UI_READ_TIMEOUT` | `10s` | Timeout de leitura HTTP |
| `REDIS_ADDR` | `localhost:6379` | Endereço do Redis |
| `REDIS_PASSWORD` | (vazio) | Senha do Redis |
| `DYNAMODB_ENDPOINT` | (vazio) | Endpoint DynamoDB (para uso com Floci/LocalStack) |
| `DYNAMODB_TABLE` | `payment_history` | Nome da tabela DynamoDB |
| `KAFKA_BROKERS` | (vazio) | Lista de brokers Kafka separados por vírgula |
| `KAFKA_TOPIC` | (vazio) | Tópico Kafka para publicação |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | (vazio) | Endpoint OTLP para OpenTelemetry |
| `OTEL_SERVICE_NAME` | `payment-ui` | Nome do serviço para OTel |
| `GRACEFUL_SHUTDOWN_TIMEOUT` | `30s` | Timeout máximo para shutdown graceful |

## Exemplos de Uso com curl

```bash
# ==========================================
# Health Check
# ==========================================
curl http://localhost:8081/healthz

# ==========================================
# Consulta
# ==========================================
# Listar todos os pagamentos
curl http://localhost:8081/api/payments

# Listar com filtro por status
curl "http://localhost:8081/api/payments?status=confirmed"

# Listar com filtro por ID
curl "http://localhost:8081/api/payments?payment_id=abc"

# Histórico de um pagamento específico
curl http://localhost:8081/api/payments/meu-id-aqui/history

# Métricas agregadas
curl http://localhost:8081/api/metrics

# ==========================================
# Publicação
# ==========================================
# Publicar um evento de pagamento
curl -X POST http://localhost:8081/api/publish \
  -H "Content-Type: application/json" \
  -d '{
    "amount": 150.00,
    "currency": "BRL",
    "status": "confirmed",
    "description": "Pedido #12345"
  }'

# Publicar sem payment_id (será auto-gerado)
curl -X POST http://localhost:8081/api/publish \
  -H "Content-Type: application/json" \
  -d '{"amount": 99.90, "currency": "USD"}'

# Publicar lote de 5 eventos
curl -X POST http://localhost:8081/api/publish/bulk \
  -H "Content-Type: application/json" \
  -d '{"count": 5}'

# ==========================================
# SSE Stream (tempo real)
# ==========================================
curl -N http://localhost:8081/api/events
```

## Middlewares

A UI aplica os seguintes middlewares em todas as requisições:

### Security Headers

| Header | Valor |
|--------|-------|
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Referrer-Policy` | `no-referrer` |
| `Content-Security-Policy` | `default-src 'self'; style-src 'self' https://fonts.googleapis.com 'unsafe-inline'; font-src 'self' https://fonts.gstatic.com; script-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'` |

### OTel Instrumentation

- **Métricas**: `payment.ui.http_requests_total` (counter), `payment.ui.http_request_duration_ms` (histograma)
- **Tracing**: span por requisição HTTP com atributos `http.method`, `http.target`, `http.host`, `http.status_code`

### Recovery

Panics são capturados e retornam 500, evitando crash do servidor.

## Edge Cases

| Cenário | Comportamento |
|---------|---------------|
| Redis indisponível | Payments list e metrics retornam 500. Health check retorna 503. SSE fica sem eventos. |
| DynamoDB indisponível | Histórico retorna 500. Demais endpoints funcionam normalmente. |
| Kafka não conectado (init) | UI inicia sem producer. Endpoints POST /api/publish* retornam 502. |
| Kafka cai durante operação | Requisição de publish retorna 502. |
| Consumer não está rodando | UI funciona com dados estáticos no Redis. SSE stream vazio (sem novos eventos). |
| Buffer do Event Bus cheio | Eventos descartados com log warning. Não bloqueia o publisher. |
| Muitas conexões SSE (100+) | Semáforo rejeita novas conexões com 503. |
| Cliente SSE desconecta abruptamente | `r.Context().Done()` finaliza a goroutine. |
| Body > 64KB (publish) | Rejeitado com 400 (MaxBytesReader). |
| Body > 4KB (bulk) | Rejeitado com 400 (MaxBytesReader). |
| Content-Type inválido | Rejeitado com 415. |
| `count` fora do range 1–100 | Rejeitado com 400. |
| Bulk com falha parcial | Response inclui campo `error` nos itens individuais que falharam. |
| Sinal SIGTERM durante requisição | Graceful shutdown: requisições ativas têm até 15s para completar (configurável via `GRACEFUL_SHUTDOWN_TIMEOUT`). |
| Write timeout vs SSE | Write timeout é `0` (desabilitado) para não fechar conexões SSE long-lived prematuramente. |
| Event Bus sem subscriber | Eventos publicados no Redis Pub/Sub são descartados se não houver subscriber. |

## Documentos Relacionados

- [`docs/features/payment-ui.md`](features/payment-ui.md) — Documentação específica da feature (spec SDD)
- [`adrs/ADR-005-sse-real-time-streaming.md`](../adrs/ADR-005-sse-real-time-streaming.md) — ADR sobre SSE
- [`adrs/ADR-006-redis-pubsub-event-bus.md`](../adrs/ADR-006-redis-pubsub-event-bus.md) — ADR sobre Event Bus
