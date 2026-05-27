# 03 — Design

## 1. Arquitetura da Solução

### 1.1 Visão Geral

A solução é inteiramente **client-side**. Nenhuma alteração no backend Go. As modificações são confinadas a:

```
internal/ui/static/
├── index.html    ← Localização pt-BR de todo o HTML
├── app.js        ← Localização pt-BR + heartbeat timeout pattern + status mapping
└── style.css     ← Sem alterações (classes CSS permanecem as mesmas)
```

### 1.2 Diagrama de Fluxo — Heartbeat Timeout

```
Navegador                          Servidor Go (:8081)
   │                                    │
   │  GET /api/events (SSE)             │
   │───────────────────────────────────▶│
   │                                    │
   │  event: open                       │
   │◀───────────────────────────────────│
   │  onOpen()                          │
   │    → status = "🟢 Conectado"       │
   │    → scheduleTimeout(45s)          │
   │                                    │
   │  event: heartbeat (a cada 30s)     │
   │◀───────────────────────────────────│
   │  onHeartbeat()                     │
   │    → status = "🟢 Conectado"       │
   │    → scheduleTimeout(45s)          │
   │                                    │
   │  ... 45s sem dados ...             │
   │  [TIMEOUT DISPARADO]               │
   │    → status = "🔴 Desconectado"    │
   │                                    │
   │  EventSource auto-reconnect (~3s)  │
   │───────────────────────────────────▶│
   │  event: open                       │
   │◀───────────────────────────────────│
   │    → status = "🟢 Conectado"       │
   │    → scheduleTimeout(45s)          │
   │                                    │
```

## 2. Componentes Modificados

### 2.1 app.js — Heartbeat Watchdog

```javascript
// --- Heartbeat Timeout ---
let heartbeatTimer = null;
const HEARTBEAT_TIMEOUT_MS = 45000; // 45s (1.5x server heartbeat interval)

function scheduleHeartbeatTimeout() {
    if (heartbeatTimer) {
        clearTimeout(heartbeatTimer);
    }
    heartbeatTimer = setTimeout(function () {
        updateConnectionStatus(false); // 🔴 Desconectado
    }, HEARTBEAT_TIMEOUT_MS);
}

function resetHeartbeatTimeout() {
    updateConnectionStatus(true);      // 🟢 Conectado
    scheduleHeartbeatTimeout();
}
```

**Pontos de chamada do `resetHeartbeatTimeout()`**:

| Evento | Local | Descrição |
|---|---|---|
| `onopen` | `app.js` | Conexão SSE estabelecida |
| `event: payment` | Event listener | Evento de pagamento recebido |
| `event: heartbeat` | Event listener | Heartbeat recebido |

**O `onerror` não chama `resetHeartbeatTimeout()`** — ele apenas loga o erro. O timeout cuidará da detecção se a conexão realmente cair.

### 2.2 app.js — Status Value Mapping

```javascript
// --- Status Value Mapping (pt-BR) ---
const STATUS_LABELS = {
    pending:   'Pendente',
    confirmed: 'Confirmado',
    failed:    'Falhou',
    refunded:  'Reembolsado'
};

function getStatusLabel(status) {
    return STATUS_LABELS[status] || status || 'desconhecido';
}
```

**Uso**: Substituir `status.toUpperCase()` e `p.status` direto por `getStatusLabel(status)` nos seguintes locais:

| Função | Uso atual | Uso novo |
|---|---|---|
| `addToFeed()` | `event.status.toUpperCase()` | `getStatusLabel(event.status).toUpperCase()` |
| `renderPaymentsTable()` | `p.status` no badge | `getStatusLabel(p.status)` |
| `renderHistoryTable()` | `h.status` no badge | `getStatusLabel(h.status)` |

### 2.3 app.js — updateConnectionStatus

```javascript
function updateConnectionStatus(connected) {
    isConnected = connected;
    connectionStatus.textContent = connected
        ? '🟢 Conectado'
        : '🔴 Desconectado';
    connectionStatus.className = connected ? 'connected' : 'disconnected';
}
```

### 2.4 index.html — Lang Attribute

```html
<html lang="pt-BR">
```

