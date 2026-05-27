# 07 — Hardening

## Resiliência

### Heartbeat Timeout Watchdog

O watchdog de conexão SSE é a principal medida de resiliência desta feature.

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

**Comportamento sob falha**:

| Cenário | Comportamento | Tempo de Detecção |
|---------|--------------|-------------------|
| Servidor cai (não envia mais heartbeats) | Timeout de 45s dispara → "🔴 Desconectado" | ~45s |
| Rede cai (half-open TCP) | Timeout de 45s dispara → "🔴 Desconectado" | ~45s |
| Servidor volta (reconexão automática) | `onopen` + heartbeat → "🟢 Conectado" | ~3s (reconexão) |
| Heartbeat atrasado (GC, carga) | Margem de 15s (45s - 30s) absorve atraso | Nenhum (dentro da margem) |
| Múltiplas abas | Cada aba tem seu próprio timer e EventSource | Independente |

### Proteção contra Timers Órfãos

```javascript
function resetHeartbeatTimeout() {
    if (heartbeatTimeout) clearTimeout(heartbeatTimeout); // ← Limpa timer anterior
    heartbeatTimer = setTimeout(function () { ... }, HEARTBEAT_TIMEOUT_MS);
}
```

- `clearTimeout` é SEMPRE chamado antes de criar um novo timer
- Isso previne o acúmulo de timers em cenários de:
  - Reconexão rápida (múltiplos `onopen` em sequência)
  - Múltiplos eventos simultâneos (rajada de payments)
  - Reset manual repetido

### Fallback de Status Desconhecido

```javascript
var statusMap = {
    pending: 'Pendente',
    confirmed: 'Confirmado',
    failed: 'Falhou',
    refunded: 'Reembolsado'
};
```

- Se um status não estiver no mapa, o valor original (em inglês) é exibido
- Nenhum crash ou erro se `event.status` for `undefined` ou `null`
- O fallback é: `statusMap[event.status] || event.status || '—'`

---

## Concorrência

### Single-threaded JavaScript

Toda a lógica roda em single thread (JavaScript no navegador). Não há
concorrência real. Os riscos de race condition se limitam a:

- **Timers sobrepostos**: mitigado pelo `clearTimeout` antes de cada reset
- **MutationObserver vs. renderização**: observers disparam assincronamente,
  mas não concorrem com a thread principal (microtask queue)

### EventSource Thread Safety

- `EventSource` é thread-safe por design no navegador
- Callbacks (`onopen`, `onerror`, `addEventListener`) são chamados na
  event loop principal, nunca em paralelo
- Nenhum mutex ou lock necessário

---

## Observabilidade

### Logs no Console

Eventos relevantes são logados para diagnóstico:

```javascript
// Erro de parsing SSE
catch (err) {
    console.error('Falha ao processar evento SSE:', err);
}

// Falha ao carregar histórico
console.error('Falha ao carregar histórico:', err);

// Falha ao carregar dados iniciais
console.error('Failed to load initial data:', err);
```

### Indicador Visual de Conexão

O status de conexão é o principal instrumento de observabilidade para o
operador:

| Estado | Indicador | Cor | Classe CSS |
|--------|-----------|-----|------------|
| Conectado | 🟢 Conectado | Verde | `connected` |
| Desconectado | 🔴 Desconectado | Vermelho | `disconnected` |

### Métricas de Frontend (não implementado)

Para versão futura, considerar coletar:

- `sse_disconnections`: contagem de eventos `onerror`
- `heartbeat_timeouts`: contagem de timeouts disparados
- `status_display_duration`: tempo gasto em cada estado de conexão

---

## Segurança Operacional

### XSS Prevention

- **`textContent` em vez de `innerHTML`**: todas as inserções de dados da API
  no DOM usam `textContent`, prevenindo injeção de HTML/script
- **Parsing seguro**: `JSON.parse(e.data)` no SSE é seguro (dados são JSON puro
  do backend)
- **Nenhum dado da API é interpolado em HTML**: apenas em `textContent`

### Content-Security-Policy (CSP)

A CSP existente no middleware Go cobre os requisitos:

```
default-src 'self';
style-src 'self' https://fonts.googleapis.com 'unsafe-inline';
font-src 'self' https://fonts.gstatic.com;
script-src 'self' 'unsafe-inline';
img-src 'self' data:;
connect-src 'self';
```

