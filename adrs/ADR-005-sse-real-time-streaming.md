# Architecture Decision Record (ADR)

**ADR ID**: ADR-005  
**Title**: Server-Sent Events (SSE) para Streaming em Tempo Real na UI  
**Status**: Accepted  
**Date**: 2026-05-25  
**Owner**: Payment Platform Team  
**Deciders**: Architect, Senior Engineer  
**Scope**: Interface web (cmd/ui) — comunicação em tempo real com o browser

---

## Core

### 1. Context

**Problem statement**: A interface web precisa exibir eventos de pagamento em tempo real conforme são processados pelo consumer. A comunicação é unidirecional (servidor → browser) e exige reconexão automática em caso de falha de rede. A solução deve ser simples, sem dependências de frameworks JavaScript.

**Goals**:
- Entregar eventos de pagamento no browser em < 1s após processamento pelo consumer
- Reconexão automática em caso de queda de conexão
- Heartbeat a cada 30s para manter conexão ativa
- Suportar múltiplas abas/sessões simultâneas (máx 100)
- Implementação leve, sem frameworks JS

**Non-goals**:
- Comunicação bidirecional (browser → servidor) — usamos REST para isso
- Suporte a WebSocket (complexidade não necessária)
- Streaming binário ou de arquivos
- Garantia de entrega de eventos perdidos (eventos são efêmeros)

**Constraints** (REQUIRED):
- Latency/SLO: Evento visível no browser em < 1s (objetivo) e < 3s (limite superior)
- Platform/runtime: HTTP/1.1, vanilla JavaScript no frontend
- Browser compatibility: SSE é suportado em todos os browsers modernos (IE除外)

**Assumptions**:
- Conexão SSE é aceita por proxies reversos (nginx, Cloudflare) com configuração adequada
- Navegadores modernos suportam SSE nativamente via `EventSource`
- Heartbeat de 30s é suficiente para manter conexão viva (proxies costumam ter timeout de 60-120s)

### 2. Decision Drivers (What Matters Most)

| Priority | Driver | Why it matters | How we measure |
|---:|---|---|---|
| 1 | Simplicidade | Sem dependências JS, sem build step | código vanilla, 0 dependências npm |
| 2 | Reconexão automática | SSE nativa reconecta em 3s | Nativo do EventSource |
| 3 | Unidirecional | Apenas servidor → cliente | Suficiente para o caso de uso |
| 4 | Baixa latência | Eventos em tempo real | P99 < 1s do processamento ao navegador |

### 3. Options Considered

| Option | Summary | Pros | Cons | Reversibility |
|---|---|---|---|---|
| A - Server-Sent Events (SSE) | `text/event-stream`, `EventSource` API | Reconexão automática nativa, simples, sem libs, heartbeat fácil | Unidirecional, limite de 6 conexões por domínio (HTTP/1.1) | Easy |
| B - WebSocket | Comunicação bidirecional via `ws://` | Bidirecional, baixa latência, sem limite de conexões | Complexidade maior, reconexão manual, depende de lib JS | Medium |
| C - Long Polling | Polling HTTP com `setInterval` | Compatibilidade total, sem SSE no servidor | Latência maior (polling interval), overhead de requests, sem evento real | Easy |
| D - Server-Sent Events com polyfill | SSE + fallback para polling | Fallback para browsers antigos | Overhead de implementação, browsers antigos não são alvo | Medium |

### 4. Decision

**We choose**: Option A - Server-Sent Events (SSE)

