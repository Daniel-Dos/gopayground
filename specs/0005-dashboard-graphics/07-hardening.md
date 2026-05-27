# 07 — Hardening

## Resiliência

### Timeout nas Requisições de Polling

Toda requisição para `/api/metrics` deve usar `AbortController` com timeout de **5 segundos**:

```javascript
async function fetchMetrics() {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 5000);

    try {
        const resp = await fetch('/api/metrics', {
            signal: controller.signal
        });
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        return await resp.json();
    } catch (err) {
        if (err.name === 'AbortError') {
            console.warn('Métricas: requisição abortada por timeout');
        }
        throw err;
    } finally {
        clearTimeout(timeoutId);
    }
}
```

### Retry com Backoff na Primeira Carga

Na carga inicial, tentar até **3 vezes** com backoff exponencial (1s, 2s, 4s) antes de mostrar erro definitivo:

```javascript
async function loadInitialMetrics(retries = 3) {
    for (let i = 0; i < retries; i++) {
        try {
            const data = await fetchMetrics();
            state.metrics = data;
            hideLoading();
            updateAllCharts(data);
            return;
        } catch (err) {
            if (i === retries - 1) {
                showError('API de métricas indisponível após múltiplas tentativas');
                hideLoading();
                return;
            }
            await new Promise(r => setTimeout(r, 1000 * Math.pow(2, i)));
        }
    }
}
```

### Polling sem Acúmulo

Usar flag para evitar requisições concorrentes:

```javascript
let isPolling = false;

async function pollMetrics() {
    if (isPolling) return; // Não acumular
    isPolling = true;
    try {
        const data = await fetchMetrics();
        state.metrics = data;
        hideError();
        updateAllCharts(data);
    } catch (err) {
        // Manter último estado válido
        showError('API de métricas indisponível');
    } finally {
        isPolling = false;
    }
}

// Iniciar polling
setInterval(pollMetrics, 10000);
```

### Graceful Degradation da API

| Cenário                          | Comportamento                                                    |
|----------------------------------|------------------------------------------------------------------|
| API retorna 5xx                  | Manter últimos dados válidos, mostrar aviso                     |
| API retorna 4xx                  | Manter últimos dados, mostrar erro (pode ser problema de permissão) |
| API não responde (timeout)       | Tentar novamente no próximo ciclo, sem bloquear UI              |
| API retorna JSON malformado      | `try/catch` no parse, manter dados anteriores                   |
| API retorna dados parciais       | Validar campos obrigatórios antes de renderizar                 |
| SSE desconecta                   | Polling continua normalmente, status "Desconectado" exibido     |
| Reconexão SSE                    | Automática (EventSource), status volta a "Conectado"            |

### Validação de Dados no Frontend

Antes de renderizar, validar estrutura do payload:

```javascript
function validateMetrics(data) {
    if (!data || typeof data !== 'object') return false;
    if (typeof data.total_processed !== 'number') return false;
    if (!data.by_status || typeof data.by_status !== 'object') return false;
    if (typeof data.success_rate !== 'number') return false;
    if (typeof data.dlq_count !== 'number') return false;
    return true;
}

// Uso:
if (!validateMetrics(data)) {
    console.error('Dados de métricas inválidos:', data);
    showError('Dados de métricas em formato inesperado');
    return;
}
```

---

## Concorrência

### Race Condition no Polling + SSE

Tanto o polling (10s) quanto o evento SSE podem disparar `fetchMetrics` simultaneamente. Use debounce no SSE:

```javascript
let sseRefreshTimeout = null;

es.addEventListener('payment', (e) => {
    state.sessionEventCount++;
    updateSessionCounter();

    // Debounce: se já tem um refresh agendado, não agenda outro
    if (sseRefreshTimeout) return;
    sseRefreshTimeout = setTimeout(() => {
        sseRefreshTimeout = null;
        pollMetrics(); // Respeita a flag isPolling
    }, 2000); // Espera 2s após último evento
});
```

