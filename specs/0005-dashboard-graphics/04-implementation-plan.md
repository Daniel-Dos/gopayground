# 04 — Plano de Implementação

## Etapas de Implementação

---

### Etapa 1: Criar arquivo `dashboard.html`

**Arquivo:** `internal/ui/static/dashboard.html`

**Ações:**
1. Criar documento HTML completo seguindo o design definido em `03-design.md`
2. Incluir links para Google Fonts (Plus Jakarta Sans, DM Sans, JetBrains Mono)
3. Incluir link para `style.css`
4. Estruturar o layout com:
   - Header com navegação (`/`, `/producer`, `/dashboard`)
   - Grid de charts com 3 colunas (donut, gauge, counter)
   - Bar chart full-width
   - DLQ section
   - Loading overlay
5. Garantir que a classe `active` está no link `/dashboard`
6. Incluir `<script src="dashboard.js"></script>` ao final do body

**Referência:** Seguir o padrão de `index.html` para header e conexão.

---

### Etapa 2: Criar arquivo `dashboard.js`

**Arquivo:** `internal/ui/static/dashboard.js`

**Ações:**

#### 2.1 Estrutura Inicial
```javascript
(function () {
    'use strict';

    // State
    const state = {
        metrics: null,
        sessionEventCount: 0,
        isConnected: false
    };

    // DOM references
    // ... (caching de elementos)

    // Init
    function init() {
        setupCanvases();
        connectSSE();
        loadInitialMetrics();
        startPolling();
        setupResizeHandler();
    }
})();
```

#### 2.2 Sistema de Cores via CSS Variables
- Implementar função `getCSSVar(name)` para ler variáveis CSS
- Mapear todas as cores necessárias:
  - `--color-accent`, `--color-accent-light` (barras)
  - `--color-success`, `--color-error`, `--color-warning`, `--color-info` (donut)
  - `--color-text`, `--color-text-muted` (rótulos)
  - `--color-border` (gridlines)

#### 2.3 Canvas Setup
- Configurar canvas com `devicePixelRatio` para retina
- Função `setupCanvas(canvasId, width, height)` que:
  - Ajusta atributos `width`/`height` do canvas × `devicePixelRatio`
  - Ajusta CSS `width`/`height` para tamanho lógico
  - Aplica escala no contexto (`ctx.scale(dpr, dpr)`)

#### 2.4 Donut Chart
- Função `drawDonutChart(canvas, data)`:
  ```javascript
  function drawDonutChart(canvas, byStatus) {
      const ctx = canvas.getContext('2d');
      const total = Object.values(byStatus).reduce((a, b) => a + b, 0);
      if (total === 0) return drawEmptyDonut(canvas);
      
      const colors = {
          pending: getCSSVar('--color-warning'),
          confirmed: getCSSVar('--color-success'),
          failed: getCSSVar('--color-error'),
          refunded: getCSSVar('--color-info')
      };
      
      const cx = canvas.width / 2 / dpr;
      const cy = canvas.height / 2 / dpr;
      const outerR = 90;
      const innerR = 55;
      
      // Animated arc drawing using requestAnimationFrame
      let startAngle = -Math.PI / 2;
      let currentProgress = 0;
      const targetProgress = 1;
      const duration = 800; // ms
      
      function animate(timestamp) {
          // ... desenho incremental dos arcos
      }
      requestAnimationFrame(animate);
  }
  ```

#### 2.5 Gauge Chart
- Função `drawGaugeChart(canvas, successRate)`:
  ```javascript
  function drawGaugeChart(canvas, rate) {
      const ctx = canvas.getContext('2d');
      const cx = canvas.width / 2 / dpr;
      const cy = canvas.height * 0.75 / dpr; // centro deslocado para baixo
      const radius = 80;
      
      // Cor baseada no threshold
      let color;
      if (rate < 70) color = getCSSVar('--color-error');
      else if (rate < 90) color = getCSSVar('--color-warning');
      else color = getCSSVar('--color-success');
      
      // Desenhar arco de fundo (semi-círculo)
      // Desenhar arco preenchido (animado)
      // Desenhar ticks e labels
      // Desenhar texto central com percentual
  }
  ```

