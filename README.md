# GoPayground

[![GoPayground](https://img.shields.io/badge/GoPayground-github.com/Daniel--Dos/gopayground-blue?logo=github)](https://github.com/Daniel-Dos/gopayground)

**Distributed payment event processing in Go — Kafka, Redis, DynamoDB, OpenTelemetry.**

## Descrição Geral

O **GoPayground** é um ambiente prático de experimentação com sistemas distribuídos em Go, centrado no processamento de eventos de pagamento com Apache Kafka. O sistema consome eventos de pagamento de um tópico Kafka, valida, persiste em DynamoDB e Redis, e oferece uma interface web para monitoramento e uma CLI para testes.

### Stack Tecnológica

| Componente         | Tecnologia                                                          |
|--------------------|---------------------------------------------------------------------|
| **Linguagem**      | Go 1.26                                                             |
| **Message Broker** | Apache Kafka (via `github.com/IBM/sarama`)                          |
| **Cache/Status**   | Redis 7 (via `github.com/redis/go-redis/v9`)                        |
| **Histórico**      | DynamoDB (via `github.com/aws/aws-sdk-go-v2`)                       |
| **Observabilidade**| OpenTelemetry (Tracing + Métricas + Logs estruturados)              |
| **Validação**      | `github.com/go-playground/validator/v10`                            |
| **Config**         | Viper (arquivo YAML + variáveis de ambiente)                        |
| **CLI**            | `flag` stdlib                                                       |

## Features

O projeto possui 3 features implementadas com base em specs SDD (Spec Driven Development):

| ID                     | Feature                | Descrição                                    | URL (dev)                    |
|------------------------|------------------------|----------------------------------------------|------------------------------|
| `0001`                 | Kafka Payment Consumer | Consumer Kafka que processa eventos de pagamento | `http://localhost:8080` |
| `0002`                 | Payment UI             | Interface web para monitoramento em tempo real | `http://localhost:8081` |
| `0003`                 | CLI/HTTP Producer      | Ferramenta CLI e servidor HTTP para publicar eventos no Kafka | `http://localhost:8082` |

## Arquitetura Geral

### Diagramas de Arquitetura

O projeto inclui diagramas visuais no formato Excalidraw (`.excalidraw`). Abra-os em [https://excalidraw.com](https://excalidraw.com) para visualização interativa.

| Diagrama | Descrição | Arquivo |
|----------|-----------|---------|
| **Arquitetura Geral** | Visão completa do sistema: Kafka, Consumer, Redis, DynamoDB, UI, OTel | [`docs/diagrams/architecture-overview.excalidraw`](docs/diagrams/architecture-overview.excalidraw) |
| **Fluxo de Processamento** | Pipeline do consumer: validação → idempotência → retry → DLQ | [`docs/diagrams/message-processing-flow.excalidraw`](docs/diagrams/message-processing-flow.excalidraw) |
| **Event Bus** | Redis Pub/Sub: consumer → EventBus → SSE → Browser | [`docs/diagrams/event-bus-flow.excalidraw`](docs/diagrams/event-bus-flow.excalidraw) |
| **Data Stores** | Schema Redis (status + idempotência) e DynamoDB (histórico) | [`docs/diagrams/data-stores.excalidraw`](docs/diagrams/data-stores.excalidraw) |
| **Deploy Docker** | Topologia Docker Compose com portas e rede | [`docs/diagrams/deployment-architecture.excalidraw`](docs/diagrams/deployment-architecture.excalidraw) |
| **Observabilidade** | Stack OTel: OTLP → Collector → Batch → Exporters | [`docs/diagrams/observability-stack.excalidraw`](docs/diagrams/observability-stack.excalidraw) |

### Diagrama ASCII (visão textual)

Para uma versão visual interativa, abra o arquivo [`docs/diagrams/architecture-overview.excalidraw`](docs/diagrams/architecture-overview.excalidraw).

```
 ┌─────────────────────────────────────────────────────────────────────┐
 │                         Kafka Cluster                              │
 │  ┌──────────────────────────────┐   ┌─────────────────────────┐     │
 │  │   payment.events (3 part)    │   │ payment.events.dlq(1p)  │     │
 │  └────────┬─────────────────────┘   └────────────▲────────────┘     │
 │           │                                      │                  │
 └───────────│──────────────────────────────────────│──────────────────┘
             │                                      │
     ┌────────────────────────┐       ┌─────────────────────┐
     │        Producer        │       │   Kafka Consumer    │
     │   CLI / HTTP Server    │       │    (cmd/consumer)   │
     │     (cmd/producer)     │       └──────────┬──────────┘
     └───────────┬────────────┘                  │
                                     ┌───────────┼───────────┐
                                     │           │           │
                                     ▼           ▼           ▼
                               ┌──────────┐ ┌──────────┐ ┌──────────────┐
                               │  Redis   │  │ DynamoDB │  │   DLQ        │
                               │ (status) │  │(history) │  │(DLQ Topic)   │
                               │ (idemp.) │  │          │  │              │
                               └─────┬────┘  └──────────┘  └──────────────┘
                                     │
                                     ▼
                               ┌──────────┐
                               │Payment UI│
                               │(:8081)   │
                               └──────────┘

 ┌─────────────────────────────────────────────────────────────────────┐
 │                        Observabilidade                             │
 │   OpenTelemetry Collector → Tracing (Jaeger) → Métricas (Prometheus)│
 │   Logs estruturados (slog JSON)                                     │
 └─────────────────────────────────────────────────────────────────────┘
```

### Fluxo de Dados

1. **Producer** (CLI `publish` ou HTTP API `serve`) ou sistema externo publica evento no tópico `payment.events`
2. **Kafka Consumer** consome a mensagem e realiza:
   - Validação do payload (schema, regras de negócio)
   - Verificação de idempotência via Redis
   - Atualização de status no Redis
   - Persistência do histórico no DynamoDB
3. Em caso de falha recuperável → **retry** com backoff exponencial (até 3 tentativas)
4. Em caso de falha permanente → **Dead Letter Queue** (tópico `payment.events.dlq`)
5. **Payment UI** exibe dados em tempo real via SSE e consulta Redis/DynamoDB

### Fluxo de Publicação (UI → Producer HTTP → Kafka)

A partir do ADR-013, a UI não publica mais diretamente no Kafka. Em vez disso, ela delega a publicação ao serviço **Producer** standalone via HTTP:

```
Browser → UI (8081) → HTTP POST → Producer (8082) → Kafka
```

**Fluxo detalhado:**

1. O browser faz `POST /api/publish` ou `POST /api/publish/bulk` na UI (porta 8081)
2. A UI valida o payload e faz um HTTP POST para `producer:8082/publish` ou `producer:8082/publish/bulk`
3. O Producer publica o evento no Kafka via Sarama SyncProducer e retorna `{payment_id, partition, offset}`
4. A UI recebe a resposta e **também** publica o evento no EventBus (Redis Pub/Sub) para alimentar o SSE em tempo real

**Implicações:**
- Logs de publicação são centralizados no Producer
- A UI não precisa mais de dependências Kafka diretas
- Se o Producer estiver off, os endpoints de publicação retornam 502 (mas consultas e SSE continuam funcionando)
- Latência adicional de ~10-50ms pelo round-trip HTTP

## Pré-requisitos

- Go 1.26+
- Docker e Docker Compose
- Kafka (via docker-compose)
- Redis (via docker-compose)
- DynamoDB Local / Floci (via docker-compose)

## Como Executar

### Stack completa com Docker Compose

```bash
# Iniciar todos os serviços
make docker-up

# Compilar e executar o consumer
make run

# Executar a UI
make run-ui

# Executar o producer (modo CLI publish — default)
make run-producer

# Executar o producer em modo servidor HTTP
make run-producer-serve
```

### Serviços individuais

```bash
# Iniciar dependências
docker-compose up -d kafka redis floci otel-collector

# Consumer (processa eventos)
go run ./cmd/consumer

# UI (dashboard web)
go run ./cmd/ui

# Producer — modo CLI (publicar evento de teste)
go run ./cmd/producer publish

# Producer — modo servidor HTTP (long-lived, porta 8082)
go run ./cmd/producer serve --port 8082
```

### Makefile

| Comando              | Descrição                                   |
|----------------------|---------------------------------------------|
| `make test`          | Executa todos os testes com race detector   |
| `make lint`          | Executa golangci-lint                       |
| `make build`         | Compila o consumer                          |
| `make build-ui`      | Compila a UI                                |
| `make build-producer`| Compila o binário producer                  |
| `make run`           | Executa o consumer                          |
| `make run-ui`        | Executa a UI                                |
| `make run-producer`  | Executa o producer (modo publish — CLI)     |
| `make run-producer-serve` | Executa o producer (modo serve — HTTP) |
| `make docker-up`     | Sobe todos os serviços Docker               |
| `make docker-down`   | Derruba todos os serviços Docker            |
| `make clean`         | Remove binários compilados                  |

## Estrutura do Projeto

```
.
├── cmd/
│   ├── consumer/main.go       # Entrypoint do Kafka Consumer
│   ├── producer/main.go       # Entrypoint do producer (modos: publish CLI + serve HTTP)
│   └── ui/main.go             # Entrypoint do servidor web UI
│
├── internal/
│   ├── config/config.go       # Configuração (viper + YAML + env vars)
│   ├── consumer/handler.go    # Handler de mensagens Kafka (sarama)
│   ├── producer/producer.go   # Lógica de publicação Kafka (reusada pelo CLI e HTTP)
│   ├── ui/
│   │   ├── server.go          # Servidor HTTP com middlewares
│   │   ├── handlers.go        # Handlers REST + SSE
│   │   ├── events.go          # Event Bus (Redis Pub/Sub)
│   │   └── static/            # Assets frontend (HTML, CSS, JS)
│   ├── models/payment.go      # Modelos de domínio (PaymentEvent, etc.)
│   ├── validator/validator.go # Validador de payload
│   ├── idempotency/redis.go   # Idempotência via Redis (SET NX)
│   ├── status/redis.go        # Status updater no Redis
│   ├── history/dynamodb.go    # Histórico no DynamoDB
│   ├── retry/handler.go       # Retry com backoff exponencial
│   └── dlq/producer.go        # Dead Letter Queue producer
│
├── pkg/
│   ├── telemetry/telemetry.go # OpenTelemetry (tracing + métricas)
│   └── model/user.go          # Modelo User (não utilizado)
│
├── adrs/                      # Architecture Decision Records
│   ├── ADR-001-kafka-message-broker.md
│   ├── ADR-002-redis-idempotency-status.md
│   ├── ADR-003-dynamodb-history.md
│   ├── ADR-004-sync-retry-backoff.md
│   ├── ADR-005-sse-real-time-streaming.md
│   ├── ADR-006-redis-pubsub-event-bus.md
│   ├── ADR-007-cli-producer-separate-binary.md
│   ├── ADR-008-eventual-consistency.md
│   ├── ADR-009-worker-pool-concurrency.md
│   ├── ADR-010-dlq-strategy.md
│   ├── ADR-011-opentelemetry-observability.md
│   └── ADR-012-graceful-shutdown.md
│
├── specs/
│   ├── 0001-kafka-payment-consumer/  # Spec do consumer
│   ├── 0002-payment-ui/              # Spec da UI
│   └── 0003-cli-producer/            # Spec do CLI producer
│
├── config.yaml               # Arquivo de configuração
├── docker-compose.yml        # Stack local (Kafka, Redis, Floci, OTel, UI)
├── Dockerfile                # Dockerfile do consumer
├── Dockerfile.ui             # Dockerfile da UI
├── otel-collector-config.yaml # Configuração do OpenTelemetry Collector
└── Makefile                  # Comandos de build, test, run
```

## Payment UI

A Payment UI oferece uma interface web para monitoramento em tempo real e também expõe **endpoints de API REST** para publicação manual de eventos.

### URL e Acesso

| Ambiente | URL |
|----------|-----|
| Desenvolvimento (local) | `http://localhost:8081` |
| Docker Compose | `http://localhost:8081` |

### Páginas Web

| Rota | Descrição |
|------|-----------|
| `GET /` | Dashboard principal (index.html) — feed SSE + tabela de pagamentos + métricas |
| `GET /dashboard` | Dashboard gráfico com visualizações interativas |
| `GET /producer` | Página HTML para publicar eventos manualmente |

### Endpoints de API

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/api/events` | SSE stream de eventos em tempo real |
| `GET` | `/api/payments` | Lista pagamentos no Redis (filtros: `?payment_id=`, `?status=`) |
| `GET` | `/api/payments/{id}/history` | Histórico completo de um pagamento no DynamoDB |
| `GET` | `/api/metrics` | Métricas agregadas (total, por status, taxa de sucesso) |
| `GET` | `/healthz` | Health check (Redis) |
| `POST` | `/api/publish` | Publica **um** evento de pagamento via Producer HTTP (porta 8082) |
| `POST` | `/api/publish/bulk` | Publica **N** eventos gerados automaticamente (1–100) via Producer HTTP |

### Exemplos de Uso com `curl`

```bash
# Health check
curl http://localhost:8081/healthz
# → {"status":"ok","redis":"connected"}

# Listar pagamentos
curl http://localhost:8081/api/payments

# Listar pagamentos com filtro
curl "http://localhost:8081/api/payments?status=failed"
curl "http://localhost:8081/api/payments?payment_id=abc"

# Histórico de um pagamento
curl http://localhost:8081/api/payments/meu-payment-id/history

# Métricas agregadas
curl http://localhost:8081/api/metrics

# Publicar um evento de pagamento
curl -X POST http://localhost:8081/api/publish \
  -H "Content-Type: application/json" \
  -d '{"amount": 150.00, "currency": "BRL", "status": "confirmed", "description": "Pedido #12345"}'

# Publicar lote de 10 eventos gerados automaticamente
curl -X POST http://localhost:8081/api/publish/bulk \
  -H "Content-Type: application/json" \
  -d '{"count": 10}'

# Consumir SSE stream (tempo real)
curl -N http://localhost:8081/api/events
```

### Dependências da UI

- **Redis**: obrigatório — armazena status dos pagamentos e canal Pub/Sub para SSE
- **DynamoDB**: obrigatório — armazena histórico completo
- **Producer HTTP** (porta 8082): obrigatório para publicação — se indisponível, os endpoints `POST /api/publish` e `POST /api/publish/bulk` retornam 502, mas a UI continua funcionando para consulta e SSE

### Documentação completa

Veja [`docs/ui.md`](docs/ui.md) para documentação detalhada da UI.

## Observabilidade

O sistema utiliza **OpenTelemetry** para tracing distribuído e métricas:

- **Tracing**: spans para cada mensagem processada, com atributos como `payment_id`, `partition`, `offset`
- **Métricas**: counters e histogramas exportados via OTLP para o Collector
- **Logs**: logs estruturados em JSON via `log/slog`
- **Health checks**: endpoints `/healthz` e `/readyz` na porta 8080 (consumer); `/healthz` na porta 8082 (producer HTTP); `/healthz` na porta 8081 (UI)

Configure via variáveis de ambiente (veja `docs/observability.md`).

## Documentação

| Documento                    | Descrição                                        |
|------------------------------|--------------------------------------------------|
| `docs/architecture.md`       | Visão geral da arquitetura                       |
| `docs/setup.md`              | Guia de setup e configuração                     |
| `docs/observability.md`      | Documentação de observabilidade (OTel, logs)     |
| `docs/features/payment-consumer.md` | Detalhes do Kafka Payment Consumer        |
| `docs/features/payment-ui.md`       | Detalhes da Payment UI (versão feature)    |
| `docs/ui.md`                        | Documentação completa da UI (API, páginas, exemplos curl) |
| `docs/producer.md`                  | Documentação completa do producer (CLI + HTTP) |
| `docs/features/cli-producer.md`     | Detalhes do modo CLI (publish) do producer |
| `docs/diagrams/`             | Diagramas Excalidraw da arquitetura do projeto   |
| `adrs/`                      | Architecture Decision Records (13 ADRs)          |
| `specs/` (pastas)            | Specs SDD completas de cada feature              |

## Decisões Arquiteturais (ADRs)

As principais decisões arquiteturais estão documentadas em formato ADR formal em `adrs/`:

| ADR | Decisão | Arquivo |
|-----|---------|---------|
| 001 | Apache Kafka como Message Broker | `adrs/ADR-001-kafka-message-broker.md` |
| 002 | Redis para Idempotência e Status | `adrs/ADR-002-redis-idempotency-status.md` |
| 003 | DynamoDB para Histórico | `adrs/ADR-003-dynamodb-history.md` |
| 004 | Retry Síncrono com Exponential Backoff | `adrs/ADR-004-sync-retry-backoff.md` |
| 005 | SSE para Streaming em Tempo Real | `adrs/ADR-005-sse-real-time-streaming.md` |
| 006 | Redis Pub/Sub como Event Bus | `adrs/ADR-006-redis-pubsub-event-bus.md` |
| 007 | CLI Producer como Binário Separado | `adrs/ADR-007-cli-producer-separate-binary.md` |
| 008 | Consistência Eventual Redis ↔ DynamoDB | `adrs/ADR-008-eventual-consistency.md` |
| 009 | Worker Pool com Semáforo | `adrs/ADR-009-worker-pool-concurrency.md` |
| 010 | Dead Letter Queue Estratégia | `adrs/ADR-010-dlq-strategy.md` |
| 011 | OpenTelemetry para Observabilidade | `adrs/ADR-011-opentelemetry-observability.md` |
| 012 | Graceful Shutdown | `adrs/ADR-012-graceful-shutdown.md` |
| 013 | UI publica eventos via Producer HTTP | `adrs/ADR-013-ui-producer-http.md` |

## Licença

Distribuído sob a licença MIT. Veja o arquivo [`LICENSE`](LICENSE) para mais detalhes.
