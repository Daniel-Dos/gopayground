# 02 — Requisitos

## Requisitos Funcionais

### RF-01: Localização pt-BR do index.html

Todos os textos visíveis no `index.html` devem estar em português-brasileiro:

| Item atual (inglês)          | Deve ser (pt-BR)                       |
|------------------------------|----------------------------------------|
| `<html lang="en">`           | `<html lang="pt-BR">`                  |
| "Payment Monitor"            | "Monitor de Pagamentos"               |
| "Real-time Payment Processing" | "Processamento de Pagamentos em Tempo Real" |
| "Payment ID"                 | "ID do Pagamento"                      |
| "Search by ID..."            | "Buscar por ID..."                     |
| "Status" (label)             | "Status" (mantém, mesmo em pt-BR)      |
| "All Statuses"               | "Todos os Status"                      |
| "Pending"                    | "Pendente"                             |
| "Confirmed"                  | "Confirmado"                           |
| "Failed"                     | "Falhou"                               |
| "Refunded"                   | "Reembolsado"                          |
| "Live Event Feed"            | "Feed de Eventos"                      |
| "Real-time" (badge)          | "Tempo Real"                           |
| "Payments"                   | "Pagamentos"                           |
| "Updated At"                 | "Atualizado Em"                        |
| "Actions"                    | "Ações"                                |
| "View History"               | "Ver Histórico"                        |
| "Payment History"            | "Histórico do Pagamento"               |
| "Timestamp" (header)         | "Timestamp" (mantém, termo técnico)    |
| "Amount" (header)            | "Valor"                                |
| "Currency" (header)          | "Moeda"                                |
| "Description" (header)       | "Descrição"                            |
| "Processed At" (header)      | "Processado Em"                        |
| "Trace ID" (header)          | "ID do Traço"                          |
| "No payments found."         | "Nenhum pagamento encontrado."         |
| "No history records found."  | "Nenhum registro de histórico encontrado." |
| "Loading..."                 | "Carregando..."                        |
| "Close" (aria-label)         | "Fechar"                               |

### RF-02: Localização pt-BR do app.js

| Item atual (inglês)         | Deve ser (pt-BR)               |
|-----------------------------|--------------------------------|
| "Connected"                 | "Conectado"                    |
| "Disconnected"              | "Desconectado"                 |
| status badge "pending"      | "Pendente"                     |
| status badge "confirmed"    | "Confirmado"                   |
| status badge "failed"       | "Falhou"                       |
| status badge "refunded"     | "Reembolsado"                  |
| "View History"              | "Ver Histórico"                |
| "Failed to load history"    | "Falha ao carregar histórico"  |

### RF-03: Status value mapping

O `app.js` deve conter um mapeamento centralizado para converter valores internos de status (em inglês) para exibição em pt-BR:

```javascript
const STATUS_LABELS = {
    pending:   'Pendente',
    confirmed: 'Confirmado',
    failed:    'Falhou',
    refunded:  'Reembolsado'
};
```

Este mapeamento deve ser usado em todos os locais onde o status é exibido:
- Feed de eventos (status badges)
- Tabela de pagamentos (status badges)
- Modal de histórico (status badges)

### RF-04: Heartbeat timeout pattern

O `app.js` deve implementar um watchdog de heartbeat:

1. Ao conectar SSE (evento `onopen`), iniciar timer de 45s
2. Ao receber qualquer dado SSE (evento `payment` ou `heartbeat`), resetar timer para 45s e marcar como "Conectado"
3. Se o timer expirar (45s sem dados), marcar como "Desconectado"
4. O timer deve ser limpo (`clearTimeout`) antes de cada reset para evitar timers órfãos

```javascript
// Comportamento esperado
onSSEDataReceived() → status = "🟢 Conectado", reset timer to 45s
no data for 45s     → status = "🔴 Desconectado"
data arrives after 45s → status = "🟢 Conectado", reset timer
```

### RF-05: Emojis literais

Todos os emojis usados no código devem ser caracteres literais, não escapes Unicode:

```javascript
// ❌ Antes (evitar):
connectionStatus.textContent = connected ? '\u{1F7E2} Connected' : '\u{1F534} Disconnected';

// ✅ Depois (usar):
connectionStatus.textContent = connected ? '🟢 Conectado' : '🔴 Desconectado';
```

### RF-06: Conexão inicial

Na primeira carga da página, o status deve iniciar como "Desconectado" até que o primeiro evento SSE seja recebido (ou o `onopen` dispare com sucesso).

## Requisitos Não Funcionais

### RNF-01: Zero dependências

A localização e a correção da conexão não devem adicionar nenhuma nova dependência (npm, Go module, etc.).

### RNF-02: Sem alterações no backend

Nenhum arquivo Go deve ser modificado. Todas as alterações devem ser confinadas aos arquivos estáticos (`index.html`, `app.js`).

### RNF-03: Compatibilidade com navegadores

O heartbeat timeout pattern deve funcionar em navegadores modernos (Chrome 90+, Firefox 90+, Safari 15+, Edge 90+). A API `setTimeout`/`clearTimeout` é suportada em todos.

### RNF-04: Tempo de detecção de desconexão

O status "Desconectado" deve aparecer em no máximo 60s após a perda real de conexão (45s de heartbeat timeout + latência de rede).

### RNF-05: Falso positivo rate

A taxa de falsos positivos (status "Desconectado" quando a conexão está ativa) deve ser < 1%. O timeout de 45s (1.5× o heartbeat de 30s) oferece tolerância suficiente para jitter de rede e pausas do GC.

## Restrições

1. A pasta de arquivos estáticos é `internal/ui/static/`
2. Os arquivos são embutidos no binário Go via `//go:embed static/*`
3. O backend envia heartbeats a cada 30s (`handlers.go:102,120-122`)
4. A API retorna status em inglês (`pending`, `confirmed`, `failed`, `refunded`) — isso **não** deve mudar

## Critérios de Aceitação

1. [ ] Todos os textos visíveis na UI estão em pt-BR
2. [ ] O indicador de conexão mostra "🟢 Conectado" quando conectado
3. [ ] O indicador de conexão mostra "🔴 Desconectado" quando desconectado
4. [ ] Ao desconectar a rede, o status muda para "🔴 Desconectado" em até 60s
5. [ ] Ao reconectar a rede, o status muda para "🟢 Conectado" automaticamente
6. [ ] Status badges mostram: Pendente, Confirmado, Falhou, Reembolsado
7. [ ] Nenhum escape Unicode (`\u{...}`) permanece nos arquivos JS/HTML
8. [ ] Nenhum arquivo Go foi modificado
9. [ ] Nenhuma nova dependência foi adicionada
