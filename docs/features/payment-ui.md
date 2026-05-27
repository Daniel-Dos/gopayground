# Payment UI

## O que é

A **Payment UI** é uma interface web de desenvolvimento para monitorar em tempo real o processamento de eventos de pagamento. Ela permite visualizar dados persistidos no Redis e DynamoDB sem acesso direto aos bancos.

## Por que existe

Sem uma interface visual, desenvolvedores e operadores precisam acessar Redis e DynamoDB via CLI para depurar problemas. A UI oferece:

- **Feed em tempo real** de eventos sendo processados (via SSE)
- **Consulta de status** de pagamentos no Redis
- **Histórico completo** de transações no DynamoDB
- **Métricas agregadas** (total processado, taxa de sucesso, etc.)
- **Filtros** por payment_id e status

## Como funciona

### Arquitetura

```
Browser (Dashboard HTML/CSS/JS)
        │
        │ HTTP (mesma origem, porta 8081)
        ▼
┌─────────────────────────────────────┐
│         Go HTTP Server              │
│                                     │
│  ┌──────────┐  ┌───────────────┐    │
│  │  Static  │  │  REST API     │    │
│  │  Files   │  │  /api/payments│    │
│  │(index.html│  │  /api/.../   │    │
│  │ app.js)  │  │  history      │    │
│  └──────────┘  │  /api/metrics │    │
│                │  /healthz     │    │
│  ┌──────────┐  └───────┬───────┘    │
│  │  SSE     │          │            │
│  │/api/event│          ▼            │
│  └────┬─────┘   ┌────────────┐      │
│       │         │   Redis    │      │
│       │         │   Reader   │      │
│       │         └────────────┘      │
│       │         ┌────────────┐      │
│       │         │  DynamoDB  │      │
│       │         │  Reader    │      │
│       │         └────────────┘      │
│       ▼                             │
│  ┌────────────┐                     │
│  │ Event Bus  │← Redis Pub/Sub      │
│  │ (in-memory)│   (canal payment:   │
│  └────────────┘    events)          │
└─────────────────────────────────────┘
```

### Endpoints da API

| Método | Rota                            | Descrição                          |
|--------|---------------------------------|------------------------------------|
| GET    | `/`                             | Serve o dashboard HTML             |
| GET    | `/api/events`                   | SSE stream de eventos em tempo real|
| GET    | `/api/payments`                 | Lista pagamentos (Redis)           |
| GET    | `/api/payments/{id}/history`    | Histórico de um pagamento (DynamoDB)|
| GET    | `/api/metrics`                  | Métricas agregadas                 |
| GET    | `/healthz`                      | Health check                       |

#### GET /api/payments

Retorna a lista de pagamentos com status atual lida do Redis. Suporta filtros opcionais:

- `?payment_id=abc` — filtra por ID (LIKE, case insensitive)
- `?status=confirmed` — filtra por status exato

```json
[
  {
    "payment_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "status": "confirmed",
    "updated_at": "2026-05-24T10:30:01Z"
  }
]
```

#### GET /api/payments/{id}/history

Retorna o histórico completo de um pagamento do DynamoDB, ordenado por timestamp ascendente.

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

#### GET /api/metrics

Retorna métricas agregadas calculadas a partir dos dados do Redis.

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

### Eventos em Tempo Real (SSE)

O endpoint `/api/events` utiliza **Server-Sent Events** para transmitir eventos em tempo real.

**Formato da mensagem SSE:**
```
event: payment
data: {"payment_id":"...","status":"confirmed","amount":150.00,...}

event: heartbeat
data: {}
```

- **Heartbeat**: enviado a cada 30s para manter a conexão ativa
- **Reconexão automática**: o navegador reconecta automaticamente via `EventSource`

### Event Bus (`internal/ui/events.go`)

O Event Bus conecta o consumer Kafka à UI usando **Redis Pub/Sub** (canal `payment:events`).

```go
type EventBus struct {
    redis       *redis.Client
    channel     string
    subscribers map[string]chan *models.PaymentEvent
    mu          sync.RWMutex
    // ...
}
```

- **Publish**: consumer publica evento no Redis Pub/Sub
- **Subscribe**: UI se inscreve e recebe eventos via canal com buffer (default 256)
- **Subscriber lento**: eventos são descartados (non-blocking) para não travar o consumer

### Frontend

O frontend é **Vanilla JS** (sem frameworks) com arquivos embutidos via `embed.FS`:

- `index.html` — estrutura da página
- `app.js` — lógica (SSE, fetch, render, filtros, modal de histórico)
- `style.css` — estilos responsivos com badges coloridos por status

**Funcionalidades JS:**
1. Conexão SSE com reconexão automática
2. Load inicial de dados (`/api/payments` + `/api/metrics`)
3. Feed em tempo real (limitado a 200 eventos)
4. Tabela de pagamentos com filtros
5. Modal de histórico com todos os campos
6. Métricas atualizadas automaticamente
7. Retry com backoff nas chamadas de API (3 tentativas)

## Configuração

| Variável              | Default          | Descrição                             |
|-----------------------|------------------|---------------------------------------|
| `UI_PORT`             | `8081`           | Porta do servidor HTTP                |
| `UI_EVENT_BUS_BUFFER` | `256`            | Tamanho do buffer do Event Bus        |
| `UI_READ_TIMEOUT`     | `10s`            | Timeout de leitura HTTP               |
| `UI_WRITE_TIMEOUT`    | `30s`            | Timeout de escrita HTTP (SSE)         |
| `REDIS_ADDR`          | `localhost:6379` | Endereço Redis (compartilhado)        |
| `REDIS_PASSWORD`      | (vazio)          | Senha Redis (compartilhado)           |
| `DYNAMODB_ENDPOINT`   | `http://localhost:4566` | Endpoint DynamoDB (compartilhado)|
| `DYNAMODB_TABLE`      | `payment_history`| Tabela DynamoDB (compartilhado)       |

## Exemplos de Uso

### Executar a UI

```bash
# Modo desenvolvimento
make run-ui

# Ou diretamente
go run ./cmd/ui

# Acessar
# http://localhost:8081
```

### Verificar health check

```bash
curl http://localhost:8081/healthz
# {"status":"ok","redis":"connected"}
```

### Listar pagamentos via API

```bash
curl http://localhost:8081/api/payments

# Com filtros
curl "http://localhost:8081/api/payments?status=failed"
curl "http://localhost:8081/api/payments?payment_id=abc"
```

## Edge Cases

| Cenário                               | Comportamento                                              |
|---------------------------------------|------------------------------------------------------------|
| Redis indisponível                    | Payments list e metrics retornam 500. Health check 503.    |
| DynamoDB indisponível                 | Histórico retorna 500. Demais funcionalidades ok.          |
| Consumer não está rodando             | UI funciona com dados estáticos. SSE vazio (sem eventos).  |
| Buffer do Event Bus cheio             | Eventos são descartados (log warning). Não bloqueia.       |
| Muitas conexões SSE (100+)            | Semáforo rejeita novas conexões com 503.                   |
| Cliente desconecta abruptamente       | `r.Context().Done()` finaliza a goroutine do SSE.          |
| Dados sensíveis (amount)              | Amount é exibido — UI é ferramenta de desenvolvimento.     |
