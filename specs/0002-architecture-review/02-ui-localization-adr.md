# Architecture Decision Record (ADR)

**ADR ID**: ADR-017 (localization), ADR-018 (connection reliability)  
**Title**: Portuguese-Brazil Localization and SSE Connection Reliability  
**Status**: Accepted  
**Date**: 2026-05-25  
**Owner**: Architect  
**Deciders**: Architect, Hardening Engineer  
**Scope**: UI frontend (internal/ui/static/) — localization and SSE connection indicator fix

---

## Core

### 1. Context

**Problem statement**: The UI frontend was entirely in English, but the development team operates in Portuguese (Brazil). Additionally, the SSE connection status indicator (`#connection-status`) was unreliable — it never changed from "Disconnected" because the frontend relied solely on `EventSource.onopen`/`onerror` events, which are not consistently fired across browsers and network conditions.

**Goals**:
- Localize all static UI text to pt-BR for the development team
- Fix the connection status indicator so it accurately reflects SSE connection state
- Keep the implementation simple — no i18n framework, no build step
- Use emoji characters directly instead of Unicode escape sequences for readability

**Non-goals**:
- Multi-language internationalization (single-language team)
- SSR (server-side rendering) for localized content
- i18n framework integration
- Connection status indicator redesign (only fix the broken detection)

**Constraints** (REQUIRED):
- Platform/runtime: All strings hardcoded in static files; no backend changes for localization
- Team/operational maturity: Single Portuguese-speaking team
- Browser compatibility: SSE EventSource API must work correctly with the heartbeat timeout pattern

**Assumptions**:
- No future requirement for additional languages (if needed, extraction to i18n system is a separate effort)
- 45-second heartbeat timeout is sufficient for all realistic network conditions
- Browser `setTimeout` accuracy (even in background tabs) is sufficient for the 45s window

### 2. Decision Drivers (What Matters Most)

| Priority | Driver | Why it matters | How we measure |
|---:|---|---|---|
| 1 | Correctness | Connection status must reflect actual connection state | Manual testing: disconnect/reconnect scenarios |
| 2 | Simplicity | No i18n framework, no build step, no backend changes | Zero new dependencies |
| 3 | Localization | Team reads pt-BR natively | All visible strings in pt-BR |
| 4 | Maintainability | Source code must be readable | Literal emoji over Unicode escapes |

### 3. Options Considered

#### Localization

| Option | Summary | Pros | Cons | Reversibility |
|---|---|---|---|---|
| A - Hardcoded pt-BR | Translate all strings directly in index.html and app.js | Zero dependencies, no build step, simple | No multi-language support | Easy |
| B - Go i18n middleware | Server-side localization via go-i18n | Proper i18n, language negotiation | Backend changes, complexity | Medium |
| C - JavaScript i18n lib | Client-side i18n (i18next, etc.) | Proper i18n, extensible | npm dependency, build step, overkill | Medium |
| D - CSS-only flag | Keep English but add pt-BR as CSS content | No JS changes | Hacks, not maintainable | Easy |

#### Connection Status Fix

| Option | Summary | Pros | Cons | Reversibility |
|---|---|---|---|---|
| A - Heartbeat timeout (45s) | Client-side timer reset on any SSE data | Reliable, simple, no backend changes | 45s delay before detecting disconnection | Easy |
| B - WebSocket + ping/pong | Replace SSE with WebSocket | Bidirectional, native ping/pong | Complex, more code, requires backend changes | Medium |
| C - Fetch-based health poll | Periodic GET /healthz | Independent of SSE | Extra network requests, doesn't test SSE path | Easy |
| D - EventSource `onerror` only | Keep existing code (no fix) | No changes | Broken — never changes from Disconnected | N/A |

### 4. Decision

**We choose**: Option A for both localization (hardcoded pt-BR) and connection fix (heartbeat timeout 45s).

**Why localization (A)**:
- The team is single-language (pt-BR). Adding an i18n framework would be over-engineering for a development tool
- All strings are static HTML text and JavaScript string literals — no dynamic content needs localization
- Backend API responses remain in English (status values `pending`, `confirmed`, etc.) — only frontend display labels are localized
- Literal emoji characters (`🟢`) are used instead of Unicode escapes (`\u{1F7E2}`) for source readability

