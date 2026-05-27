# Architecture Decision Record (ADR)

**ADR ID**: ADR-006  
**Title**: Redis Pub/Sub como Event Bus entre Consumer e UI  
**Status**: Accepted  
**Date**: 2026-05-25  
**Owner**: Payment Platform Team  
**Deciders**: Architect, Senior Engineer  
**Scope**: Comunicação assíncrona entre consumer (cmd/consumer) e UI (cmd/ui)

---

## Core

### 1. Context

**Problem statement**: O consumer (cmd/consumer) e a UI (cmd/ui) são processos independentes que não se comunicam diretamente. Quando o consumer processa um pagamento, a UI precisa ser notificada em tempo real para atualizar a interface. É necessário um mecanismo de comunicação assíncrona, desacoplado e de baixa latência entre os dois processos.

**Goals**:
- Notificar a UI imediatamente após o processamento de cada pagamento
- Desacoplar completamente consumer e UI (nenhum conhecimento mútuo)
- Suportar múltiplas instâncias da UI (várias abas/sessões)
- Usar Redis já disponível no stack como o meio de comunicação
- Baixa latência (< 100ms entre publish e receive)

**Non-goals**:
- Garantia de entrega de mensagens (eventos são efêmeros)
- Fila persistente (mensagens não consumidas são perdidas)
- Ordenação global de mensagens
- Roteamento complexo (apenas 1 canal)

**Constraints** (REQUIRED):
- Latency/SLO: < 100ms do publish ao subscriber
- Platform/runtime: Redis 7 Alpine (mesma instância do idempotency/status)
- Team/operational maturity: Implementação simples, padrão Pub/Sub bem conhecido

**Assumptions**:
- Redis Pub/Sub é confiável para notificações em tempo real
- Perda de eventos não é crítica (UI pode recarregar via REST)
- Canal `payment:events` não terá alta taxa de mensagens (> 100 msg/s)

### 2. Decision Drivers (What Matters Most)

| Priority | Driver | Why it matters | How we measure |
|---:|---|---|---|
| 1 | Desacoplamento | Consumer não conhece UI | Zero import entre os módulos |
| 2 | Latência | Notificação em tempo real | < 100ms publish → subscriber |
| 3 | Simplicidade | Sem brokers extras | Redis já está no stack |
| 4 | Múltiplos subscribers | Várias abas da UI | Map de subscribers com RWMutex |

### 3. Options Considered

| Option | Summary | Pros | Cons | Reversibility |
|---|---|---|---|---|
| A - Redis Pub/Sub | Canal `payment:events` | Redis já usado, simplicidade, múltiplos subscribers | Mensagens não persistem, sem garantia de entrega, subscriber lento perde msg | Easy |
| B - Kafka como event bus | Tópico separado para notificações | Garantia de entrega, persistência, replay | Complexidade (mais 1 tópico), consumer extra na UI, dependência de Kafka | Medium |
| C - HTTP callback | Consumer faz POST na UI | Simplicidade, sem dependências | Acoplamento direto (UI precisa estar no ar), latência de rede, consumer precisa saber URL da UI | Easy |
| D - gRPC stream | Stream bidirecional entre consumer e UI | Baixa latência, tipado | Complexidade, gRPC não está no stack, acoplamento | Hard |

### 4. Decision

**We choose**: Option A - Redis Pub/Sub

**Why**:
- Redis já é dependência obrigatória do sistema (idempotência + status) — zero novas dependências
- Implementação `internal/ui/events.go` com ~117 linhas, cobertura de testes completa
- `EventBus.Publish()` publica no canal Redis `payment:events` — `events.go:45-51`
- `EventBus.listenRedis()` escuta em goroutine dedicada e distribui para subscribers locais — `events.go:75-106`
- Canal Redis `payment:events` definido em `internal/ui/server.go:30` e configurável
- Múltiplos subscribers via `map[string]chan *models.PaymentEvent` com `sync.RWMutex` — `events.go:19-22`
- Buffer configurável (`ui_event_bus_buffer: 256`) evita bloqueio no publish — `events.go:36`

### 5. Architecture Impact (Implementation-Ready)

**Boundaries and contracts**
- Public APIs/contracts affected:
  - `EventBus.Publish(ctx, event)` — usado pelo handler do consumer
  - `EventBus.Subscribe()` → `(<-chan *PaymentEvent, func())` — usado pelo SSE handler
  - `EventBus.Close()` — chamado no shutdown da UI
  - Canal Redis: `payment:events` (configurável)