#### 2.6 Bar Chart
- Função `drawBarChart(canvas, data)`:
  ```javascript
  function drawBarChart(canvas, buckets) {
      const ctx = canvas.getContext('2d');
      const dpr = window.devicePixelRatio || 1;
      
      // 24 barras
      const barCount = 24;
      const padding = { top: 20, right: 16, bottom: 40, left: 40 };
      // ... lógica de desenho
      
      // Gradiente para as barras
      const gradient = ctx.createLinearGradient(0, 0, 0, chartHeight);
      gradient.addColorStop(0, getCSSVar('--color-accent'));
      gradient.addColorStop(1, getCSSVar('--color-accent-light'));
  }
  ```

#### 2.7 Session Counter
- Função `updateSessionCounter()`:
  ```javascript
  function updateSessionCounter() {
      const el = document.getElementById('session-counter');
      el.textContent = state.sessionEventCount;
      el.style.transform = 'scale(1.15)';
      setTimeout(() => { el.style.transform = 'scale(1)'; }, 150);
  }
  ```

#### 2.8 DLQ Counter
- Função `updateDLQCount(count)`:
  ```javascript
  function updateDLQCount(count) {
      document.getElementById('dlq-count').textContent = count;
  }
  ```

#### 2.9 Polling e Atualização
```javascript
function loadInitialMetrics() {
    fetchMetrics()
        .then(data => {
            state.metrics = data;
            hideLoading();
            updateAllCharts(data);
        })
        .catch(err => {
            showError('API de métricas indisponível');
            hideLoading();
        });
}

function startPolling() {
    setInterval(async () => {
        try {
            const data = await fetchMetrics();
            state.metrics = data;
            hideError();
            updateAllCharts(data);
        } catch (err) {
            showError('API de métricas indisponível');
        }
    }, 10000);
}

function updateAllCharts(metrics) {
    drawDonutChart(getCanvas('donut'), metrics.by_status);
    drawGaugeChart(getCanvas('gauge'), metrics.success_rate);
    updateDLQCount(metrics.dlq_count);
    
    // Gerar dados mockados para bar chart baseado nos dados reais
    const buckets = generateHourlyBuckets(metrics);
    drawBarChart(getCanvas('bar'), buckets);
}
```

#### 2.10 SSE Connection
```javascript
function connectSSE() {
    const es = new EventSource('/api/events');
    
    es.addEventListener('payment', (e) => {
        state.sessionEventCount++;
        updateSessionCounter();
        // Flash effect no session counter
        // Refresh metrics imediatamente
        fetchMetrics().then(data => {
            state.metrics = data;
            updateAllCharts(data);
        }).catch(() => {});
    });
    
    es.addEventListener('heartbeat', () => {
        setConnected(true);
    });
    
    es.onopen = () => setConnected(true);
    es.onerror = () => { /* EventSource auto-reconnects */ };
    
    // Timeout para detectar heartbeat perdido
    let heartbeatTimeout;
    function resetHeartbeat() {
        clearTimeout(heartbeatTimeout);
        heartbeatTimeout = setTimeout(() => setConnected(false), 45000);
    }
}
```

#### 2.11 Geração de Buckets Horários
```javascript
function generateHourlyBuckets(metrics) {
    // Distribui total_processed pelas 24 horas
    // usando a distribuição de status como peso
    const buckets = new Array(24).fill(0);
    const total = metrics.total_processed || 0;
    if (total === 0) return buckets;
    
    // Distribuição uniforme com variação senoidal para simular
    // picos em horário comercial
    const now = new Date();
    const currentHour = now.getHours();
    
    for (let i = 0; i < 24; i++) {
        const hour = (currentHour - 23 + i + 24) % 24;
        // Distribuir com peso baseado em hora comercial (8-18h tem mais volume)
        const isBusinessHour = hour >= 8 && hour <= 18;
        const baseWeight = isBusinessHour ? 1.5 : 0.5;
        const randomFactor = 0.7 + Math.random() * 0.6;
        buckets[i] = Math.round((total / 24) * baseWeight * randomFactor);
    }
    
    return buckets;
}
```