**Why**:
- `EventSource` nativo do browser reconecta automaticamente em ~3 segundos após queda — sem código adicional
- Implementação no servidor é trivial: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive` em `internal/ui/handlers.go:92-94`
- Heartbeat a cada 30s (`handlers.go:102`, `handlers.go:120-122`) mantém proxy/load balancer ativo
- SSE é estritamente unidirecional (servidor → browser), que é exatamente o que precisamos
- Sem dependências JavaScript — `app.js` usa `new EventSource('/api/events')` sem polyfills
- Limite de 100 conexões simultâneas via semáforo (`var sseSemaphore = make(chan struct{}, 100)` em `handlers.go:24`)

### 5. Architecture Impact (Implementation-Ready)

**Boundaries and contracts**
- Public APIs/contracts affected:
  - `GET /api/events` — SSE endpoint em `internal/ui/handlers.go:66-125`
  - Eventos: `event: payment` com payload JSON, `event: heartbeat` com `data: {}`
  - Métrica: SSE connections counters (`sseConnections`, `sseTotalConnections`) — `handlers.go:28-29`
- Backward compatibility plan: N/A (primeira implementação)
- Schema evolution strategy: Eventos adicionais adicionam novos `event:` types mantendo compatibilidade

**Data and consistency**
- Source of truth: Eventos são efêmeros, originados do Redis Pub/Sub (ADR-006)
- Consistency model: Eventual — eventos podem ser perdidos se subscriber estiver lento
- Migration strategy: N/A

**Failure modes and resilience**
- Known failure modes:
  - Conexão perdida → browser reconecta automaticamente em ~3s (nativo EventSource)
  - Subscriber lento → evento é descartado (`internal/ui/events.go:99-100`)
  - Semáforo cheio (100 conexões) → `http.Error(w, "too many connections", 503)` — `handlers.go:78`
  - EventBus nil → `http.Error(w, "event bus not available", 500)` — `handlers.go:68`
- Timeouts/retries/backoff policy:
  - `UIWriteTimeout: 30s` em `config.go:73` — SSE é exceção, mas Go http.Server write timeout não afeta SSE (streaming)
  - Reconexão gerenciada pelo browser
- Idempotency strategy: N/A
- Degradation plan: SSE indisponível → UI continua funcional (REST ainda funciona, apenas sem atualização em tempo real)

**Security**
- Threat model summary: SSE não tem autenticação (apenas conexão anônima)
- AuthN/AuthZ model: N/A (ambiente controlado)
- Secret and key management: N/A
- Audit logging requirements: N/A

**Observability**
- SLIs/SLOs: SSE connections ativas < 100, eventos entregues > 99%
- Metrics/traces/logs to add:
  - Logs: `"SSE client connected"` / `"SSE client disconnected"` (`handlers.go:99-100`)
  - Logs: `"SSE semaphore full"` quando limite é atingido (`handlers.go:77`)
- Dashboards and alerts: Se sseSemaphore rejeitar conexões, alertar

**Cost and capacity**
- Expected traffic/load: 1-10 conexões SSE simultâneas
- Cost model: Inexistente (1 goroutine por conexão, ~5KB RAM cada)
- Capacity plan: 100 conexões máx (configurável no semáforo)

### 6. Rollout, Validation, and Rollback

**Rollout plan**
- Feature flag / staged rollout: N/A
- Data migration steps: N/A
- Runbook updates: N/A

**Validation plan**
- Tests to add (unit/integration/contract):
  - `internal/ui/handlers_test.go` — SSE handler com ResponseRecorder
  - `internal/ui/events_test.go` — EventBus publish/subscribe
  - `cmd/ui/main_test.go` — teste de integração
- Load/perf tests: Testar 100 conexões SSE simultâneas (limite do semáforo)

**Rollback plan**
- How to revert code: `git revert`
- How to revert data (or forward-fix): N/A
- Timebox for rollback decision: 15 min

### 7. Consequences

**Positive**
- Reconexão automática sem código JavaScript — `EventSource` nativo dos browsers
- Heartbeat de 30s mantém conexão ativa através de proxies e load balancers
- Semáforo de 100 conexões (`handlers.go:24`) protege o servidor contra excesso de conexões SSE
- Frontend vanilla JS recebe eventos como: `eventSource.addEventListener('payment', handler)` em `app.js`
- Métricas de conexões SSE (`sseConnections`) permitem monitoramento

**Negative / tradeoffs**
- Limite de 6 conexões SSE por domínio em HTTP/1.1 (navegador limita) — resolvido com HTTP/2 em produção
- Subscriber lento perde eventos (drop no `events.go:100`) — sem fila de replay
- Sem autenticação nas conexões SSE — qualquer um pode conectar
- WriteTimeout de 30s (`config.go:73`) pode conflitar com SSE de longa duração — resolvido porque SSE não usa Write no Go (apenas Flush)

**Follow-ups**
- Considerar migração para HTTP/2 em produção para aumentar limite de conexões SSE (Owner: DevOps, Q3 2026)
- Implementar fila de replay para subscribers lentos (Owner: Senior Engineer, Q4 2026)
- Avaliar autenticação via token JWT nas conexões SSE (Owner: Security Team, Q3 2026)

### 8. Links

- Código: `internal/ui/handlers.go` — linhas 66-125 (HandleSSE)
- Código: `internal/ui/events.go` — EventBus (subscriber distribuído via Redis Pub/Sub)
- Código: `internal/ui/server.go` — linhas 17-18 (embed static), 53-58 (HTTP server config)
- Frontend: `internal/ui/static/app.js` — EventSource listener
- Testes: `internal/ui/handlers_test.go`, `internal/ui/events_test.go`
- Relacionados: ADR-006 (Redis Pub/Sub), ADR-012 (graceful shutdown)

---

## References

- ADR concept and template rationale: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
- MDN Server-Sent Events: https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events