- Backward compatibility plan: N/A
- Schema evolution strategy: Eventos serializados como JSON, novos campos são opcionais

**Data and consistency**
- Source of truth: Eventos são efêmeros (não armazenados)
- Consistency model: At-most-once (eventos podem ser perdidos)
- Migration strategy: N/A

**Failure modes and resilience**
- Known failure modes:
  - Redis indisponível → `Publish` falha, subscriber não recebe evento (consumer continua)
  - Subscriber lento → evento descartado com log `"dropping event for slow subscriber"` — `events.go:99-100`
  - Contexto cancelado → `listenRedis` encerra goroutine — `events.go:82-83`
  - Close() → cancela contexto, fecha subscribers e channels — `events.go:109-117`
- Timeouts/retries/backoff policy: N/A (Pub/Sub não tem retry)
- Idempotency strategy: N/A
- Degradation plan: EventBus indisponível → UI não recebe notificações em tempo real, mas REST ainda funciona

**Security**
- Threat model summary: Canal Redis sem autenticação
- AuthN/AuthZ model: N/A
- Secret and key management: N/A
- Audit logging requirements: N/A

**Observability**
- SLIs/SLOs: Eventos perdidos < 1%
- Metrics/traces/logs to add:
  - Log: "dropping event for slow subscriber" — `events.go:100`
  - Log: "failed to unmarshal event from Redis" — `events.go:90`
- Dashboards and alerts: N/A

**Cost and capacity**
- Expected traffic/load: ~1-10 msg/s (um evento por pagamento)
- Cost model: CPU mínimo no Redis (~0.01% de 1 core)
- Capacity plan: Buffer de 256 por subscriber é suficiente para rajadas

### 6. Rollout, Validation, and Rollback

**Rollout plan**
- Feature flag / staged rollout: N/A
- Data migration steps: N/A
- Runbook updates: N/A

**Validation plan**
- Tests to add (unit/integration/contract):
  - `internal/ui/events_test.go`:
    - `TestEventBus_PublishSubscribe` — publish → subscribe funciona
    - `TestEventBus_MultipleSubscribers` — 2 subscribers recebem mesmo evento
    - `TestEventBus_Unsubscribe` — unsubscribe fecha canal
    - `TestEventBus_Close` — Close limpa subscribers
    - `TestEventBus_ConcurrentSubscribeUnsubscribe` — 20 subs + 10 publishes concorrentes sem panic
- Load/perf tests: N/A

**Rollback plan**
- How to revert code: `git revert`
- How to revert data (or forward-fix): N/A
- Timebox for rollback decision: 15 min

### 7. Consequences

**Positive**
- Consumer publica eventos processados via `EventBus.Publish()` em `internal/ui/events.go:45` sem saber da existência da UI
- UI escuta eventos via `Subscribe()` e distribui para SSE em `handlers.go:96`
- Subscribers lentos são descartados (`events.go:99-100`) sem bloquear outros subscribers ou o publish
- `sync.RWMutex` protege o map de subscribers para acesso concorrente seguro
- Testes de concorrência (`TestEventBus_ConcurrentSubscribeUnsubscribe`) provam ausência de race conditions

**Negative / tradeoffs**
- Mensagens podem ser perdidas se subscriber não ler rápido o suficiente (buffer de 256 lotado)
- Se Redis cair, eventos entre consumer e UI são perdidos para sempre (não há replay)
- Sem garantia de ordenação (Redis Pub/Sub entrega na ordem que chega, mas múltiplos publishers podem intercalar)
- `listenRedis` é single point of failure dentro do processo UI — se a goroutine morrer, subscribers param

**Follow-ups**
- Adicionar buffer dinâmico com backpressure (Owner: Senior Engineer, Q3 2026)
- Considerar fallback para polling REST se EventBus falhar (Owner: Senior Engineer, Q3 2026)
- Monitorar taxa de eventos dropped vs published (Owner: Hardening Engineer, Q2 2026)

### 8. Links

- Código: `internal/ui/events.go` — implementação completa do EventBus (117 linhas)
- Código: `internal/ui/server.go` — linha 30 (criação do EventBus)
- Código: `internal/ui/handlers.go` — linhas 96-97 (Subscribe no SSE handler)
- Config: `ui_event_bus_buffer: 256` (em `config.go:36`)
- Testes: `internal/ui/events_test.go` (5 testes, cobertura de concorrência e cenários básicos)
- Relacionados: ADR-005 (SSE), ADR-002 (Redis)

---

## References

- ADR concept and template rationale: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
- Redis Pub/Sub: https://redis.io/docs/latest/develop/interact/pubsub/
