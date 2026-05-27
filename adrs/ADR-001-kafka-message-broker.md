# Architecture Decision Record (ADR)

**ADR ID**: ADR-001  
**Title**: Apache Kafka como Message Broker Central  
**Status**: Accepted  
**Date**: 2026-05-25  
**Owner**: Payment Platform Team  
**Deciders**: Architect, Senior Engineer  
**Scope**: Sistema de mensageria distribuída (consumer, producer, UI)

---

## Core

### 1. Context

**Problem statement**: O sistema precisa de um message broker distribuído que suporte comunicação assíncrona entre produtores (CLI, APIs externas) e consumidores (serviço de processamento), com garantia de entrega *at-least-once*, ordenação por chave, e capacidade de reprocessamento.

**Goals**:
- Garantir que toda mensagem publicada seja processada pelo menos uma vez
- Suportar múltiplos consumidores concorrentes com rebalanceamento automático
- Permitir reprodução de eventos através de retenção configurável no tópico
- Oferecer baixa latência (< 100ms P99) para publicação e consumo
- Separar produtores de consumidores para evolução independente

**Non-goals**:
- Garantia de ordem global entre partições (apenas por chave de payment_id)
- Suporte a mensagens transacionais entre tópicos
- Dead Letter Queue via Kafka (coberto em ADR-010)
- Cluster multi-broker em produção (single node para desenvolvimento)

**Constraints** (REQUIRED):
- Regulatory/compliance: N/A
- Latency/SLO: Publicação em até 100ms, consumo com latência máxima de 30s (configurado via `Consumer.MaxProcessingTime`)
- Data residency: N/A
- Platform/runtime: Docker Compose, Go 1.26
- Team/operational maturity: Time pequeno, baixa necessidade de operação de cluster

**Assumptions**:
- Kafka rodando em single-broker em desenvolvimento; produção usará cluster gerenciado (Confluent/MSK)
- Tópicos criados via auto-criação (`KAFKA_AUTO_CREATE_TOPICS_ENABLE=true`)
- Não há necessidade de Kafka Streams ou ksqlDB

### 2. Decision Drivers (What Matters Most)

| Priority | Driver | Why it matters | How we measure |
|---:|---|---|---|
| 1 | Durabilidade | Mensagens não podem ser perdidas | Offset committed após processamento |
| 2 | Escalabilidade | Consumidores em grupo com paralelismo | Consumer group lag, throughput |
| 3 | Simplicidade operacional | Time pequeno precisa de setup mínimo | Tempo de docker-compose up |
| 4 | Custo | Sem dependências gerenciadas caras em dev | Apenas imagem Docker |

### 3. Options Considered

| Option | Summary | Pros | Cons | Reversibility |
|---|---|---|---|---|
| A - Apache Kafka | Broker maduro, sarama-go, consumer groups | Ordenação por chave, retenção, reprocessamento, ecosistema maduro | Operação mais complexa, maior consumo de RAM | Medium (trocar de broker exige migração de tópicos) |
| B - RabbitMQ | Mensageria AMQP clássica | Simples de operar, suporte a exchanges | Sem partições, sem retenção de longo prazo, consumidor concorrente limitado | Medium |
| C - Redis Streams | Streams nativos do Redis | Zero dependência extra (Redis já usado), baixa latência | Sem consumer groups avançados, sem rebalanceamento, sem retenção configurável | Easy (apenas mudar producer/consumer) |
| D - NATS | Mensageria leve em Go | Performance excepcional, simples, cluster nativo | Sem ordenação por chave, sem retenção, ecossistema menor | Medium |

### 4. Decision

**We choose**: Option A - Apache Kafka

**Why**:
- Consumer groups com rebalanceamento automático permitem escalar horizontalmente `cmd/consumer`
- Ordenação por chave (payment_id) é requisito fundamental — mesma partição para mesmo payment_id
- Retenção no tópico permite reprocessamento de eventos passados
- `sarama` (v1.49.0) é biblioteca Go madura, suporta SyncProducer e ConsumerGroup
- Três partições configuradas (`KAFKA_NUM_PARTITIONS=3`) permitem paralelismo igual ao `worker_count` padrão de 10

### 5. Architecture Impact (Implementation-Ready)

**Boundaries and contracts**
- Public APIs/contracts affected: Tópico `payment.events` (3 partições) e `payment.events.dlq` (1 partição)
- Backward compatibility plan: Schema via `PaymentEvent` em JSON com `validate:"required,uuid4"` para `payment_id`
- Schema evolution strategy: Novos campos opcionais no JSON; validador rejeita campos desconhecidos não mapeados

**Data and consistency**
- Source of truth: Kafka commit log (offset gerenciado pelo consumer group)
- Consistency model: At-least-once (idempotência no Redis garante exactly-once na prática)
- Migration strategy: N/A (primeira implementação)