---

### Etapa 3: Atualizar `server.go`

**Arquivo:** `internal/ui/server.go`

**Ações:**
1. Adicionar função `serveDashboardPage` (similar à `serveProducerPage` existente)
2. Registrar rota `GET /dashboard` no `mux` em `NewServer()`

```go
// serveDashboardPage serves the dashboard graphics HTML page.
func serveDashboardPage(w http.ResponseWriter, r *http.Request) {
    data, err := staticFiles.ReadFile("static/dashboard.html")
    if err != nil {
        http.Error(w, "dashboard page not found", http.StatusNotFound)
        return
    }
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    _, _ = w.Write(data)
}
```

Registrar em `NewServer()`:
```go
// Dashboard gráfica
mux.HandleFunc("GET /dashboard", serveDashboardPage)
```

---

### Etapa 4: Atualizar navegação no `index.html` e `producer.html`

**Arquivos:** `internal/ui/static/index.html`, `internal/ui/static/producer.html`

**Ações:**
1. Adicionar link `<a href="/dashboard" class="nav-link">📊 Gráficos</a>` no `<nav>` de cada página
2. Garantir que a classe `active` está apenas no link da página atual

---

### Etapa 5: Atualizar CSP no middleware

**Arquivo:** `internal/ui/server.go` — função `securityHeadersMiddleware()`

**Ação:** Verificar se a CSP atual (`connect-src 'self'`) já permite o polling e SSE. Como ambos são para a mesma origem (`/api/metrics` e `/api/events`), a CSP existente já é suficiente. Nenhuma alteração necessária.

---

### Etapa 6: Adicionar estilos CSS para os gráficos

**Local:** No próprio `dashboard.html` dentro de uma tag `<style>` no `<head>`, ou adicionar ao `style.css`.

**Recomendação:** Incluir os estilos específicos da dashboard gráfica em um bloco `<style>` no `<head>` do `dashboard.html` para não poluir o CSS compartilhado. Apenas reutilizar as variáveis CSS do `style.css`.

---

### Etapa 7: Testar integração

1. Servir a página e verificar se `/dashboard` carrega corretamente
2. Verificar se os gráficos renderizam com dados reais da API
3. Verificar se o polling de 10s funciona
4. Verificar se o SSE conecta e atualiza o contador
5. Verificar responsividade (redimensionar navegador)
6. Verificar comportamento offline (desligar API)

---

### Ordem de Implementação

| Ordem | Tarefa                        | Arquivo(s)                   | Depende de |
|-------|-------------------------------|------------------------------|------------|
| 1     | Criar `dashboard.html`        | `static/dashboard.html`      | —          |
| 2     | Adicionar rota Go             | `server.go`                  | 1          |
| 3     | Adicionar estilos CSS         | `dashboard.html` (<style>)   | 1          |
| 4     | Criar `dashboard.js` — estrutura| `static/dashboard.js`      | 1,3        |
| 5     | Implementar Donut Chart       | `dashboard.js`               | 4          |
| 6     | Implementar Gauge Chart       | `dashboard.js`               | 4          |
| 7     | Implementar Bar Chart         | `dashboard.js`               | 4          |
| 8     | Implementar Session Counter   | `dashboard.js`               | 4          |
| 9     | Implementar DLQ Counter       | `dashboard.js`               | 4          |
| 10    | Implementar polling + SSE     | `dashboard.js`               | 4          |
| 11    | Atualizar navegação           | `index.html`, `producer.html`| —          |
| 12    | Testes de integração          | —                            | 1-11       |

---

### Estimativa de Esforço

| Etapa          | Estimativa | Complexidade |
|----------------|------------|--------------|
| HTML + CSS     | 1h         | Baixa        |
| Rota Go        | 15min      | Muito Baixa  |
| Donut Chart    | 2h         | Média        |
| Gauge Chart    | 1.5h       | Média        |
| Bar Chart      | 2h         | Média        |
| Counter + DLQ  | 30min      | Baixa        |
| Polling + SSE  | 1h         | Baixa        |
| Navegação      | 15min      | Muito Baixa  |
| **Total**      | **~8.5h**  |              |
