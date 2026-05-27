# Architecture Decision Record (ADR)

**ADR ID**: ADR-012  
**Title**: Graceful Shutdown com Context Cancellation e Signal Handling  
**Status**: Accepted  
**Date**: 2026-05-25  
**Owner**: Payment Platform Team  
**Deciders**: Architect, Senior Engineer  
**Scope**: Encerramento gracioso de todos os componentes (consumer, ui, producer)

---

## Core

### 1. Context

**Problem statement**: Todos os três componentes do sistema (consumer, UI, CLI producer) precisam ser encerrados graciosamente ao receber sinais SIGINT/SIGTERM. O encerramento abrupto pode causar perda de mensagens em processamento (consumer), conexões SSE interrompidas (UI), ou eventos parcialmente publicados (CLI). É necessário um padrão consistente de shutdown para todos os entry points.

**Goals**:
- Finalizar processamento de mensagens em andamento antes de sair (consumer)
- Fechar conexões Redis, Kafka, DynamoDB ordenadamente
- Notificar clientes SSE sobre o encerramento (ou apenas fechar conexões)
- Completar publicações em andamento (CLI producer)
- Respeitar timeout configurável para shutdown forçado

**Non-goals**:
- Shutdown de componentes de infraestrutura (Kafka, Redis, DynamoDB)
- State persistence entre restarts
- Rolling updates com zero downtime (fora do escopo atual)
- Health check endpoint para liveness probe (Kubernetes)

**Constraints** (REQUIRED):
- Platform/runtime: Go 1.26, signal.Notify
- SLO: Shutdown completo em < 30s (configurável via `graceful_shutdown_timeout`)
- Team/operational maturity: Padrão Go bem estabelecido

**Assumptions**:
- Sinais SIGINT (Ctrl+C) e SIGTERM (Docker stop/kill) são os únicos necessários
- Consumer pode estar no meio de um retry com backoff — deve respeitar cancelamento
- UI pode ter conexões SSE ativas — devem ser fechadas

### 2. Decision Drivers (What Matters Most)

| Priority | Driver | Why it matters | How we measure |
|---:|---|---|---|
| 1 | Perda zero de mensagens | Mensagens em processamento não podem ser perdidas | Idempotência garante reprocessamento |
| 2 | Fechamento ordenado | Recursos liberados corretamente | defer + Shutdown calls |
| 3 | Timeout | Não travar infinitamente | `graceful_shutdown_timeout: 30s` |
| 4 | Consistência | Mesmo padrão nos 3 entry points | Todos usam signal.Notify + context |

### 3. Options Considered

| Option | Summary | Pros | Cons | Reversibility |
|---|---|---|---|---|
| A - Context cancellation + signal.Notify (escolhido) | signal.Notify → cancel → shutdown | Padrão Go idiomático, funciona em todos os componentes | Precisa de defer para cada recurso | Easy |
| B - http.Server.Shutdown only | Apenas para UI server | Simples, adequado para HTTP | Não cobre consumer (Kafka) nem CLI | N/A (limitado) |
| C - syscall.Exec + supervisor | Mata e reinicia via supervisor | Zero esforço de shutdown | Perda de mensagens garantida, anti-pattern | Hard |
| D - Grace period com goroutine monitor | Goroutine dedicada monitora tempo de shutdown | Controle fino de deadline | Complexidade extra desnecessária | Medium |

### 4. Decision

**We choose**: Option A - Context cancellation + signal.Notify

**Why**:
- **Consumer** (`cmd/consumer/main.go`):
  - `signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)` — linha 162
  - Goroutine escuta sinal → `consumeCancel()` — linha 175
  - `consumerGroup.Consume(consumeCtx, ...)` respeita contexto — linha 193
  - `defer` para fechar consumerGroup, syncProducer, rdb, tp, mp — linhas 51-68, 80-83, 122-126, 132-137
  - Health server NÃO faz graceful shutdown (goroutine simples)
- **UI** (`cmd/ui/main.go`):
  - `signal.Notify(sigCh, ...)` — linha 61
  - Goroutine → `server.Shutdown(ctx)` com timeout de 15s — linha 68
  - `Server.Shutdown` → `eventBus.Close()` + `httpServer.Shutdown()` — `server.go:75-79`
- **CLI Producer** (`cmd/producer/main.go`):
  - `signal.Notify(sigCh, ...)` — linha 249
  - Goroutine → `cancel()` — linha 253
  - `Publish(ctx, events, rate)` respeita contexto — `producer.go:55-62`
- Worker pool com semáforo (ADR-009) respeita contexto via `ctx.Done()` — `handler.go:195-196`
- Retry handler (ADR-004) respeita contexto via `select` — `retry/handler.go:42-46`

### 5. Architecture Impact (Implementation-Ready)

**Boundaries and contracts**
- Public APIs/contracts affected:
  - `consumer.Handler.Setup`/`Cleanup` — session lifecycle (handler.go:121-136)
  - `ui.Server.Shutdown(ctx)` — graceful shutdown (server.go:75-79)
  - `ui.EventBus.Close()` — cleanup subscribers (events.go:109-117)
  - DLQ publish com context timeout (dlq/producer.go:70-84)