**Why connection fix (A)**:
- The heartbeat timeout pattern is the simplest reliable solution — no backend changes, no new dependencies
- Server already sends heartbeats every 30s (`handlers.go:102,120-122`) — the client just needs to track them
- 45s timeout (1.5× the heartbeat interval) balances detection speed with tolerance for jitter
- The fix works across all browsers because it only uses standard `setTimeout`/`clearTimeout` and `EventSource` events
- Status remains correct during initial connection, drop, reconnect, and server restart scenarios

### 5. Architecture Impact

**Boundaries and contracts**
- Public APIs/contracts affected: **None** — the API endpoints remain unchanged
- Backward compatibility plan: Fully backward compatible — no API changes
- Schema evolution strategy: N/A

**Data and consistency**
- Source of truth: Localized text is in the static source files; connection status is derived from heartbeat timeout
- Consistency model: N/A (UI-only changes)
- Migration strategy: N/A

**Failure modes and resilience**
- Known failure modes:
  - Browser throttles `setTimeout` in background tabs → timeout may fire later than 45s (acceptable)
  - Server stops sending heartbeats → status correctly shows "Desconectado" after 45s
  - Clock skew between server and client → not relevant (timeout is monotonic, not absolute)
- Timeouts/retries/backoff policy:
  - 45s heartbeat timeout; no retry (browser EventSource auto-reconnects)
- Idempotency strategy: N/A
- Degradation plan: If heartbeat timeout fires incorrectly, status briefly shows "Desconectado" until next event arrives

**Security**
- Threat model summary: N/A (UI-only, no new endpoints)
- AuthN/AuthZ model: N/A
- Secret and key management: N/A
- Audit logging requirements: N/A

**Observability**
- SLIs/SLOs: Connection status accuracy > 99% (no false positives/negatives)
- Metrics/traces/logs to add: `console.error` for SSE parse errors remains (`app.js:46`)
- Dashboards and alerts: N/A

**Cost and capacity**
- Expected traffic/load: N/A (no new network requests)
- Cost model: N/A (client-side only)
- Capacity plan: N/A

### 6. Rollout, Validation, and Rollback

**Rollout plan**
- Feature flag / staged rollout: N/A (UI files are deployed as embedded assets)
- Data migration steps: N/A
- Runbook updates: N/A

**Validation plan**
- Tests to add (unit/integration/contract):
  - Manual validation: verify all UI strings are in pt-BR
  - Manual validation: disconnect network → status shows "Desconectado" within 60s → reconnect → status shows "Conectado"
  - Manual validation: verify status badges show pt-BR labels (Pendente, Confirmado, Falhou, Reembolsado)

**Rollback plan**
- How to revert code: `git revert` the commit containing the pt-BR translations and heartbeat fix
- How to revert data (or forward-fix): Revert to English version (previous commit)
- Timebox for rollback decision: 15 min

### 7. Consequences

**Positive**
- UI is now fully in pt-BR — all labels, headings, placeholders, and status messages
- Connection status indicator reliably shows "Conectado" or "Desconectado" based on actual data flow
- 45s timeout provides timely detection of connection issues without false positives
- No new dependencies, no backend changes, no build step
- Literal emoji characters in source code are readable and maintainable
- Status value mapping centralizes display logic

**Negative / tradeoffs**
- Adding a second language in the future requires extracting all strings from HTML/JS into an i18n system
- 45s delay between actual disconnection and UI feedback — acceptable for a development tool
- `setTimeout` in background tabs may be throttled by browsers (Chrome ≥1s, Firefox ≥1s) — still < 5% of 45s window
- Backend API responses remain in English (status values are programmatic, not user-facing)

**Follow-ups**
- Consider adding a manual "Reconnect" button if users need to force reconnection (Owner: Frontend Engineer, Q3 2026)
- Monitor connection status accuracy in production with different browsers (Owner: Hardening Engineer, Q3 2026)

### 8. Links

- Frontend: `internal/ui/static/index.html` — pt-BR localized HTML
- Frontend: `internal/ui/static/app.js` — heartbeat timeout pattern, pt-BR status mapping
- Backend: `internal/ui/handlers.go` — SSE heartbeat every 30s (unchanged)
- ADR-005: Server-Sent Events (SSE) — original SSE design
- Spec: `specs/0003-ui-localization/` — full spec for UI localization work

---

## References

- ADR concept and template rationale: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
- MDN EventSource: https://developer.mozilla.org/en-US/docs/Web/API/EventSource
- MDN setTimeout throttling: https://developer.mozilla.org/en-US/docs/Web/API/Window/setTimeout#throttling