**Failure modes and resilience**
- Known failure modes:
  - Brokers indisponíveis → producer retorna erro, consumer entra em loop de retry com `time.Sleep(1s)`
  - Partição offline → sarama gerencia automaticamente
  - Rebalanceamento em andamento → consumer session é pausada, `Setup`/`Cleanup` são chamados
- Timeouts/retries/backoff policy:
  - `Consumer.MaxProcessingTime = 30 * time.Second` (configurado em `cmd/consumer/main.go:109`)
  - `Producer.Timeout = 10 * time.Second` (configurado em `cmd/producer/main.go:271`)
  - `Net.DialTimeout = 5 * time.Second` (configurado em `cmd/producer/main.go:272`)
  - `config.Producer.MaxMessageBytes = 100 * 1024` (configurado em `cmd/producer/main.go:273`)
- Idempotency strategy: Idempotência via Redis (ADR-002), NÃO via `Producer.Idempotent = true`
- Degradation plan: Sem Kafka, todo o sistema para (dependência crítica)

**Security**
- Threat model summary: Cluster Kafka sem autenticação (PLAINTEXT), apenas em rede interna Docker
- AuthN/AuthZ model: Nenhum (ambiente controlado)
- Secret and key management: N/A
- Audit logging requirements: Mensagens incluem header `source` (e.g., `cli-producer`) e `timestamp`

**Observability**
- SLIs/SLOs: Consumer lag < 1000 mensagens, throughput > 100 msg/s
- Metrics/traces/logs to add:
  - `payment.consumer.messages_received` (counter por partição) — `handler.go:76`
  - `payment.consumer.messages_processed` (counter por status) — `handler.go:83`
  - `payment.consumer.processing_duration` (histogram, buckets: 1,5,10,25,50,100,250,500,1000ms) — `handler.go:90`
  - Atributos OpenTelemetry: offset, partition, messaging.system, messaging.destination — `handler.go:149-157`
- Dashboards and alerts: Lag alert se > 10k mensagens

**Cost and capacity**
- Expected traffic/load: Centenas a milhares de eventos/dia
- Cost model: Docker image `apache/kafka:latest` (~2GB RAM em dev)
- Capacity plan: 3 partições, 1 consumidor, 10 workers concorrentes

### 6. Rollout, Validation, and Rollback

**Rollout plan**
- Feature flag / staged rollout: N/A (primeira implementação)
- Data migration steps: N/A
- Runbook updates: `docker-compose up -d` sobe Kafka junto

**Validation plan**
- Tests to add (unit/integration/contract):
  - `cmd/consumer/main_test.go` — testes de integração com Kafka mock
  - `internal/consumer/handler_test.go` — testes unitários do handler
  - `internal/producer/producer_test.go` — testes do producer
  - `internal/dlq/producer_test.go` — testes do DLQ producer
- Load/perf tests: Não implementados ainda
- Chaos/failure injection: N/A

**Rollback plan**
- How to revert code: `git revert` + rebuild
- How to revert data (or forward-fix): Reconsumir do offset inicial com `Consumer.Offsets.Initial = sarama.OffsetOldest`
- Timebox for rollback decision: 30 min após deploy

### 7. Consequences

**Positive**
- Consumer group `payment-consumer-group` permite escalar instâncias do consumer horizontalmente
- Três partições para `payment.events` distribuem carga e permitem paralelismo
- Offset gerenciado pelo Kafka permite exatamente um processamento por mensagem (com idempotência)
- Tópico DLQ separado (`payment.events.dlq`) isola mensagens com falha permanente

**Negative / tradeoffs**
- Complexidade operacional: Kafka é o componente mais pesado do stack (~2GB RAM)
- Mensagens não podem ser consumidas fora de ordem (intencional, por design)
- Dependência externa: sistema não funciona sem Kafka

**Follow-ups**
- Configurar produção com Kafka gerenciado (Confluent/MSK) e autenticação SASL (Owner: DevOps, Q3 2026)
- Avaliar necessidade de aumentar `KAFKA_NUM_PARTITIONS` sob carga (Owner: Senior Engineer, Q4 2026)

### 8. Links

- Código: `cmd/consumer/main.go` — linhas 104-127 (configuração sarama)
- Código: `cmd/producer/main.go` — linhas 269-273 (configuração producer)
- Código: `docker-compose.yml` — linhas 4-21 (serviço Kafka)
- Código: `internal/consumer/handler.go` — linhas 139-189 (ConsumeClaim)
- Config: `config.yaml` → `kafka_brokers=localhost:9092`, `kafka_topic=payment.events`, `kafka_consumer_group=payment-consumer-group`
- Testes: `internal/consumer/handler_test.go`, `internal/producer/producer_test.go`
- Relacionados: ADR-004 (retry), ADR-010 (DLQ), ADR-009 (worker pool)

---

## References

- ADR concept and template rationale: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
- Sarama library: https://github.com/IBM/sarama