**Verificações**:
- ✅ `connect-src 'self'`: permite SSE e fetch para `/api/*` (mesma origem)
- ✅ `script-src 'self' 'unsafe-inline'`: permite scripts inline no `index.html`
- ✅ `img-src 'self' data:`: permite emojis e ícones inline

### Validação de Entrada

- Dados SSE são validados via `JSON.parse` — se falhar, o erro é capturado
  e logado, sem quebrar a página
- `event.status` é validado contra o mapa de status — se não existir,
  fallback seguro
- Nenhuma alteração no backend significa que as validações existentes
  (validator, handlers) continuam intactas

### Proteção de Dados

- Nenhum dado sensível é exposto nos logs do console
- Os textos localizados são strings estáticas, não refletem dados do usuário

---

## Produção

### Compatibilidade com Navegadores

| Navegador | Versão Mínima | Observação |
|-----------|---------------|------------|
| Chrome | 90+ | `setTimeout`/`clearTimeout` suportado |
| Firefox | 90+ | Íntegro |
| Safari | 15+ | Íntegro |
| Edge | 90+ | Íntegro |

**Funcionalidades usadas**:
- `EventSource`: suportado desde Chrome 9, Firefox 33, Safari 5, Edge 79
- `setTimeout`/`clearTimeout`: suportado desde JS 1.0 (todos os navegadores)
- `MutationObserver`: suportado desde Chrome 18, Firefox 14, Safari 6, Edge 79
- `textContent`: suportado desde IE 9 (todos os navegadores modernos)

### Comportamento sob Falha Parcial

| Falha | Impacto | Comportamento |
|-------|---------|--------------|
| SSE falha na conexão inicial | Feed sem eventos, status "🔴 Desconectado" | Polling de métricas a cada 30s como fallback |
| Heartbeat não chega por 45s+ | Status "🔴 Desconectado" | Reconexão automática do EventSource restaura status |
| API de métricas fora do ar | Cards de métricas mostram "N/A" | Tabela de pagamentos ainda funciona |
| API de histórico fora do ar | Modal mostra erro "Falha ao carregar histórico" | Demais funcionalidades intactas |
| JavaScript desabilitado | Página sem funcionalidade | `<noscript>` informa necessidade de JS |

### Rollback

Como as alterações são exclusivamente client-side:

1. Reverter `index.html` e `app.js` para a versão anterior (em inglês)
2. Se os enhancement scripts foram adicionados, removê-los do `index.html`
3. Recarregar a página no navegador (ou rebuildar o binário se `go:embed`
   foi recompilado)
4. Verificar que a dashboard voltou ao estado original

> **Nota**: Não há migração de dados, alteração de schema ou modificação
> no backend. Rollback é instantâneo e sem efeitos colaterais.

### Monitoramento Pós-Deploy

Checklist para primeiras horas após deploy:

- [ ] Página carrega sem erros no console
- [ ] Status SSE mostra "🟢 Conectado" após conexão
- [ ] Badges de status exibem textos em pt-BR
- [ ] Filtro por status funciona (options em pt-BR)
- [ ] Modal de histórico abre e fecha corretamente
- [ ] Enhancement scripts: ícones nas métricas aparecem
- [ ] Enhancement scripts: timestamp extraído no feed
- [ ] Enhancement scripts: contagem de linhas na tabela

---

## Checklist de Hardening

| # | Item | Aplicável | Verificado |
|---|------|-----------|------------|
| H1 | `clearTimeout` antes de resetar timer | ✅ | ⬜ |
| H2 | Fallback de status desconhecido (inglês) | ✅ | ⬜ |
| H3 | `textContent` em vez de `innerHTML` | ✅ | ⬜ |
| H4 | Try/catch no parsing SSE | ✅ | ⬜ |
| H5 | CSP compatível (sem alterações necessárias) | ✅ | ⬜ |
| H6 | Nenhum dado sensível em log | ✅ | ⬜ |
| H7 | Timers não acumulam (único heartbeatTimer) | ✅ | ⬜ |
| H8 | Reconexão automática detectada corretamente | ✅ | ⬜ |
| H9 | Compatibilidade cross-browser | ✅ | ⬜ |
| H10 | Rollback sem efeitos colaterais | ✅ | ⬜ |
| H11 | Nenhuma alteração no backend Go | ✅ | ⬜ |
| H12 | Enhancement scripts não interferem na funcionalidade principal | ✅ | ⬜ |
