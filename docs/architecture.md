# Arquitetura do Sistema

## Visão Geral

O **GoPayground** é um sistema distribuído de processamento de pagamentos baseado em microsserviços e mensageria assíncrona, construído como ambiente de experimentação em Go. A arquitetura segue o padrão **Event-Driven** com Kafka como espinha dorsal.

### Diagramas de Arquitetura

Os diagramas abaixo foram criados no formato Excalidraw e podem ser abertos em [https://excalidraw.com](https://excalidraw.com) para visualização interativa.

| Diagrama | Descrição | Arquivo |
|----------|-----------|---------|
| **Arquitetura Geral** | Visão geral do sistema: Kafka, Consumer, Redis, DynamoDB, UI e Observabilidade | [`diagrams/architecture-overview.excalidraw`](diagrams/architecture-overview.excalidraw) |
| **Fluxo de Processamento** | Pipeline detalhado do consumer: validação → idempotência → retry → DLQ | [`diagrams/message-processing-flow.excalidraw`](diagrams/message-processing-flow.excalidraw) |
| **Event Bus** | Fluxo Redis Pub/Sub: consumer → Redis → EventBus → SSE → Browser | [`diagrams/event-bus-flow.excalidraw`](diagrams/event-bus-flow.excalidraw) |
| **Data Stores** | Schema Redis (status + idempotência) e DynamoDB (histórico) | [`diagrams/data-stores.excalidraw`](diagrams/data-stores.excalidraw) |
| **Deploy Docker** | Topologia Docker Compose: serviços, portas e rede | [`diagrams/deployment-architecture.excalidraw`](diagrams/deployment-architecture.excalidraw) |
| **Observabilidade** | Stack OTel: aplicações → collector → batch → exporters (Jaeger + debug) | [`diagrams/observability-stack.excalidraw`](diagrams/observability-stack.excalidraw) |

### Diagrama de Componentes (visão textual)

O diagrama abaixo representa a arquitetura em formato ASCII. Para uma versão visual interativa, abra o arquivo [`diagrams/architecture-overview.excalidraw`](diagrams/architecture-overview.excalidraw).

```
╔══════════════════════════════════════════════════════════════════════╗
║                        KAFKA CLUSTER                                ║
║                                                                      ║
║  ┌──────────────────────────────────────┐  ┌──────────────────────┐  ║
║  │  payment.events (3 partições)        │  │ payment.events.dlq   │  ║
║  │  - Produtor: API externa, CLI        │  │  (1 partição)        │  ║
║  │  - Consumidor: payment-consumer-group │  │  - Mensagens com     │  ║
║  │  - Retenção: configurável             │  │    falha permanente   │  ║
║  └────────────────┬─────────────────────┘  └──────────▲───────────┘  ║
║                   │                                    │              ║
╚═══════════════════│════════════════════════════════════│══════════════╝
                    │                                    │
          Publica   │                           Publica  │
                    │                           (DLQ)   │
    ┌───────────────┴──────────┐                         │
    │                          │                         │
    ▼                          │                         │
┌──────────────────┐           │                         │
│  CLI Producer    │           │                         │
│  (cmd/producer)  │           │                         │
│  SyncProducer    │           │                         │
│  Valida antes    │           │                         │
│  de publicar     │           │                         │
└──────────────────┘           │                         │
                               │                         │
┌──────────────────────────────────────────────────────────────────────┐
│                         CONSUMER SERVICE                             │
│                         (cmd/consumer)                               │
│                                                                      │
│  ┌────────────────┐  ┌────────────────┐  ┌────────────────────┐     │
│  │  Validador     │  │  Idempotência  │  │  Status Updater    │     │
│  │  (go-validator) │  │  (Redis SET NX)│  │  (Redis HSET)      │     │
│  └────────┬───────┘  └───────┬────────┘  └──────────┬─────────┘     │
│           │                  │                      │                │
│  ┌────────┴──────────────────┴──────────────────────┴─────────┐     │
│  │                    Retry Handler                             │     │
│  │            (3 tentativas, backoff exp + jitter)              │     │
│  └────────┬─────────────────────────────────────────────┬───────┘     │
│           │                                              │           │
│     ┌─────┴─────┐                                  ┌────┴────┐     │
│     │  História  │                                  │   DLQ   │     │
│     │  DynamoDB  │                                  │  Kafka  │     │
│     └─────┬─────┘                                  └─────────┘     │
└───────────│──────────────────────────────────────────────────────────┘
            │
            │ Eventos via Redis Pub/Sub (canal: payment:events)
            │
┌───────────▼──────────────────────────────────────────────────────────┐
│                        PAYMENT UI                                    │
│                        (cmd/ui)                                      │
│                                                                      │
│  ┌────────────────┐  ┌────────────────┐  ┌────────────────────┐     │
│  │  Event Bus     │  │  REST API      │  │  SSE Handler       │     │
│  │  Redis Pub/Sub │  │  /api/payments │  │  /api/events       │     │
│  │  + subscribers │  │  /api/metrics  │  │  heartbeat 30s     │     │
│  └────────────────┘  └────────────────┘  └────────────────────┘     │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐    │
│  │           Frontend (Vanilla JS + HTML + CSS)                 │    │
│  │  ┌───────────┐  ┌────────────┐  ┌────────────┐              │    │
│  │  │ Live Feed │  │ Payments   │  │ History    │              │    │
│  │  │ (SSE)     │  │ Table      │  │ Modal      │              │    │
│  │  └───────────┘  └────────────┘  └────────────┘              │    │
│  └──────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────┘

╔══════════════════════════════════════════════════════════════════════╗
║                     DATA STORES                                     ║
║                                                                      ║
║  ┌──────────────┐    ┌────────────────────┐    ┌────────────────┐   ║
║  │    Redis     │    │     DynamoDB        │    │    Redis       │   ║
║  │  (Status)    │    │  (Payment History)  │    │ (Idempotência) │   ║
║  │  payment:id  │    │  payment_id (PK)    │    │ idempotency:id │   ║
║  │  HSET fields │    │  timestamp (SK)     │    │ SET NX + TTL   │   ║
║  │  TTL 7 dias  │    │  schemaless         │    │ TTL 24h        │   ║
║  └──────────────┘    └────────────────────┘    └────────────────┘   ║
╚══════════════════════════════════════════════════════════════════════╝
> 🖼️ **Diagrama visual disponível**: [`diagrams/data-stores.excalidraw`](diagrams/data-stores.excalidraw) — Schema detalhado do Redis e DynamoDB com chaves e relações.

╔══════════════════════════════════════════════════════════════════════╗
║                    OBSERVABILIDADE                                   ║
║                                                                      ║
║  ┌──────────────┐    ┌────────────────────┐    ┌────────────────┐   ║
║  │  Logs (slog) │    │  Tracing (OTel)    │    │  Métricas      │   ║
║  │  JSON output │    │  spans por mensagem│    │  counters +    │   ║
║  │  stdout      │    │  OTLP gRPC export  │    │  histograms    │   ║
║  └──────────────┘    └────────────────────┘    └────────────────┘   ║
║                                                                      ║
║  ┌──────────────────────────────────────────────────────────────┐    ║
║  │           OpenTelemetry Collector                             │    ║
║  │  OTLP Receiver → Batch Processor → Exporters (debug, Jaeger)  │    ║
║  └──────────────────────────────────────────────────────────────┘    ║
╚══════════════════════════════════════════════════════════════════════╝
> 🖼️ **Diagrama visual disponível**: [`diagrams/observability-stack.excalidraw`](diagrams/observability-stack.excalidraw) — Stack OTel completa com pipelines, exporters e hierarquia de spans.
```

## Fluxo de Dados Detalhado

> 🖼️ **Diagrama visual disponível**: [`diagrams/message-processing-flow.excalidraw`](diagrams/message-processing-flow.excalidraw) — Pipeline completo do consumer (validação → idempotência → retry → DLQ).

### 1. Publicação de Evento

```
[ Origem ] → Kafka Topic (payment.events)
             ├── CLI Producer (cmd/producer)
             │   ├── Valida no cliente (reusa validator)
             │   ├── Publica via SyncProducer
             │   └── Confirma partition + offset
             │
             └── API Externa / Gateway (fora do escopo)
                 ├── Publica no mesmo tópico
                 └── Mensagens seguem schema definido
```

### 2. Processamento (Consumer)

> 🖼️ **Diagrama visual disponível**: [`diagrams/message-processing-flow.excalidraw`](diagrams/message-processing-flow.excalidraw) — Fluxograma detalhado com decision boxes e caminhos de sucesso/falha.

```
Kafka Message
    │
    ▼
1. Acquire semaphore (worker pool)
    │
    ▼
2. Validate payload
    ├── Sucesso → continua
    └── Falha  → DLQ (erro permanente)
    │
    ▼
3. Idempotency Check (Redis)
    ├── Já processado → commit offset (skip)
    └── Novo → MarkProcessed (SET NX, TTL 24h)
    │
    ▼
4. Retry.Do (3 tentativas com backoff)
    ├── Atualiza Status (Redis HSET)
    ├── Registra Histórico (DynamoDB PutItem)
    │
    ├── Sucesso → commit offset
    └── Falha (exaustão) → DLQ
```

### 3. Consulta (UI)

> 🖼️ **Diagrama visual disponível**: [`diagrams/event-bus-flow.excalidraw`](diagrams/event-bus-flow.excalidraw) — Fluxo Redis Pub/Sub do consumer até o browser via SSE.

```
Browser
    │
    ├── GET / → index.html (embed.FS)
    │
    ├── GET /api/payments
    │   └── Redis: SCAN payment:* → HGETALL
    │
    ├── GET /api/payments/{id}/history
    │   └── DynamoDB: Query por payment_id
    │
    ├── GET /api/metrics
    │   └── Redis: SCAN + contagem por status
    │
    └── GET /api/events (SSE)
        └── EventBus ← Redis Pub/Sub (canal payment:events)
```

## Padrões Distribuídos Utilizados

| Padrão                    | Implementação                                      |
|---------------------------|----------------------------------------------------|
| **Event-Driven**          | Kafka como message broker central                  |
| **Consumer Group**        | `payment-consumer-group` para escalabilidade       |
| **Idempotência**          | Redis `SET NX` + DynamoDB `ConditionExpression`    |
| **Retry with Backoff**    | Exponencial (1x, 3x, 9x) + jitter (±25%)          |
| **Dead Letter Queue**     | Tópico Kafka dedicado `payment.events.dlq`         |
| **Worker Pool**           | Semáforo com `chan struct{}` (configurável)        |
| **Circuit Breaker**       | Preparado para DynamoDB (gobreaker)                 |
| **Graceful Shutdown**     | Context cancellation + signal handling             |
| **Health Check**          | Endpoints `/healthz` e `/readyz`                   |
| **Server-Sent Events**    | Streaming de eventos em tempo real para o browser  |
| **Pub/Sub**               | Redis Pub/Sub para desacoplar consumer e UI        |
| **Observabilidade**       | OpenTelemetry (tracing + métricas + logs)          |

## Decisões Arquiteturais (ADRs)

As decisões arquiteturais do projeto estão documentadas em formato ADR (Architecture Decision Record) formal no diretório [`adrs/`](../adrs/):

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

## Tradeoffs

| Decisão                 | Prós                                      | Contras                                    |
|-------------------------|-------------------------------------------|--------------------------------------------|
| **Consistência eventual** | Baixa latência, tolerante a falhas       | Risco de leitura defasada                  |
| **TTL idempotência 24h**  | Simples, automático                      | Mensagens >24h podem ser reprocessadas     |
| **Worker pool fixo**      | Previsível, controlado                   | Subutilização em baixa carga               |
| **Validação restritiva**  | Dados garantidos, segurança              | Mensagens fora do formato são rejeitadas   |
| **Vanilla JS na UI**      | Zero dependências, sem build step        | Mais código manual                         |

## Segurança

- **Secrets**: via variáveis de ambiente (nunca hardcoded)
- **Payload validation**: limite de 10 KB, sem caracteres de controle
- **Logs**: sem dados sensíveis (amount, descrições completas)
- **Headers de segurança**: `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`
- **Path traversal**: prevenido pelo `embed.FS`
