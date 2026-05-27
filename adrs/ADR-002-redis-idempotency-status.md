# Architecture Decision Record (ADR)

**ADR ID**: ADR-002  
**Title**: Redis para Idempotência e Status de Pagamentos  
**Status**: Accepted  
**Date**: 2026-05-25  
**Owner**: Payment Platform Team  
**Deciders**: Architect, Senior Engineer  
**Scope**: Armazenamento de estado efêmero (idempotência e status)

---

## Core

### 1. Context

**Problem statement**: O sistema precisa de dois armazenamentos de baixa latência: (1) verificação atômica de idempotência para evitar processamento duplicado de mensagens Kafka, e (2) status atual dos pagamentos para consulta em tempo real pela UI. Ambos exigem expiração automática (TTL) e operações atômicas.

**Goals**:
- Verificar se uma mensagem já foi processada em < 10ms (P99)
- Marcar processamento com operação atômica (SET se não existe)
- Armazenar status do pagamento com expiração automática (7 dias)
- Suportar consulta de todos os pagamentos via SCAN + HGETALL
- Pipeline de operações para minimizar round-trips

**Non-goals**:
- Armazenamento permanente de histórico (coberto no DynamoDB, ADR-003)
- Consultas complexas (joins, agregações)
- Transações distribuídas (consistência eventual entre Redis e DynamoDB)
- Cache distribuído para outros serviços

**Constraints** (REQUIRED):
- Latency/SLO: idempotência < 10ms, status update < 20ms
- Platform/runtime: Redis 7 Alpine, instância única via Docker
- Durabilidade: Idempotência pode ser perdida (TTL 24h) sem prejuízo; status pode ser reconstruído do DynamoDB

**Assumptions**:
- Redis roda em single-instance (sem cluster ou sentinel) em desenvolvimento
- Perda de dados Redis é tolerável (status pode ser reconstruído)
- Payload de idempotência é mínimo (chave + flag "1")

### 2. Decision Drivers (What Matters Most)

| Priority | Driver | Why it matters | How we measure |
|---:|---|---|---|
| 1 | Latência | Idempotência e status são caminho crítico do consumer | P99 < 10ms para SETNX, < 5ms para HGETALL |
| 2 | Atomicidade | SETNX garante operação atômica sem race conditions | Testes de concorrência |
| 3 | Expiração automática | TTL evita acúmulo de dados obsoletos | Monitoramento de memória Redis |
| 4 | Simplicidade | Redis já disponível no stack | Zero novas dependências |

### 3. Options Considered

| Option | Summary | Pros | Cons | Reversibility |
|---|---|---|---|---|
| A - Redis | SETNX, HSET, Pipeline, TTL nativo | Baixíssima latência, atômico, TTL automático, pipeline | Sem persistência forte, sem consultas complexas | Medium |
| B - DynamoDB | Tabela separada para idempotência + status | Mesmo banco do histórico, consistência forte | Latência maior (10-50ms), sem TTL automático nativo para status | Medium |
| C - PostgreSQL | Tabela relacional com índice único | Garantia transacional, familiaridade | Latência maior, requer gerenciamento de schema, overhead operacional | Hard |
| D - Memória local (Go map) | sync.Map no consumer | Zero latência de rede, sem dependência | Perde tudo em restart, não compartilhado entre instâncias | Easy |

### 4. Decision

**We choose**: Option A - Redis

**Why**:
- `SETNX` (comando `SetNX` em `internal/idempotency/redis.go:41`) oferece atomicidade perfeita para idempotência — se a chave não existe, cria com TTL; se existe, retorna false
- `HSET` em pipeline com `Expire` em `internal/status/redis.go:31-37` permite atualizar status + renovar TTL em um único round-trip
- TTL nativo: 24h para idempotência (`idempotency_ttl_hours: 24`), 168h (7 dias) para status (`status_ttl_hours: 168`)
- Redis Pub/Sub no mesmo client possibilita o Event Bus (ADR-006)
- Latência sub-milissegundo para operações locais

### 5. Architecture Impact (Implementation-Ready)

**Boundaries and contracts**
- Public APIs/contracts affected:
  - `internal/idempotency.Checker` interface: `IsProcessed(ctx, paymentID)` e `MarkProcessed(ctx, paymentID)`
  - `internal/status.Updater` interface: `UpdateStatus(ctx, paymentID, status)`
- Backward compatibility plan: N/A
- Schema evolution strategy: Chaves com prefixo `idempotency:` e `payment:` separam namespaces