### Resize Handler com Debounce

```javascript
let resizeTimeout = null;
window.addEventListener('resize', () => {
    clearTimeout(resizeTimeout);
    resizeTimeout = setTimeout(() => {
        if (state.metrics) {
            updateAllCharts(state.metrics);
        }
    }, 200);
});
```

### Animation Frame Management

Usar `requestAnimationFrame` com controle para evitar múltiplas animações simultâneas:

```javascript
let animationId = null;

function animateChart(renderFn, duration) {
    if (animationId) {
        cancelAnimationFrame(animationId);
    }
    const start = performance.now();
    function frame(timestamp) {
        const progress = Math.min((timestamp - start) / duration, 1);
        renderFn(progress);
        if (progress < 1) {
            animationId = requestAnimationFrame(frame);
        }
    }
    animationId = requestAnimationFrame(frame);
}
```

---

## Observabilidade

### Logs no Console

```javascript
// Eventos relevantes com nível apropriado
console.info('[Dashboard] Métricas carregadas:', {
    total: data.total_processed,
    successRate: data.success_rate,
    dlq: data.dlq_count
});

console.warn('[Dashboard] API de métricas indisponível', {
    error: err.message,
    timestamp: new Date().toISOString()
});

console.info('[Dashboard] SSE conectado');
console.warn('[Dashboard] SSE desconectado');

console.debug('[Dashboard] Gráfico renderizado:', {
    chart: 'donut',
    duration: `${end - start}ms`
});
```

### Métricas de Frontend (Auto-Monitoramento)

Para versão futura, considerar coletar:

| Métrica                        | Como Coletar                             |
|--------------------------------|------------------------------------------|
| `dashboard_metrics_load_time`  | Tempo entre fetch e renderização completa|
| `dashboard_metrics_error_total`| Contador de erros de polling             |
| `dashboard_sse_disconnections` | Contador de eventos `onerror` do SSE     |
| `dashboard_chart_render_time`  | Performance.now() antes/depois de render |

### Health Check Visual

A página deve indicar visualmente se está saudável:
- Badge "Tempo Real" verde se SSE conectado e métricas atualizadas
- Badge "Tempo Real" amarelo se apenas polling ativo (sem SSE)
- Badge "Tempo Real" vermelho se sem dados

---

## Segurança Operacional

### Content-Security-Policy (CSP)

A CSP existente no middleware Go já cobre os requisitos:

```
default-src 'self';
style-src 'self' https://fonts.googleapis.com 'unsafe-inline';
font-src 'self' https://fonts.gstatic.com;
script-src 'self' 'unsafe-inline';
img-src 'self' data:;
connect-src 'self';
```

**Verificações:**
- ✅ `connect-src 'self'` permite fetch para `/api/metrics` e `/api/events` (mesma origem)
- ✅ `script-src 'self' 'unsafe-inline'` permite o JS inline e o `dashboard.js`
- ✅ `img-src 'self' data:` permite ícone inline e canvas (data: já incluso)
- ✅ `style-src 'self' ... 'unsafe-inline'` permite estilos inline no dashboard.html

**Sem necessidade de alteração na CSP.**

### Proteção contra XSS

- Todo conteúdo da API é `JSON.parse` seguro (API retorna JSON puro)
- Inserção no DOM usa `textContent`, não `innerHTML`
- Canvas API desenha gráficos, não executa HTML
- Atributos `aria-label` usam strings fixas ou interpoladas com dados numéricos

### Validação de Entrada

- Dados da API validados por schema antes de renderizar
- Dados que não passam validação são ignorados, não renderizados
- Nenhum dado da API é interpolado em HTML (apenas Canvas + textContent)

### Headers de Segurança

Headers já aplicados pelo middleware existente:

| Header                     | Valor             | Proteção                    |
|----------------------------|-------------------|-----------------------------|
| `X-Content-Type-Options`   | `nosniff`         | MIME type sniffing          |
| `X-Frame-Options`          | `DENY`            | Clickjacking                |
| `Referrer-Policy`          | `no-referrer`     | Vazamento de URL            |
| `Content-Security-Policy`  | (ver acima)       | XSS, data injection         |