- Backward compatibility plan: N/A
- Schema evolution strategy: N/A

**Data and consistency**
- Source of truth: N/A
- Consistency model: N/A
- Migration strategy: N/A

**Failure modes and resilience**
- Known failure modes:
  - Shutdown timeout excedido → `http.Server.Shutdown` força fechamento (`cmd/ui/main.go:68`)
  - Consumer em meio a retry → `ctx.Done()` interrompe backoff (`retry/handler.go:42-46`)
  - Kafka consumerGroup fechando → session encerrada, `Cleanup` chamado (`handler.go:130`)
  - EventBus com subscribers ativos → todos fechados via `Close()` (`events.go:109-117`)
- Timeouts/retries/backoff policy:
  - `graceful_shutdown_timeout: 30s` (consumer) — `config.go:32`
  - Timeout de 15s para UI server shutdown (hardcoded em `cmd/ui/main.go:66`)
  - Tracer provider shutdown com timeout configurável (`telemetry.go:51`)
  - Meter provider shutdown com timeout configurável (`telemetry.go:64`)
- Idempotency strategy: Mensagens não commitadas são reprocessadas (idempotência garante)
- Degradation plan: Se shutdown travar, timeout força saída

**Security**
- Threat model summary: N/A
- AuthN/AuthZ model: N/A
- Secret and key management: N/A
- Audit logging requirements: Logs de shutdown em cada componente

**Observability**
- SLIs/SLOs: Shutdown < 30s
- Metrics/traces/logs to add:
  - Log: `"shutdown signal received"` — todos os entry points
  - Log: `"consumer session ended"` — `handler.go:131`
  - Log: `"shutting down UI server"` — `server.go:76`
- Dashboards and alerts: N/A

**Cost and capacity**
- Expected traffic/load: N/A (shutdown é raro)
- Cost model: N/A
- Capacity plan: N/A

### 6. Rollout, Validation, and Rollback

**Rollout plan**
- Feature flag / staged rollout: N/A
- Data migration steps: N/A
- Runbook updates: Docker stop/kill envia SIGTERM, shutdown é automático

**Validation plan**
- Tests to add (unit/integration/contract):
  - Testes de integração: enviar SIGTERM e verificar shutdown ordenado
  - Testes de timeout: verificar que shutdown não trava
- Load/perf tests: N/A

**Rollback plan**
- How to revert code: `git revert`
- How to revert data (or forward-fix): Mensagens não commitadas serão reprocessadas
- Timebox for rollback decision: 30 min

### 7. Consequences

**Positive**
- Todos os 3 entry points implementam o mesmo padrão: `signal.Notify` + `context.WithCancel`
- Consumer loop usa `consumeCtx` que é cancelado no sinal → `consumerGroup.Consume` retorna — `cmd/consumer/main.go:193`
- Retry handler respeita `ctx.Done()` entre tentativas — `retry/handler.go:42-46`
- Worker pool (semáforo) respeita `ctx.Done()` no acquire — `handler.go:195-196`
- EventBus.Close() fecha todos os subscribers e canais — `events.go:109-117`
- OTel providers têm shutdown explícito com timeout — `telemetry.go:51-55`
- DLQ publish com timeout via goroutine separada — `dlq/producer.go:70-84`
- UI server `http.Server.Shutdown` com timeout de 15s — `cmd/ui/main.go:66`

**Negative / tradeoffs**
- CLI producer usa `context.WithCancel` mas `Publish` pode deixar eventos não publicados se cancelado no meio (`producer.go:55-62`)
- Health server no consumer (`startHealthServer`) NÃO tem graceful shutdown — é uma goroutine simples que ignora shutdown
- EventBus.Close() fecha subscribers mas não notifica clientes SSE — conexão é apenas fechada
- Timeout de 15s na UI pode ser curto se houver muitas conexões SSE para drenar
- Consumer loop tem `for { select { case <-consumeCtx.Done(): return } }` — pode levar até 1 ciclo para sair (se estiver em Consume)

**Follow-ups**
- Adicionar graceful shutdown ao health server do consumer (Owner: Senior Engineer, Q2 2026)
- Centralizar shutdown pattern em pacote `internal/shutdown` (Owner: Senior Engineer, Q3 2026)
- Enviar evento `shutdown` SSE para clientes antes de fechar (Owner: Senior Engineer, Q3 2026)

### 8. Links

- Código: `cmd/consumer/main.go` — linhas 161-199 (shutdown do consumer)
- Código: `cmd/ui/main.go` — linhas 60-78 (shutdown da UI)
- Código: `cmd/producer/main.go` — linhas 248-254 (shutdown do CLI)
- Código: `internal/ui/server.go` — linhas 75-79 (Shutdown method)
- Código: `pkg/telemetry/telemetry.go` — linhas 51-55, 64-68 (OTel shutdown)
- Config: `graceful_shutdown_timeout: 30s` (em `config.go:32`)
- Relacionados: ADR-004 (retry with context), ADR-009 (worker pool), ADR-006 (EventBus.Close)

---

## References

- ADR concept and template rationale: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
- Go signal handling: https://pkg.go.dev/os/signal
- Go http.Server.Shutdown: https://pkg.go.dev/net/http#Server.Shutdown
