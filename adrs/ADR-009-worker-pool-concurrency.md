# Architecture Decision Record (ADR)

**ADR ID**: ADR-009  
**Title**: Worker Pool com Semáforo para Concorrência Controlada  
**Status**: Accepted  
**Date**: 2026-05-25  
**Owner**: Payment Platform Team  
**Deciders**: Architect, Senior Engineer  
**Scope**: Controle de concorrência no processamento de mensagens Kafka

---

## Core

### 1. Context

**Problem statement**: O consumidor Kafka (`cmd/consumer`) processa mensagens concorrentemente dentro do `ConsumeClaim`. O Kafka entrega mensagens em uma goroutine por partição, mas precisamos de controle fino sobre quantas mensagens são processadas simultaneamente para evitar sobrecarga no Redis, DynamoDB, e uso excessivo de memória.

**Goals**:
- Limitar processamento simultâneo de mensagens a um número configurável de workers
- Garantir que o shutdown respeite workers em andamento (drene antes de parar)
- Evitar race conditions no acesso a recursos compartilhados (Redis, DynamoDB)
- Utilizar eficientemente CPU e conexões de rede
- Permitir ajuste do nível de concorrência sem reinício (no futuro)

**Non-goals**:
- Agendamento complexo de workers (prioridade, filas)
- Worker pool com goroutine pool reutilizável (criamos goroutines por mensagem, controlamos com semáforo)
- Balanceamento de carga entre workers (Kafka consumer group já faz)
- Escalonamento automático de workers baseado em carga

**Constraints** (REQUIRED):
- Platform/runtime: Go 1.26, goroutines
- SLO: Throughput mínimo de 100 msg/s com 10 workers
- Resource limits: 10 workers simultâneos padrão

**Assumptions**:
- Kafka entrega mensagens mais rápido que os workers processam (precisamos de backpressure)
- Workers são I/O-bound (Redis + DynamoDB), não CPU-bound
- 10 workers padrão é adequado para carga inicial

### 2. Decision Drivers (What Matters Most)

| Priority | Driver | Why it matters | How we measure |
|---:|---|---|---|
| 1 | Controle de concorrência | Evitar sobrecarga | Número de goroutines ativas < worker_count |
| 2 | Backpressure natural | Semáforo bloqueia quando cheio | `select` com `chan struct{}` |
| 3 | Shutdown seguro | Workers drenam antes de sair | `ctx.Done()` no semáforo |
| 4 | Simplicidade | Chan como semáforo | 4 linhas de código (acquire + release) |

### 3. Options Considered

| Option | Summary | Pros | Cons | Reversibility |
|---|---|---|---|---|
| A - Semáforo com chan struct{} (escolhido) | `make(chan struct{}, workerCount)` | Simples, eficiente, integrado com context | Capacidade fixa, sem prioridade | Easy |
| B - Goroutine pool (ants) | Pool de goroutines reutilizáveis | Reuso de goroutines, métricas integradas | Dependência externa, complexidade desnecessária | Medium |
| C - WaitGroup sem limite | `sync.WaitGroup` sem semáforo | Máximo throughput | Sem controle de concorrência, risco de explosão de goroutines | Easy |
| D - Buffer de canais com worker pool | N workers escutam um canal de jobs | Controle fino, workers reutilizáveis | Mais código, gerenciamento de ciclo de vida dos workers | Medium |

### 4. Decision

**We choose**: Option A - Semáforo com `chan struct{}`

**Why**:
- Implementação em `internal/consumer/handler.go`:
  - Semáforo: `semaphore: make(chan struct{}, workerCount)` — linha 64
  - Acquire: `case h.semaphore <- struct{}{}:` — linha 194 (bloqueia se cheio)
  - Release: `defer func() { <-h.semaphore }()` — linha 198
  - Context cancellation durante acquire: `case <-ctx.Done(): return ctx.Err()` — linha 195-196
- 10 workers padrão (`worker_count: 10` em `config.go:60`) — configurável
- Semáforo como `chan struct{}` é padrão Go idiomático (effective Go)
- Integração nativa com `context.Context` para graceful shutdown
- Zero dependências externas

