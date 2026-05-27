# Producer

## O que é

O **Producer** é o serviço responsável por publicar eventos de pagamento no tópico Kafka `payment.events`. Ele opera em **dois modos distintos**:

| Modo | Subcomando | Descrição |
|------|-----------|-----------|
| **CLI (publish)** | `producer publish` | Ferramenta de linha de comando one-shot para publicar eventos diretamente do terminal |
| **Servidor HTTP (serve)** | `producer serve` | Servidor HTTP long-lived que expõe endpoints REST para publicação de eventos |

Ambos os modos compartilham a mesma lógica de negócio (`internal/producer/producer.go`) e o mesmo validador (`internal/validator`).

## Por que existe

- **Modo CLI**: Desenvolvedores e QA precisam de uma ferramenta rápida para testar o consumer, simular cenários e executar testes de carga sem dependência de sistemas externos.
- **Modo HTTP**: O ambiente Docker / orquestração precisa de um serviço long-lived que possa receber requisições HTTP de outros serviços ou de ferramentas externas (curl, Postman, scripts) sem exigir acesso ao terminal do container.

## Como funciona

### Visão geral do binário

O binário `producer` usa um dispatcher de subcomandos na função `run()`:

```
producer [subcomando] [flags]
```

- Se o primeiro argumento for `serve`, executa o modo servidor HTTP.
- Caso contrário, executa o modo CLI `publish` (compatibilidade retroativa).

### Modo CLI (`publish`)

Documentação detalhada em [`docs/features/cli-producer.md`](features/cli-producer.md).

**Fluxo resumido:**

1. Parse de flags individuais, `--payload`, `--file` ou stdin pipe
2. Geração ou leitura dos eventos de pagamento
3. Validação contra o schema (reusa o `internal/validator`)
4. Publicação no Kafka via `sarama.SyncProducer`
5. Exibição dos resultados (partição/offset ou erro)

### Modo Servidor HTTP (`serve`)

O servidor HTTP transforma o producer em um **serviço long-lived**, ideal para execução em containers Docker.

**Fluxo:**

```
Cliente HTTP (curl, script, outro serviço)
        │
        ▼
┌──────────────────────────────┐
│  POST /publish              │ → Publica 1 evento
│  POST /publish/bulk         │ → Publica N eventos gerados
│  GET  /healthz              │ → Health check
└──────────┬───────────────────┘
           │
           ▼
┌──────────────────────────────┐
│  Validação da requisição    │
│  ├─ Content-Type JSON       │
│  ├─ Tamanho do body (64KB)  │
│  └─ Validação de campos     │
└──────────┬───────────────────┘
           │
           ▼
┌──────────────────────────────┐
│  Publicação Kafka           │
│  (SyncProducer com retry)   │
└──────────┬───────────────────┘
           │
           ▼
┌──────────────────────────────┐
│  Resposta JSON              │
│  200 → sucesso              │
│  400 → erro de validação    │
│  502 → falha Kafka          │
└──────────────────────────────┘
```

#### Conexão Kafka com Retry

O servidor tenta conectar ao Kafka com **backoff exponencial** antes de iniciar o HTTP:

| Parâmetro | Valor |
|-----------|-------|
| Backoff inicial | 500ms |
| Backoff máximo | 8s |
| Timeout total | ~30s |
| Estratégia | 500ms → 1s → 2s → 4s → 8s → 8s... |

Se o Kafka não estiver disponível após 30 segundos, o servidor falha na inicialização.

#### Graceful Shutdown

O servidor HTTP responde a `SIGINT` e `SIGTERM`:
1. Recebe o sinal → inicia shutdown
2. HTTP server para de aceitar novas conexões
3. Conexões existentes têm até **15 segundos** para finalizar
4. Kafka producer é fechado

## Endpoints HTTP

### `POST /publish`

Publica um único evento de pagamento no Kafka.

**Request:**

