# Architecture Decision Record (ADR)

**ADR ID**: ADR-011  
**Title**: OpenTelemetry para Observabilidade Distribuída  
**Status**: Accepted  
**Date**: 2026-05-25  
**Owner**: Payment Platform Team  
**Deciders**: Architect, Senior Engineer  
**Scope**: Tracing distribuído, métricas e logs em todos os componentes

---

## Core

### 1. Context

**Problem statement**: O sistema tem múltiplos componentes distribuídos (Kafka consumer, CLI producer, UI server, Redis, DynamoDB) que precisam de observabilidade para diagnóstico de problemas, monitoramento de performance, e rastreamento de fluxos de ponta-a-ponta. Uma mensagem passa por validação → idempotência → Redis status → DynamoDB history → EventBus → SSE, e precisamos correlacionar todos esses passos.

**Goals**:
- Rastrear uma mensagem do Kafka até a UI (trace distribuído)
- Capturar métricas de negócio (mensagens processadas, erros, latência)
- Log estruturado em JSON para correlação com tracing
- Exportar traces e métricas via OTLP para collector externo
- Baixo overhead em produção (< 5% de CPU adicional)

**Non-goals**:
- APM completo (Dashboards, alertas — fora do escopo desta ADR)
- Profiling contínuo (pprof)
- Monitoramento de infraestrutura (Kafka, Redis, DynamoDB)
- Log aggregation (fora do escopo)

**Constraints** (REQUIRED):
- Platform/runtime: Go 1.26, OTel SDK v1.43.0
- Export: OTLP gRPC para collector local (localhost:4317)
- Cost: OTel SDK é open source, collector roda em Docker
- Team/operational maturity: Time com experiência em OTel

**Assumptions**:
- OTel Collector será usado em produção (roteamento para Jaeger, Prometheus, etc.)
- AlwaysSample() é aceitável em desenvolvimento; em produção usar rate limiting
- Logs estruturados em JSON são suficientes (sem OTel logs SDK)

### 2. Decision Drivers (What Matters Most)

| Priority | Driver | Why it matters | How we measure |
|---:|---|---|---|
| 1 | Rastreabilidade | Correlacionar operações entre serviços | Trace ID presente em todos os componentes |
| 2 | Baixo overhead | Não impactar throughput | < 5% CPU adicional |
| 3 | Padronização | OTel é padrão da indústria | Suporte a múltiplos backends |
| 4 | Métricas de negócio | Saber volume e saúde do sistema | Counters + histograms |

### 3. Options Considered

| Option | Summary | Pros | Cons | Reversibility |
|---|---|---|---|---|
| A - OpenTelemetry (escolhido) | OTel SDK + Collector | Padrão da indústria, multi-backend, baixo overhead, rico em features | Complexidade de configuração, dependência do collector | Medium |
| B - Prometheus + Jaeger direto | Prometheus client + Jaeger client | Mais simples que OTel, sem collector | Vendor lock-in, dois agentes, sem integração padronizada | Hard |
| C - Apenas logs estruturados | slog JSON + correlation ID | Máxima simplicidade | Sem tracing distribuído, sem métricas automáticas | Easy |
| D - Datadog APM | Agente Datadog | Tudo integrado, rico em features | Vendor lock-in, custo, sem suporte offline (Floci) | Hard |

### 4. Decision

**We choose**: Option A - OpenTelemetry

**Why**:
- `pkg/telemetry/telemetry.go` inicializa `TracerProvider` e `MeterProvider` com export OTLP gRPC — ~90 linhas
- Traces: `sdktrace.AlwaysSample()` + batcher com 5s timeout — `telemetry.go:42-46`
- Métricas: `sdkmetric.NewPeriodicReader` com intervalo de 10s — `telemetry.go:73-75`
- Traces capturados no consumer em `handler.go:148-157` com span `"process_message"` e atributos: offset, partition, messaging.system, messaging.destination
- Métricas implementadas no consumer como counters e histogram:
  - `payment.consumer.messages_received` (counter) — `handler.go:76`
  - `payment.consumer.messages_processed` (counter) — `handler.go:83`
  - `payment.consumer.processing_duration` (histogram, buckets: 1,5,10,25,50,100,250,500,1000ms) — `handler.go:90`
  - `payment.consumer.retry_attempts` (counter) — `handler.go:99`
  - `payment.consumer.dlq_published` (counter) — `handler.go:105`
  - `payment.consumer.idempotency_hits` (counter) — `handler.go:112`
- `trace_id` propagado para DynamoDB (`dynamodb.go:40-41`) e DLQ headers (`dlq/producer.go:35-38`)
- Collector configurado em `otel-collector-config.yaml` com pipeline traces + metrics e batch processor

### 5. Architecture Impact (Implementation-Ready)

**Boundaries and contracts**
- Public APIs/contracts affected:
  - `pkg/telemetry.InitTracerProvider(ctx, cfg)` — inicializa tracing
  - `pkg/telemetry.InitMeterProvider(ctx, cfg)` — inicializa métricas
  - `pkg/telemetry.NewMeter(name)` / `pkg/telemetry.NewTracer(name)` — factories
  - Config: `otel_exporter_otlp_endpoint`, `otel_service_name`
  - Collector: OTLP receiver na porta 4317 (gRPC) e 4318 (HTTP)
