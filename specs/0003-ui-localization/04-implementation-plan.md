# 04 — Plano de Implementação

## Etapas

A implementação da localização pt-BR e correção do indicador de conexão SSE
é inteiramente **client-side**, sem alterações no backend Go. Todas as
modificações são confinadas aos arquivos estáticos em `internal/ui/static/`.

---

### Etapa 1 — Localizar index.html (pt-BR)

**Arquivo**: `internal/ui/static/index.html`

1. Alterar `<html lang="en">` para `<html lang="pt-BR">`
2. Traduzir o título da página: `<title>Monitor de Pagamentos</title>`
3. Traduzir o cabeçalho principal:
   - `<h1>Monitor de Pagamentos</h1>`
   - `<span class="brand-subtitle">Processamento de Pagamentos em Tempo Real</span>`
4. Traduzir todos os labels, placeholders e textos da interface:
   - Labels dos filtros: "ID do Pagamento", "Situação"
   - Placeholder do input: "Buscar por ID..."
   - Options do select: "Todos", "Pendente", "Confirmado", "Falhou", "Reembolsado"
   - Título das seções: "Feed de Eventos ao Vivo", "Pagamentos"
   - Headers da tabela: "ID do Pagamento", "Situação", "Atualizado Em", "Ações"
   - Modal: "Histórico do Pagamento", headers "Data/Hora", "Valor", "Moeda", "Descrição", "Processado Em", "ID de Rastreamento"
5. Traduzir o status inicial de conexão: `🔴 Desconectado`
6. Traduzir `aria-label` do botão de fechar modal para "Close" → "Fechar"

**Total de alterações**: ~30 linhas alteradas em `index.html`.

### Etapa 2 — Implementar Heartbeat Timeout Pattern em app.js

**Arquivo**: `internal/ui/static/app.js`

Adicionar watchdog baseado em timeout para detecção de desconexão SSE,
substituindo a lógica anterior que dependia exclusivamente de `onopen`/`onerror`.

```javascript
var heartbeatTimeout = null;
var HEARTBEAT_TIMEOUT_MS = 45000;

function resetHeartbeatTimeout() {
    if (heartbeatTimeout) clearTimeout(heartbeatTimeout);
    heartbeatTimeout = setTimeout(function () {
        updateConnectionStatus(false);
    }, HEARTBEAT_TIMEOUT_MS);
}
```

**Pontos de chamada**:
- `eventSource.addEventListener('heartbeat', ...)` → chamar `resetHeartbeatTimeout()`
- `eventSource.onopen` → chamar `updateConnectionStatus(true)` + `resetHeartbeatTimeout()`
- `eventSource.onerror` → apenas log (não altera status)
- Inicialização: chamar `resetHeartbeatTimeout()` após criar o EventSource

### Etapa 3 — Implementar Status Value Mapping em app.js

**Arquivo**: `internal/ui/static/app.js`

Adicionar mapeamento centralizado para status:

```javascript
var statusMap = {
    pending: 'Pendente',
    confirmed: 'Confirmado',
    failed: 'Falhou',
    refunded: 'Reembolsado'
};

var statusLabelMap = {
    pending: 'Pendentes',
    confirmed: 'Confirmados',
    failed: 'Falhas',
    refunded: 'Reembolsados'
};
```

**Uso**:
- Feed de eventos: `statusMap[event.status]` em badges
- Tabela de pagamentos: `statusMap[p.status]` em badges
- Tabela de histórico: `statusMap[h.status]` em badges
- Métricas: `statusLabelMap[status]` para labels

### Etapa 4 — Localizar app.js (pt-BR + Emojis literais)

**Arquivo**: `internal/ui/static/app.js`

1. Substituir `'🟢 Conectado'` / `'🔴 Desconectado'` em `updateConnectionStatus()`
   - Usar emojis literais (não escapes Unicode)
2. Traduzir mensagens de texto:
   - `'Nenhum pagamento encontrado.'`
   - `'Falha ao carregar histórico'`
   - `'Carregando...'`
   - `'Ver Histórico'`
   - `'Nenhum registro de histórico encontrado.'`
3. Traduzir labels de métricas: "Total Processado", "Taxa de Sucesso", "DLQ Contagem"

### Etapa 5 — Adicionar Enhancement Scripts (index.html)

**Arquivo**: `internal/ui/static/index.html`

Adicionar script de enhancement visual no final do `<body>` com:

1. **Ícones em métricas**: MutationObserver que adiciona emoji correspondente
   ao título de cada card de métrica (ex: "Total Processado" → 📊)
2. **Extrair timestamp do feed**: MutationObserver que identifica o padrão
   `@ timestamp` no feed de eventos e extrai para um elemento `<span>` separado
3. **Contagem de linhas**: MutationObserver que atualiza o contador de
   pagamentos na seção de tabela

---

## Ordem de Implementação Sugerida

| Ordem | Tarefa                          | Arquivo(s)                | Depende de |
|-------|---------------------------------|---------------------------|------------|
| 1     | Traduzir textos do index.html   | `index.html`              | —          |
| 2     | Adicionar heartbeat timeout     | `app.js`                  | —          |
| 3     | Adicionar status value mapping  | `app.js`                  | —          |
| 4     | Localizar textos do app.js      | `app.js`                  | 2,3        |
| 5     | Adicionar enhancement scripts   | `index.html`              | 1          |
| 6     | Validar visualmente             | Navegador                 | 1-5        |

## Dependências

- Nenhuma nova dependência Go ou JavaScript
- Nenhuma alteração em `style.css`, `server.go`, `handlers.go` ou backend Go
- Nenhuma alteração em configuração de infraestrutura

## Checklist de Build

```bash
go build ./cmd/ui/            # deve compilar sem erros
go vet ./internal/ui/         # sem warnings
```

> Como as alterações são apenas em arquivos estáticos (HTML/JS),
> não há necessidade de rebuild do binário se o `go:embed` não for
> afetado. Apenas alterar os arquivos e recarregar a página no navegador
> (em desenvolvimento com reload a quente) ou rebuildar o binário.

---

## Estrutura de Pastas (inalterada)

```
internal/ui/static/
├── index.html    ← Alterado: localização pt-BR + enhancement scripts
├── app.js        ← Alterado: heartbeat timeout + status mapping + localização
├── style.css     ← Sem alterações
└── ...           ← Demais arquivos
```