### CORS

Não necessário. Frontend e backend na mesma porta (8081).

---

## Produção

### Graceful Degradation

A página deve funcionar em **modo degradado** quando componentes falham:

| Componente com Falha | Modo Degradado                                          |
|----------------------|---------------------------------------------------------|
| API de métricas      | Últimos dados exibidos, erro amigável, polling contínuo |
| SSE                  | Polling mantém atualização, indicador "Desconectado"    |
| Canvas API           | `<canvas>` não suportado → mensagem "Navegador não suporta Canvas" |
| JavaScript off       | `<noscript>` com mensagem                               |

### Comportamento sob Falha Parcial

| Falha Parcial                        | Impacto                                              |
|--------------------------------------|------------------------------------------------------|
| Redis lento (API responde devagar)   | Polling timeout de 5s aborta requisição              |
| Consumer parou de produzir eventos   | SSE sem eventos, polling mantém dados estáticos      |
| Alta latência de rede                | Timeout de 5s, retry com backoff na carga inicial    |
| Memória do navegador baixa           | `devicePixelRatio` limitado, menos resolução         |

### Rollback

Se necessário reverter a feature:

1. **Backend:** Remover função `serveDashboardPage` e rota `GET /dashboard` de `server.go`
2. **Frontend:** Remover `static/dashboard.html` e `static/dashboard.js`
3. **Navegação:** Remover link `/dashboard` de `index.html` e `producer.html`
4. Build e deploy do binário

### Monitoramento Pós-Deploy

Checklist para primeiras 24h após deploy:

- [ ] Página `/dashboard` carrega sem erros 5xx
- [ ] Donut chart renderiza com dados reais
- [ ] Gauge chart mostra taxa de sucesso correta
- [ ] Bar chart exibe 24 barras com distribuição
- [ ] Session counter incrementa com eventos SSE
- [ ] Polling de 10s mantém dados atualizados
- [ ] Conexão SSE estabelecida (indicador verde)
- [ ] Responsivo em mobile (< 768px) — gráficos empilhados
- [ ] Navegação funciona nas 3 páginas (/, /producer, /dashboard)
- [ ] Sem warnings de CSP no console do navegador
- [ ] Erro de API tratado corretamente (simular desligando Redis)

---

## Checklist de Hardening

| #  | Item                                                | Aplicável | Verificado |
|----|-----------------------------------------------------|-----------|------------|
| H1 | Timeout de 5s no fetch                              | ✅        | ⬜         |
| H2 | AbortController para cancelar requisição            | ✅        | ⬜         |
| H3 | Retry com backoff na carga inicial                  | ✅        | ⬜         |
| H4 | Flag de polling para evitar concorrência           | ✅        | ⬜         |
| H5 | Validação de schema dos dados da API                | ✅        | ⬜         |
| H6 | Debounce no refresh via SSE                         | ✅        | ⬜         |
| H7 | Debounce no resize                                  | ✅        | ⬜         |
| H8 | Cancelamento de animation frame anterior            | ✅        | ⬜         |
| H9 | Logs no console (info/warn/error)                   | ✅        | ⬜         |
| H10| CSP compatível com a página                         | ✅        | ⬜         |
| H11| Uso de textContent em vez de innerHTML              | ✅        | ⬜         |
| H12| Canvas com role="img" e aria-label                  | ✅        | ⬜         |
| H13| Graceful degradation sem API                       | ✅        | ⬜         |
| H14| Graceful degradation sem SSE                       | ✅        | ⬜         |
| H15| Graceful degradation sem Canvas                    | ✅        | ⬜         |
| H16| Noscript para JS desabilitado                      | ✅        | ⬜         |
| H17| Limpeza de timers no descarte da página            | ✅        | ⬜         |
| H18| devicePixelRatio limitado para performance         | ✅        | ⬜         |
