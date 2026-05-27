# Architecture Decision Record (ADR)

**ADR ID**: ADR-008  
**Title**: Consistência Eventual entre Redis (Status) e DynamoDB (Histórico)  
**Status**: Accepted  
**Date**: 2026-05-25  
**Owner**: Payment Platform Team  
**Deciders**: Architect, Senior Engineer  
**Scope**: Modelo de consistência entre armazenamentos de pagamento

---

## Core

### 1. Context

**Problem statement**: O sistema armazena dados de pagamento em dois lugares: Redis para status corrente (rápido, TTL 7 dias) e DynamoDB para histórico permanente. Esses dois armazenamentos são atualizados na mesma operação (`retry.Do` em `handler.go:259-283`), mas não em uma transação distribuída. Um pode falhar enquanto o outro não, criando uma janela de inconsistência. É necessário definir e documentar o modelo de consistência entre eles.

**Goals**:
- Definir modelo de consistência entre Redis (status) e DynamoDB (histórico)
- Garantir que a UI possa consultar ambos os armazenamentos
- Minimizar latência nas operações de escrita (sem transações distribuídas)
- Fornecer mecanismo de reconciliação para casos de inconsistência

**Non-goals**:
- Transações distribuídas entre Redis e DynamoDB
- Consistência forte entre os dois armazenamentos
- Sincronismo de dados entre Redis e DynamoDB via jobs background
- Garantia de leitura após escrita imediata

**Constraints** (REQUIRED):
- Latency/SLO: Escrita < 200ms combinado (Redis + DynamoDB)
- Durability: DynamoDB é a fonte da verdade permanente; Redis é efêmero
- Platform/runtime: Redis e DynamoDB são sistemas independentes

**Assumptions**:
- Status no Redis pode divergir do histórico no DynamoDB por alguns segundos
- UI lida com essa divergência (status é em tempo real, histórico é fonte da verdade)
- Reconciliação pode ser feita externamente se necessário

### 2. Decision Drivers (What Matters Most)

| Priority | Driver | Why it matters | How we measure |
|---:|---|---|---|
| 1 | Latência de escrita | Atualizar status + histórico em < 200ms | P99 < 200ms combinado |
| 2 | Disponibilidade | Ambos sistemas podem falhar independentemente | Uptime separado de cada dependência |
| 3 | Simplicidade | Sem saga, sem 2PC, sem transações | Código linear em `handler.go:259-283` |
| 4 | Rastreabilidade | Saber quando e onde houve divergência | Logs, trace_id, métricas |

### 3. Options Considered

| Option | Summary | Pros | Cons | Reversibility |
|---|---|---|---|---|
| A - Consistência eventual (escolhido) | Redis e DynamoDB atualizados no mesmo retry, sem transação | Simples, baixa latência, failover parcial | Janela de inconsistência, divergência possível | Medium |
| B - Escrever apenas no DynamoDB | Redis deriva do DynamoDB (cache-aside) | Fonte da verdade única, sem divergência | Latência maior nas consultas (DynamoDB toda vez), Redis subutilizado | Medium |
| C - Escrita dupla com transação | Redis + DynamoDB em saga/2PC | Consistência forte | Complexidade altíssima, latência maior, Redis não suporta 2PC | Hard |
| D - DynamoDB Streams → Redis | DynamoDB streams atualizam Redis via trigger | Fonte da verdade única, Redis sempre consistente | Complexidade (Lambda/worker), latência adicional do stream, dependência de DynamoDB Streams | Hard |

### 4. Decision

**We choose**: Option A - Consistência eventual

**Why**:
- Escrita em Redis (`status.UpdateStatus`) e DynamoDB (`history.RecordHistory`) ocorre dentro do mesmo `retry.Do` em `handler.go:259-283` — falha em um cancela a transação e retenta ambos
- `ConditionExpression` no DynamoDB (`dynamodb.go:53`) garante idempotência na fonte da verdade
- Redis tem TTL de 7 dias (`status_ttl_hours=168`) — após expirar, status pode ser reconstruído do DynamoDB
- UI consulta ambos: status via Redis (`handlers.go:128-191`) e histórico via DynamoDB (`handlers.go:194-241`)
- Modelo de falha: se Redis falha após DynamoDB escrever, status fica desatualizado até retry ou expiração
- Sem dependência de transações distribuídas

### 5. Architecture Impact (Implementation-Ready)

**Boundaries and contracts**
- Public APIs/contracts affected:
  - `GET /api/payments` → retorna status do Redis (pode estar desatualizado)
  - `GET /api/payments/{id}/history` → retorna histórico do DynamoDB (fonte da verdade)
  - `GET /api/metrics` → métricas agregadas do Redis (pode divergir)
- Backward compatibility plan: N/A
- Schema evolution strategy: N/A

**Data and consistency**
- Source of truth: DynamoDB é a fonte da verdade permanente
- Consistency model: Eventual — status Redis e histórico DynamoDB podem divergir por até ~1.3s (tempo de retry)
- Migration strategy: N/A