### 2.5 index.html — Status Select Options

```html
<select id="filter-status">
    <option value="">Todos os Status</option>
    <option value="pending">Pendente</option>
    <option value="confirmed">Confirmado</option>
    <option value="failed">Falhou</option>
    <option value="refunded">Reembolsado</option>
</select>
```

**Nota**: O `value` de cada option mantém o valor em inglês (`pending`, `confirmed`, `failed`, `refunded`) porque é usado para filtrar a lista de pagamentos comparando com os status retornados pela API (que estão em inglês).

## 3. Detalhamento das Alterações

### 3.1 Alterações no `index.html`

| Linha (original) | Conteúdo original | Conteúdo novo |
|---|---|---|
| 2 | `<html lang="en">` | `<html lang="pt-BR">` |
| 6 | `<title>Payment Monitor</title>` | `<title>Monitor de Pagamentos</title>` |
| 21 | `<h1>Payment Monitor</h1>` | `<h1>Monitor de Pagamentos</h1>` |
| 22 | `<span class="brand-subtitle">Real-time Payment Processing</span>` | `<span class="brand-subtitle">Processamento de Pagamentos em Tempo Real</span>` |
| 25 | `<div id="connection-status">● Disconnected</div>` | `<div id="connection-status">🔴 Desconectado</div>` |
| 35 | `<label for="filter-payment-id" class="filter-label">Payment ID</label>` | `<label for="filter-payment-id" class="filter-label">ID do Pagamento</label>` |
| 36 | `<input type="text" id="filter-payment-id" placeholder="Search by ID...">` | `<input type="text" id="filter-payment-id" placeholder="Buscar por ID...">` |
| 41 | `<option value="">All Statuses</option>` | `<option value="">Todos os Status</option>` |
| 42 | `<option value="pending">Pending</option>` | `<option value="pending">Pendente</option>` |
| 43 | `<option value="confirmed">Confirmed</option>` | `<option value="confirmed">Confirmado</option>` |
| 44 | `<option value="failed">Failed</option>` | `<option value="failed">Falhou</option>` |
| 45 | `<option value="refunded">Refunded</option>` | `<option value="refunded">Reembolsado</option>` |
| 55 | `<h2>Live Event Feed</h2>` | `<h2>Feed de Eventos</h2>` |
| 56 | `<span class="section-head-badge">Real-time</span>` | `<span class="section-head-badge">Tempo Real</span>` |
| 67 | `<h2>Payments</h2>` | `<h2>Pagamentos</h2>` |
| 75 | `<th>Payment ID</th>` | `<th>ID do Pagamento</th>` |
| 76 | `<th>Status</th>` | `<th>Status</th>` (mantém) |
| 77 | `<th>Updated At</th>` | `<th>Atualizado Em</th>` |
| 78 | `<th>Actions</th>` | `<th>Ações</th>` |
| 92 | `<button class="close" aria-label="Close">&times;</button>` | `<button class="close" aria-label="Fechar">&times;</button>` |
| 93 | `<h2 id="modal-payment-id">Payment History</h2>` | `<h2 id="modal-payment-id">Histórico do Pagamento</h2>` |
| 98 | `<th>Timestamp</th>` | `<th>Timestamp</th>` (mantém) |
| 99 | `<th>Status</th>` | `<th>Status</th>` (mantém) |
| 100 | `<th>Amount</th>` | `<th>Valor</th>` |
| 101 | `<th>Currency</th>` | `<th>Moeda</th>` |
| 102 | `<th>Description</th>` | `<th>Descrição</th>` |
| 103 | `<th>Processed At</th>` | `<th>Processado Em</th>` |
| 104 | `<th>Trace ID</th>` | `<th>ID do Traço</th>` |

### 3.2 Alterações no `app.js`