- Backward compatibility plan: N/A
- Schema evolution strategy: Atributos OTel podem ser adicionados sem quebrar existentes

**Data and consistency**
- Source of truth: N/A (observabilidade)
- Consistency model: N/A
- Migration strategy: N/A

**Failure modes and resilience**
- Known failure modes:
  - Collector indisponível → OTel SDK descarta dados (non-blocking, sem retry)
  - Falha no InitTracerProvider → consumer exit code 1 (`cmd/consumer/main.go:47-49`)
  - Falha no InitMeterProvider → consumer exit code 1 (`cmd/consumer/main.go:60-62`)
  - Métrica counter falha → apenas warn log (handler.go:80)
- Timeouts/retries/backoff policy: Batcher OTel com 5s timeout
- Idempotency strategy: N/A
- Degradation plan: Sem OTel, sistema funciona sem observabilidade

**Security**
- Threat model summary: OTLP sem TLS (`otlptracegrpc.WithInsecure()`) — apenas em rede interna Docker
- AuthN/AuthZ model: N/A
- Secret and key management: N/A
- Audit logging requirements: `trace_id` armazenado no DynamoDB como registro de auditoria

**Observability**
- SLIs/SLOs: TODO (definir após baseline)
- Metrics/traces/logs to add: (já implementados)
  - 6 métricas no consumer
  - 3 atributos de span (offset, partition, messaging)
  - Logs JSON com slog (`slog.NewJSONHandler`)
- Dashboards and alerts: N/A (fora do escopo)

**Cost and capacity**
- Expected traffic/load: 1 trace por mensagem, ~10 spans por trace
- Cost model: Collector em Docker (~256MB RAM, 0.5 CPU)
- Capacity plan: Se > 1000 msg/s, implementar rate limiting de sampling

### 6. Rollout, Validation, and Rollback

**Rollout plan**
- Feature flag / staged rollout: N/A
- Data migration steps: N/A
- Runbook updates: Verificar `otel_exporter_otlp_endpoint` no config.yaml, `otel-collector-config.yaml` no docker-compose

**Validation plan**
- Tests to add (unit/integration/contract):
  - `pkg/telemetry/telemetry_test.go` — testes de inicialização
  - Testes manuais: verificar traces no Jaeger e métricas no Prometheus
- Load/perf tests: Medir overhead do OTel (CPU, memória)

**Rollback plan**
- How to revert code: `git revert`
- How to revert data (or forward-fix): N/A
- Timebox for rollback decision: 30 min

### 7. Consequences

**Positive**
- Trace distribuído do Kafka → processamento → DynamoDB: span `"process_message"` em `handler.go:148`
- `trace_id` armazenado no DynamoDB (`dynamodb.go:40-41`) permite auditoria completa
- `trace_id` nos headers DLQ (`dlq/producer.go:48-52`) permite rastrear origem da falha
- 6 métricas de negócio no consumer permitem monitorar saúde do sistema
- Histogram `processing_duration` com buckets customizados (1-1000ms) — `handler.go:92`
- Logs JSON com `slog.NewJSONHandler` — `cmd/consumer/main.go:37`
- Graceful shutdown dos providers com timeout configurável (`telemetry.go:51-55`, `telemetry.go:64-68`)

**Negative / tradeoffs**
- OTel SDK adiciona ~5MB ao binário compilado
- InitTracerProvider falha → exit code 1 (não permite degradação)
- `AlwaysSample()` em produção pode gerar volume alto de traces (> 1000 msg/s)
- Collector é mais um componente para operar (mesmo que leve)
- CLI producer não tem OTel (ADR-007) — eventos CLI não geram traces

**Follow-ups**
- Implementar rate limiting no sampler para produção (`sdktrace.RatioBasedSampler`) (Owner: Senior Engineer, Q2 2026)
- Adicionar tracing no Redis e DynamoDB via instrumentação automática (Owner: Senior Engineer, Q3 2026)
- Criar dashboard Grafana com as métricas do consumer (Owner: DevOps, Q3 2026)
- Adicionar OTel ao CLI producer (Owner: Senior Engineer, Q3 2026)

### 8. Links

- Código: `pkg/telemetry/telemetry.go` — inicialização OTel (90 linhas)
- Código: `cmd/consumer/main.go` — linhas 45-72 (inicialização + shutdown)
- Código: `internal/consumer/handler.go` — linhas 73-118 (6 métricas), 148-157 (span), 159-184 (métrica recording)
- Código: `otel-collector-config.yaml` — configuração do collector
- Código: `docker-compose.yml` — linhas 60-71 (serviço otel-collector)
- Config: `otel_exporter_otlp_endpoint=localhost:4317`, `otel_service_name=payment-consumer`
- Testes: `pkg/telemetry/telemetry_test.go`
- Relacionados: Todos os ADRs (observabilidade transversal)

---

## References

- ADR concept and template rationale: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
- OpenTelemetry Go: https://opentelemetry.io/docs/languages/go/
- OpenTelemetry Collector: https://opentelemetry.io/docs/collector/