### 5. Architecture Impact (Implementation-Ready)

**Boundaries and contracts**
- Public APIs/contracts affected:
  - `consumer.NewHandler(..., workerCount int, ...)` — parâmetro `workerCount` define o tamanho do semáforo
  - Config: `worker_count` (default 10)
- Backward compatibility plan: N/A
- Schema evolution strategy: N/A

**Data and consistency**
- Source of truth: N/A
- Consistency model: N/A
- Migration strategy: N/A

**Failure modes and resilience**
- Known failure modes:
  - Semáforo cheio → `processMessage` bloqueia em `h.semaphore <- struct{}{}` — backpressure natural contra Kafka
  - Contexto cancelado durante acquire → retorna `ctx.Err()` — worker não inicia, Kafka offset não é commitado
  - Panic no worker → `defer` libera semáforo (Go garante execução de defer)
  - Worker lento → semáforo cheio → Kafka consumer pausa (backpressure)
- Timeouts/retries/backoff policy: Sem timeout no acquire (bloqueia até slot disponível)
- Idempotency strategy: N/A (workers processam mensagens diferentes)
- Degradation plan: Se `worker_count` = 1, sistema opera em série (modo seguro)

**Security**
- Threat model summary: N/A
- AuthN/AuthZ model: N/A
- Secret and key management: N/A
- Audit logging requirements: N/A

**Observability**
- SLIs/SLOs: Workers ativos < worker_count
- Metrics/traces/logs to add:
  - Métrica de workers ocupados (via `len(h.semaphore)`)
  - Logs de session start/stop com member_id e generation_id (`handler.go:121-135`)
- Dashboards and alerts: Se workers consistentemente no limite, aumentar worker_count

**Cost and capacity**
- Expected traffic/load: 10 workers processam ~100 msg/s (estimado 100ms por operação)
- Cost model: Cada worker = 1 goroutine + 1 conexão Redis + 1 conexão DynamoDB
- Capacity plan: worker_count ajustável via config.yaml

### 6. Rollout, Validation, and Rollback

**Rollout plan**
- Feature flag / staged rollout: N/A
- Data migration steps: N/A
- Runbook updates: Ajustar `worker_count` conforme necessidade de throughput

**Validation plan**
- Tests to add (unit/integration/contract):
  - `internal/consumer/handler_test.go` — testes com diferentes worker_counts
  - Testes de concorrência com -race
- Load/perf tests: Benchmark throughput com 1, 5, 10, 20 workers

**Rollback plan**
- How to revert code: `git revert`
- How to revert data (or forward-fix): N/A
- Timebox for rollback decision: 15 min

### 7. Consequences

**Positive**
- Apenas 4 linhas de código para controle de concorrência (acquire + release com defer)
- Backpressure natural: semáforo cheio → `ConsumeClaim` bloqueia → Kafka pausa entrega
- Shutdown graceful: contexto cancelado → acquire retorna erro → worker não inicia ou termina
- Configurável: basta mudar `worker_count` no config.yaml
- Go idiomático: padrão semáforo com `chan struct{}` é amplamente conhecido

**Negative / tradeoffs**
- Capacidade fixa em compilação (precisa restart para mudar)
- Semáforo não diferencia tipos de operação (status vs history têm pesos diferentes)
- Se um worker travar (deadlock), ocupa slot permanentemente (mas semáforo + defer previne isso)
- Sem métricas expostas do nível de ocupação do semáforo atualmente

**Follow-ups**
- Expor `len(semaphore)` como gauge métrica OTel (Owner: Hardening Engineer, Q2 2026)
- Considerar worker pool dinâmico com base em latência (Owner: Senior Engineer, Q4 2026)

### 8. Links

- Código: `internal/consumer/handler.go` — linha 32 (declaração), 64 (criação), 193-198 (acquire/release)
- Config: `worker_count: 10` (em `config.go:60`)
- Makefile: linha 4 (`go test ./... -race`)
- Testes: `internal/consumer/handler_test.go`
- Relacionados: ADR-001 (Kafka), ADR-004 (retry)

---

## References

- ADR concept and template rationale: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
- Go Concurrency Patterns: https://go.dev/blog/pipelines