```json
{
  "payment_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "status": "confirmed",
  "amount": 150.00,
  "currency": "BRL",
  "description": "Pagamento de teste"
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

**Response (502 — falha Kafka):**

```json
{
  "error": "kafka publish failed"
}
```

### `POST /publish/bulk`

Gera e publica N eventos de pagamento automaticamente.

**Request:**

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
    "error": "kafka error description"
  }
]
```

### `GET /healthz`

Health check simples para readiness/liveness.

**Response (200):**

```json
{
  "status": "ok"
}
```

## Configuração

### Modo CLI (`publish`)

| Parâmetro | Default | Flag |
|-----------|---------|------|
| Brokers Kafka | `localhost:9092` | `--brokers` |
| Tópico | `payment.events` | `--topic` |
| Timeout Kafka | 10s | (fixo) |
| Dial timeout | 5s | (fixo) |

### Modo Servidor HTTP (`serve`)

| Parâmetro | Default | Flag / Env |
|-----------|---------|------------|
| Porta HTTP | `8082` | `--port` |
| Brokers Kafka | `localhost:9092` | `--brokers` |
| Tópico | `payment.events` | `--topic` |

## Exemplos de Uso

### Modo CLI

```bash
# Publicar evento com flags
producer publish --status confirmed --amount 250.00 --currency USD

# Modo dry-run
producer publish --dry-run

# Bulk com rate limit
producer publish --count 50 --rate 10
```

Veja mais exemplos em [`docs/features/cli-producer.md`](features/cli-producer.md).

### Modo Servidor HTTP

```bash
# Iniciar servidor
producer serve --port 8082 --brokers localhost:9092

# Publicar evento
curl -X POST http://localhost:8082/publish \
  -H "Content-Type: application/json" \
  -d '{"amount": 150.00, "currency": "BRL"}'

# Publicar lote de 10 eventos
curl -X POST http://localhost:8082/publish/bulk \
  -H "Content-Type: application/json" \
  -d '{"count": 10}'

# Health check
curl http://localhost:8082/healthz
```

### Docker Compose

No `docker-compose.yml`, o serviço `producer` já está configurado para iniciar em modo `serve`:

```yaml
producer:
  build:
    context: .
    dockerfile: Dockerfile.producer
  command: ["serve", "--port", "8082", "--brokers", "kafka:9092"]
  ports:
    - "8082:8082"
```

Acesse: `http://localhost:8082`

## Observações Técnicas

### Modo CLI vs Modo HTTP

| Aspecto | CLI (publish) | HTTP (serve) |
|---------|---------------|--------------|
| Ciclo de vida | One-shot (executa e sai) | Long-lived (até SIGTERM) |
| Fonte de dados | Flags, stdin, arquivo, auto-geração | JSON body da requisição |
| Saída | Texto ou JSON no stdout | JSON na resposta HTTP |
| Rate limiting | Sim (`--rate`) | Não (publica tudo de uma vez) |
| Logging | Mensagens em stderr | JSON estruturado (slog) |
| Graceful shutdown | Sim (Ctrl+C interrompe bulk) | Sim (SIGINT/SIGTERM) |
| Retry na conexão Kafka | Sim (backoff exponencial) | Sim (backoff exponencial) |

### Portas

| Serviço | Porta |
|---------|-------|
| Producer HTTP | 8082 |
| Consumer HTTP | 8080 |
| UI | 8081 |

### Edge Cases

| Cenário | Comportamento |
|---------|---------------|
| Kafka indisponível na inicialização | Retry por ~30s, depois falha |
| Kafka cai durante operação | Requisição retorna 502 |
| Body muito grande (>64KB) | Rejeitado com 400 |
| Content-Type ausente ou inválido | Rejeitado com 415 |
| `count` fora do range 1-100 | Rejeitado com 400 |
| Bulk com falha parcial | Response inclui campo `error` nos itens falhos |
| Sinal SIGTERM durante requisição | Graceful shutdown: requisições ativas têm 15s para completar |