**Failure modes and resilience**
- Known failure modes:
  - Redis escreve OK, DynamoDB falha → retry tenta novamente ambas operações; se exaustão → DLQ
  - DynamoDB escreve OK, Redis falha → status não atualizado (UI mostra status antigo); próximo retry tenta Redis novamente
  - Ambos falham → retry com backoff exponencial
  - Redis OK, mas TTL expira → status desaparece; UI não lista pagamento antigo (mas histórico ainda existe no DynamoDB)
- Timeouts/retries/backoff policy:
  - Redis: timeout de 5s por operação (`handler.go:267`)
  - DynamoDB: timeout de 10s por operação (`handler.go:275`)
  - Retry: 3 tentativas, backoff 100ms, 300ms, 900ms
- Idempotency strategy:
  - Redis: `HSET` é idempotente (mesmo valor)
  - DynamoDB: `ConditionExpression` previne duplicatas
- Degradation plan:
  - Redis down → status indisponível na UI, mas histórico ainda consultável
  - DynamoDB down → histórico indisponível, status ainda visível na UI

**Security**
- Threat model summary: N/A
- AuthN/AuthZ model: N/A
- Secret and key management: N/A
- Audit logging requirements: `trace_id` no DynamoDB permite rastrear operação

**Observability**
- SLIs/SLOs: Divergência Redis/DynamoDB resolvida em < 30s (próximo retry ou reprocessamento)
- Metrics/traces/logs to add:
  - Métricas de sucesso/falha por operação (Redis status, DynamoDB history)
  - Log de warning quando `status.UpdateStatus` falha (`handler.go:270`)
  - Log de warning quando `history.RecordHistory` falha (`handler.go:278`)
- Dashboards and alerts: Se divergência > 1% das mensagens, investigar

**Cost and capacity**
- Expected traffic/load: 100% das mensagens geram 2 escritas (1 Redis + 1 DynamoDB)
- Cost model: Escrever em ambos duplica IOPS
- Capacity plan: Ajustar DynamoDB WCUs se divergência causar retries extras

### 6. Rollout, Validation, and Rollback

**Rollout plan**
- Feature flag / staged rollout: N/A
- Data migration steps: N/A
- Runbook updates: Documentar que Redis e DynamoDB podem divergir

**Validation plan**
- Tests to add (unit/integration/contract):
  - Testes de integração: Redis OK + DynamoDB falha → sistema retenta
  - Testes de integração: DynamoDB OK + Redis falha → sistema retenta
  - Testes de integração: Ambos OK → consistência eventual resolvida
- Load/perf tests: Medir latência combinada das duas escritas

**Rollback plan**
- How to revert code: `git revert`
- How to revert data (or forward-fix): Reconstruir Redis do DynamoDB via script
- Timebox for rollback decision: 1 hora

### 7. Consequences

**Positive**
- Latência baixa: Redis `HSET` (~1-5ms) + DynamoDB `PutItem` (~10-50ms) em série = < 100ms típico
- Falha parcial não corrompe dados: DynamoDB (fonte da verdade) sempre tem o registro completo
- UI pode mostrar "status em tempo real" via Redis + "histórico completo" via DynamoDB na mesma tela
- TTL Redis de 7 dias significa que apenas pagamentos recentes têm divergência potencial
- Métricas agregadas (`/api/metrics`) usam Redis para performance, mas podem divergir levemente

**Negative / tradeoffs**
- Janela de inconsistência: entre a escrita no Redis e no DynamoDB, pode haver divergência
- Sem atomicidade: Redis OK + DynamoDB falha → status mostra processado mas histórico não existe (resolvido no retry)
- Métricas `by_status` no `/api/metrics` podem divergir do estado real no DynamoDB
- Se Redis perder dados (restart sem persistência), status corrente é perdido — UI fica sem dados até novos eventos

**Follow-ups**
- Implementar script de reconciliação que varre DynamoDB e atualiza Redis (Owner: Senior Engineer, Q3 2026)
- Adicionar endpoint `GET /api/payments/sync` que consulta DynamoDB e atualiza Redis (Owner: Senior Engineer, Q3 2026)
- Considerar Redis com AOF persistente para redução de perda de dados (Owner: DevOps, Q3 2026)

### 8. Links

- Código: `internal/consumer/handler.go` — linhas 259-283 (escrita dupla no retry)
- Código: `internal/status/redis.go` — `UpdateStatus` (escrita Redis)
- Código: `internal/history/dynamodb.go` — `RecordHistory` (escrita DynamoDB)
- Código: `internal/ui/handlers.go` — linhas 128-191 (leitura status Redis), 194-241 (leitura histórico DynamoDB)
- Config: `status_ttl_hours: 168`, `dynamodb_table: payment_history`
- Relacionados: ADR-002 (Redis), ADR-003 (DynamoDB), ADR-004 (retry)

---

## References

- ADR concept and template rationale: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
- CAP Theorem: https://en.wikipedia.org/wiki/CAP_theorem
- DynamoDB Condition Expressions: https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/Expressions.ConditionExpressions.html