| Função | Alteração |
|---|---|
| `updateConnectionStatus()` | `'🟢 Conectado'` / `'🔴 Desconectado'` (emojis literais, pt-BR) |
| `connectSSE()` | Adicionar `resetHeartbeatTimeout()` no `onopen`, `payment` event, `heartbeat` event |
| `connectSSE()` | Manter `onerror` apenas para log (não muda status diretamente) |
| `addToFeed()` | Usar `getStatusLabel(event.status)` para exibição |
| `renderPaymentsTable()` | Usar `getStatusLabel(p.status)` para exibição |
| `renderHistoryTable()` | Usar `getStatusLabel(h.status)` para exibição |
| Novo | Adicionar `STATUS_LABELS` map |
| Novo | Adicionar `getStatusLabel()` helper |
| Novo | Adicionar `scheduleHeartbeatTimeout()` |
| Novo | Adicionar `resetHeartbeatTimeout()` |
| Novo | Adicionar variável `heartbeatTimer` |
| Novo | Adicionar constante `HEARTBEAT_TIMEOUT_MS` |

### 3.3 style.css — Sem alterações

O CSS não requer alterações porque:
- As classes CSS (`connected`, `disconnected`, `status-badge`, `event-status`) permanecem as mesmas
- O conteúdo textual está no HTML/JS, não no CSS
- As cores e estilos dos status badges já funcionam com as classes existentes

## 4. Fluxo de Inicialização

```
1. Página carrega
   ├── updateConnectionStatus(false) → "🔴 Desconectado"
   └── connectSSE()
        ├── new EventSource('/api/events')
        ├── onopen → resetHeartbeatTimeout() → "🟢 Conectado", timer 45s
        └── onerror → console.error (apenas log)

2. Heartbeat chega (a cada 30s)
   └── resetHeartbeatTimeout() → "🟢 Conectado", timer reset para 45s

3. Evento de pagamento chega
   └── addToFeed(event)
   └── updatePayment(event)
   └── refreshMetrics()
   └── resetHeartbeatTimeout() → "🟢 Conectado", timer reset para 45s

4. Conexão cai (rede, servidor)
   ├── 45s sem heartbeat/evento
   └── heartbeatTimer dispara → "🔴 Desconectado"
   ├── Browser EventSource auto-reconnect (~3s)
   └── onopen → resetHeartbeatTimeout() → "🟢 Conectado", timer 45s
```

## 5. Edge Cases

### 5.1 Reconexão rápida

Se o navegador reconectar em < 1s, o `onopen` dispara antes do heartbeat timer expirar. O `clearTimeout()` em `scheduleHeartbeatTimeout()` previne que o timer antigo dispare após a reconexão.

### 5.2 Múltiplas abas

Cada aba do navegador mantém seu próprio `heartbeatTimer` e `EventSource`. Não há estado compartilhado entre abas.

### 5.3 Background tab

Navegadores throttlam `setTimeout` em abas em background (Chrome: ≥1s, Firefox: ≥1s). Com 45s, o atraso máximo seria ~46s — ainda aceitável.

### 5.4 Server sem heartbeats

Se o servidor parar de enviar heartbeats (bug, restart), o timeout de 45s detecta a falha e mostra "🔴 Desconectado". Quando o servidor voltar, o browser reconecta e o próximo heartbeat/evento restaura o status.

### 5.5 Timeout exato

O timer de 45s usa `setTimeout`, não `setInterval`. Isso evita o acúmulo de chamadas se a execução for atrasada. Cada reset cria um novo timeout.

## 6. Riscos e Trade-offs

| Risco | Impacto | Probabilidade | Mitigação |
|---|---|---|---|
| Timeout falso positivo durante GC pesado | "Desconectado" por alguns segundos | Baixa | Timeout de 45s (1.5×) absorve pausas |
| Browser não suporta `setTimeout` | Watchdog não funciona | Muito baixa | `setTimeout` existe desde JS 1.0 |
| Timeout não limpo em SPA navigation | Vazamento de timer | N/A (não é SPA navegável) | `clearTimeout` em cada reset |
| `onopen` nunca dispara | Status preso em "Desconectado" | Baixa | Timeout também não será resetado → correto (realmente desconectado) |

## 7. Observabilidade

- **Console**: `console.error` mantido para erros de parse SSE (`app.js`)
- **Não adicionado**: Nenhuma métrica ou log novo — a correção é puramente visual

## 8. Estratégia de Rollback

Reverter o commit que contém as alterações nos arquivos `index.html` e `app.js`. Sem migração de dados necessária.