**Data and consistency**
- Source of truth: Redis (para status corrente), DynamoDB (para histórico permanente)
- Consistency model: Eventual — status no Redis pode divergir do DynamoDB por alguns segundos
- Migration strategy: N/A (primeira implementação)

**Failure modes and resilience**
- Known failure modes:
  - Redis indisponível → `Ping` em `cmd/consumer/main.go:86` loga warning, consumer continua (opera com degradação)
  - Falha no `MarkProcessed` → consumer loga warning mas continua (possível reprocessamento)
  - Falha no `IsProcessed` → consumer assume não processado (procede otimisticamente, `handler.go:234`)
- Timeouts/retries/backoff policy: Timeout de 5s para operações de idempotência e status (`handler.go:229`, `handler.go:248`, `handler.go:267`)
- Idempotency strategy: Chave `idempotency:<payment_id>` com `SetNX` e TTL 24h
- Degradation plan: Se Redis falha, consumer opera sem idempotência (risco de duplicação) e sem status atualizado (UI fica desatualizada)

**Security**
- Threat model summary: Redis sem autenticação em dev (config `redis_password: ""`)
- AuthN/AuthZ model: Senha opcional via `redis_password`
- Secret and key management: `redis_password` via variável de ambiente
- Audit logging requirements: N/A

**Observability**
- SLIs/SLOs: Operações de idempotência < 10ms, status update < 20ms
- Metrics/traces/logs to add:
  - `payment.consumer.idempotency_hits` (counter) — `handler.go:112`
  - Logs estruturados em `handler.go:241` para idempotency hit
- Dashboards and alerts: N/A

**Cost and capacity**
- Expected traffic/load: ~1-10k chaves simultâneas (24h TTL para idempotência, 7d TTL para status)
- Cost model: Redis 7 Alpine em Docker (~150MB RAM para 10k chaves)
- Capacity plan: Monitorar `used_memory` no Redis; escalar para Redis Cluster se > 1M chaves

### 6. Rollout, Validation, and Rollback

**Rollout plan**
- Feature flag / staged rollout: N/A
- Data migration steps: N/A
- Runbook updates: Verificar `redis_addr` no config.yaml

**Validation plan**
- Tests to add (unit/integration/contract):
  - `internal/idempotency/redis_test.go` — testes com miniredis
  - `internal/status/redis_test.go` — testes com miniredis
- Load/perf tests: Benchmark de SETNX vs HGETALL

**Rollback plan**
- How to revert code: `git revert`
- How to revert data (or forward-fix): FLUSHALL (apenas dev)
- Timebox for rollback decision: 30 min

### 7. Consequences

**Positive**
- `IsProcessed` + `MarkProcessed` atômicos eliminam race conditions mesmo com múltiplos workers
- Pipeline de `HSET` + `Expire` em `internal/status/redis.go:31-37` reduz latência para 1 round-trip
- TTL separado para idempotência (24h) e status (7d) equilibra memória vs retenção
- UI consulta status via `SCAN payment:*` → `HGETALL` em `internal/ui/handlers.go:148-188`

**Negative / tradeoffs**
- Sem Redis, idempotência é perdida — risco de duplicação em falha parcial
- `SCAN` não é determinístico em performance para milhões de chaves
- TTL de 24h para idempotência significa que mensagens com mais de 24h podem ser reprocessadas
- `MarkProcessed` em `handler.go:251` é best-effort — se falha, não bloqueia o fluxo

**Follow-ups**
- Implementar fallback para idempotência em DynamoDB se Redis falhar (Owner: Senior Engineer, Q3 2026)
- Avaliar Redis Cluster para escala horizontal (Owner: DevOps, Q4 2026)

### 8. Links

- Código: `internal/idempotency/redis.go` — interface `Checker`, implementação `redisChecker`
- Código: `internal/status/redis.go` — interface `Updater`, implementação `redisUpdater`
- Código: `internal/consumer/handler.go` — linhas 228-256 (uso no fluxo de processamento)
- Código: `internal/ui/handlers.go` — linhas 148-188, 244-278 (consulta de status e métricas)
- Config: `idempotency_ttl_hours: 24`, `status_ttl_hours: 168` (em `config.go`)
- Testes: `internal/idempotency/redis_test.go`, `internal/status/redis_test.go`
- Relacionados: ADR-008 (consistência eventual), ADR-006 (Redis Pub/Sub)

---

## References

- ADR concept and template rationale: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
- go-redis: https://github.com/redis/go-redis
