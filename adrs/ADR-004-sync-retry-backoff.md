# Architecture Decision Record (ADR)

**ADR ID**: ADR-004  
**Title**: Retry Síncrono com Exponential Backoff + Jitter  
**Status**: Accepted  
**Date**: 2026-05-25  
**Owner**: Payment Platform Team  
**Deciders**: Architect, Senior Engineer  
**Scope**: Mecanismo de retry para operações de status (Redis) e histórico (DynamoDB)

---

## Core

### 1. Context

**Problem statement**: Operações de atualização de status (Redis) e registro de histórico (DynamoDB) podem falhar transientemente devido a timeouts de rede, contenção de recursos, ou indisponibilidade temporária dos serviços. O sistema precisa de um mecanismo de retry que maximize a chance de sucesso sem introduzir complexidade de repúblicação no Kafka.

**Goals**:
- Retentar operações falhas com backoff exponencial + jitter para evitar thundering herd
- Limitar o número máximo de tentativas para evitar loops infinitos
- Preservar o contexto da mensagem original durante todas as tentativas
- Cancelar retry se o contexto for cancelado (graceful shutdown)
- não repúblicar no Kafka — retry é síncrono no mesmo worker

**Non-goals**:
- Retry de mensagens inteiras via repúblicação no Kafka
- Circuit breaker (separado para DynamoDB, fora do escopo desta ADR)
- Retry assimétrico (ex: threads separadas para retry)
- Persistência de tentativas entre restart do consumer

**Constraints** (REQUIRED):
- Latency/SLO: Cada tentativa adiciona delay exponencial: 100ms, 300ms, 900ms (3 tentativas = ~1.3s máximo)
- Platform/runtime: Go 1.26, goroutine do worker
- Team/operational maturity: Implementação simples, sem dependências externas

**Assumptions**:
- Falhas são transientes (timeout, overload) — retry com backoff resolve
- 3 tentativas são suficientes para falhas transientes
- Jitter de ±25% evita contention em cenários de múltiplos workers

### 2. Decision Drivers (What Matters Most)

| Priority | Driver | Why it matters | How we measure |
|---:|---|---|---|
| 1 | Simplicidade | Evitar complexidade de repúblicação | Código em 1 arquivo, 76 linhas |
| 2 | Preservar ordenação | Mensagem não sai do worker | Offset não é commitado até sucesso |
| 3 | Latência previsível | Delay máximo conhecido | ~1.3s para exaustão (100ms + 300ms + 900ms) |
| 4 | Context cancellation | Respeitar shutdown | `select` com `ctx.Done()` |

### 3. Options Considered

| Option | Summary | Pros | Cons | Reversibility |
|---|---|---|---|---|
| A - Retry síncrono com backoff (escolhido) | `retry.Do()` no mesmo worker | Simples, preserva contexto, sem repúblicação, fácil de testar | Bloqueia worker durante backoff, não escala com retries longos | Easy |
| B - Repúblicar no Kafka com delay | Publicar mensagem em tópico de retry com consumidor separado | Não bloqueia worker, retry assíncrono | Complexidade: novo tópico, consumidor, possibilidade de loop infinito, perda de ordenação | Hard |
| C - Redis como buffer de retry | Armazenar mensagem em Redis, worker separado processa | Desacoplado, retentativas não bloqueiam workers | Complexidade adicional, Redis como dependência extra para retry, risco de perda | Medium |
| D - Sem retry (falha → DLQ) | Mensagem vai direto para DLQ | Máxima simplicidade | Mensagens transientes seriam perdidas para DLQ desnecessariamente | Easy |

### 4. Decision

**We choose**: Option A - Retry síncrono com exponential backoff + jitter

**Why**:
- Implementação em `internal/retry/handler.go` com apenas ~76 linhas de código
- Interface `retry.Handler` com método `Do(ctx, fn)` é simples e testável
- Progressão exponencial `baseDelay * 3^(attempt-1)` — 100ms, 300ms, 900ms (configurado em `handler.go:39`)
- Jitter de ±25% via `addJitter()` em `handler.go:65-75` evita thundering herd entre workers concorrentes
- Context cancellation integrado: `select` em `handler.go:42-46` respeita cancelamento entre tentativas
- Não bloqueia offset commit: retry é parte do `processMessage` que só commita ao final
- 3 tentativas padrão (`retry_max_attempts: 3`), configurável via `config.go:64`

### 5. Architecture Impact (Implementation-Ready)

**Boundaries and contracts**
- Public APIs/contracts affected:
  - `internal/retry.Handler` interface: `Do(ctx context.Context, fn func(context.Context) error) error`
  - Config: `retry_max_attempts` (default 3), `retry_base_delay_ms` (default 100)
- Backward compatibility plan: N/A (primeira implementação)
- Schema evolution strategy: N/A

**Data and consistency**
- Source of truth: N/A (retry não armazena dados)
- Consistency model: N/A
- Migration strategy: N/A

**Failure modes and resilience**
- Known failure modes:
  - Todas as tentativas falham → erro é retornado e mensagem vai para DLQ (`handler.go:287-300`)
  - Contexto cancelado durante backoff → retorna `ctx.Err()` imediatamente (`handler.go:43`)
  - Tentativa 0 não tem delay (atemp-0ms) — otimização para sucesso na primeira
  - Delay jitter pode resultar em delay 0 se `d + jitter < 0` — safeguard em `handler.go:72-73`
- Timeouts/retries/backoff policy:
  - `baseDelay = 100ms` (configurável)
  - Progressão: 1x, 3x, 9x (para attempts 1, 2, 3)
  - Jitter: `±25%` com `rand.Float64()*2 - 1`
  - `maxAttempts = 3` (configurável)
- Idempotency strategy: Função `fn` deve ser idempotente (status Redis e history DynamoDB já são idempotentes)
- Degradation plan: Retry exaustado → DLQ. Sem retry alternativo.

**Security**
- Threat model summary: N/A
- AuthN/AuthZ model: N/A
- Secret and key management: N/A
- Audit logging requirements: Cada tentativa falha loga warning via `slog.Warn` em `handler.go:55`

**Observability**
- SLIs/SLOs: Taxa de sucesso na primeira tentativa > 99%
- Metrics/traces/logs to add:
  - `payment.consumer.retry_attempts` (counter por attempt) — `handler.go:99`, `handler.go:262`
  - Logs: `"retry attempt failed"` com attempt, max_attempts, error — `handler.go:55`
  - Logs: `"message processing failed"` após exaustão — `handler.go:177`
- Dashboards and alerts: Se retry_attempts > 10% de messages_received, investigar

**Cost and capacity**
- Expected traffic/load: Retry em < 5% das mensagens
- Cost model: Tempo extra de CPU durante backoff (delay com `time.After` não consome CPU)
- Capacity plan: Worker pool de 10 workers, cada worker pode estar em backoff por até ~1.3s

### 6. Rollout, Validation, and Rollback

**Rollout plan**
- Feature flag / staged rollout: N/A
- Data migration steps: N/A
- Runbook updates: `retry_max_attempts` e `retry_base_delay_ms` no config.yaml

**Validation plan**
- Tests to add (unit/integration/contract):
  - `internal/retry/handler_test.go` — 7 testes cobrindo:
    - `TestRetry_SuccessFirstAttempt` — sucesso na primeira
    - `TestRetry_SuccessAfterRetry` — sucesso na segunda
    - `TestRetry_Exhaustion` — exaustão após 3 tentativas
    - `TestRetry_ContextCancelled` — contexto cancelado antes de iniciar
    - `TestRetry_ContextCancelledDuringBackoff` — cancelamento durante backoff
    - `TestRetry_MaxAttemptsOne` — apenas 1 tentativa
    - `TestRetry_JitterDoesNotProduceNegative` — jitter não produz delay negativo
    - `TestRetry_NoDelayOnFirstAttempt` — primeira tentativa sem delay
- Load/perf tests: Benchmark do loop de retry

**Rollback plan**
- How to revert code: `git revert`
- How to revert data (or forward-fix): N/A
- Timebox for rollback decision: 30 min

### 7. Consequences

**Positive**
- Implementação minimalista (~76 linhas) com cobertura de testes abrangente (8 testes de mesa)
- Progressão `baseDelay * 3^(attempt-1)` é mais conservadora que binary shift (2x), dando mais tempo para recuperação
- Jitter de ±25% (`handler.go:69`) reduz sincronia entre workers sem introduzir atraso excessivo
- Retry síncrono mantém a ordenação da mensagem no worker — mesma goroutine processa todas as tentativas
- Config `retry_max_attempts=1` desliga efetivamente o retry (modo fail-fast)

**Negative / tradeoffs**
- Worker fica bloqueado durante backoff — se muitos workers estiverem em retry, throughput cai
- Delay máximo de ~1.3s para exaustão (com config padrão): 100ms + 300ms + 900ms
- Jitter usa `math/rand` sem seed explícito em `handler.go:69` — pode ser previsível
- Retry não diferencia entre falha transiente (timeout) e permanente (erro de validação) — validação falha nunca deveria chegar ao retry (é capturada antes em `handler.go:201-216`)

**Follow-ups**
- Diferenciar erros recoverable vs non-recoverable no `retry.Do` (Owner: Senior Engineer, Q3 2026)
- Substituir `math/rand` por `crypto/rand` ou seed explícito (Owner: Hardening Engineer, Q2 2026)

### 8. Links

- Código: `internal/retry/handler.go` — implementação completa (76 linhas)
- Código: `internal/consumer/handler.go` — linhas 259-283 (uso do retry no fluxo)
- Config: `retry_max_attempts: 3`, `retry_base_delay_ms: 100` (em `config.go:63-64`)
- Testes: `internal/retry/handler_test.go` (8 testes, cobertura completa)
- Relacionados: ADR-010 (DLQ), ADR-009 (worker pool), ADR-001 (Kafka)

---

## References

- ADR concept and template rationale: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
- Exponential backoff pattern: https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
