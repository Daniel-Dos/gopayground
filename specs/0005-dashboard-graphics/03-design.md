# 03 — Design

## Arquitetura Geral

A Dashboard Gráfica é uma **página web estática** servida pelo mesmo servidor Go (`cmd/ui`), sem alterações no backend existente. Todo o processamento de gráficos ocorre no **cliente (browser)** via JavaScript vanilla com Canvas API.

```
Navegador                     Servidor Go (:8081)
─────────                     ────────────────
                             
GET /dashboard  ──────────►  serveDashboardPage()
  ◄─────────────  dashboard.html (HTML + CSS + JS inline/separado)
  
                   ◄──────── Polling GET /api/metrics (10s)
                   ◄──────── SSE  GET /api/events (conexão persistente)
```

### Decisões de Design

| Decisão                     | Opção Escolhida              | Alternativa Rejeitada           | Justificativa                                   |
|-----------------------------|------------------------------|--------------------------------|--------------------------------------------------|
| Biblioteca de gráficos      | Canvas API (vanilla JS)      | Chart.js, D3.js, ECharts       | Zero dependências, controle total, tamanho mínimo|
| Atualização de dados        | Polling 10s + SSE           | Apenas SSE                     | SSE pode perder dados; polling garante consistência |
| Dados de bar chart (24h)    | Gerados localmente a partir dos dados atuais | Nova API no backend | Evita depender de persistência histórica no servidor |
| Formato da página           | HTML + CSS + JS separados    | SPA com framework               | Segue padrão existente do projeto                |
| Gráfico para eventos tempo real | Contador numérico + flash visual | Gráfico de linha contínua | Dados efêmeros; contador é mais informativo |

---

## Estrutura de Rotas (Backend)

### Nova Rota

| Método | Path          | Handler               | Descrição                              |
|--------|---------------|-----------------------|----------------------------------------|
| GET    | `/dashboard`  | `serveDashboardPage`  | Serve o arquivo `static/dashboard.html` |

### Rota Existente (Reutilizada)

| Método | Path             | Handler               | Descrição                              |
|--------|------------------|-----------------------|----------------------------------------|
| GET    | `/api/metrics`   | `HandleMetrics`       | Retorna métricas agregadas em JSON     |
| GET    | `/api/events`    | `HandleSSE`           | Stream de eventos em tempo real (SSE)  |

### Implementação do Handler (server.go)

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

Registrar no `NewServer()`:

```go
mux.HandleFunc("GET /dashboard", serveDashboardPage)
```

---

## Layout da Página

```
┌─────────────────────────────────────────────────────────┐
│  HEADER                                                  │
│  ⬡ Payment Monitor        [📋 Dashboard] [📝 Producer]  │
│                            [📊 Gráficos]                  │ 🟢 Conectado
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────────┐ │
│  │  Donut Chart  │  │ Gauge Taxa   │  │ Eventos Sessão  │ │
│  │  (Status)     │  │ de Sucesso   │  │      ██        │ │
│  │               │  │    ○○○○○     │  │    42 eventos  │ │
│  │  ┌──────┐     │  │   71.4%     │  │  desde que abriu│ │
│  │  │canvas│     │  │  ○○○○○      │  │                │ │
│  │  └──────┘     │  └──────────────┘  │                │ │
│  │  Legenda      │                     │                │ │
│  └──────────────┘                     └────────────────┘ │
│                                                          │
│  ┌──────────────────────────────────────────────────┐    │
│  │  Bar Chart — Pagamentos por Hora (últimas 24h)   │    │
│  │  ██                                              │    │
│  │  ██ ██                                           │    │
│  │  ██ ██ ██                                        │    │
│  │  ██ ██ ██ ██  ██                                 │    │
│  │  └──────────────────────────────────────────     │    │
│  │  12  13  14  15  16  17  18  19  20  21  22  23  │    │
│  └──────────────────────────────────────────────────┘    │
│                                                          │
│  ┌──────────────────────────────────────────────────┐    │
│  │  DLQ Counter                                       │    │
│  │  2 mensagens na Dead Letter Queue                  │    │
│  │  [⚠️ Atenção: mensagens não processadas]           │    │
│  └──────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────┘
```

### Estrutura HTML (dashboard.html)

```html
<!DOCTYPE html>
<html lang="pt-BR">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Dashboard de Métricas — Payment Monitor</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@500;600;700;800&family=DM+Sans:wght@400;500;600&family=JetBrains+Mono:wght@400;500;600&display=swap" rel="stylesheet">
    <link rel="stylesheet" href="style.css">
    <link rel="icon" href="data:image/svg+xml,...">
</head>
<body>
    <div class="dashboard">

        <!-- HEADER -->
        <header>
            <div class="header-brand">
                <span class="brand-icon">⬡</span>
                <div class="brand-text">
                    <h1>Dashboard de Métricas</h1>
                    <span class="brand-subtitle">Processamento de Pagamentos em Tempo Real</span>
                </div>
            </div>
            <nav class="header-nav">
                <a href="/" class="nav-link">📋 Dashboard</a>
                <a href="/producer" class="nav-link">📝 Producer</a>
                <a href="/dashboard" class="nav-link active">📊 Gráficos</a>
            </nav>
            <div id="connection-status">🔴 Desconectado</div>
        </header>

        <!-- CHARTS GRID -->
        <div class="charts-grid">
            <!-- Donut Chart -->
            <div class="chart-card" id="chart-donut">
                <div class="chart-header">
                    <span class="section-head-accent"></span>
                    <h2>Distribuição por Status</h2>
                    <span class="section-head-badge">Tempo Real</span>
                </div>
                <div class="chart-body">
                    <canvas id="donut-canvas" role="img" aria-label="Gráfico de distribuição de pagamentos por status"></canvas>
                    <div id="donut-legend" class="chart-legend"></div>
                </div>
            </div>

            <!-- Gauge -->
            <div class="chart-card" id="chart-gauge">
                <div class="chart-header">
                    <span class="section-head-accent"></span>
                    <h2>Taxa de Sucesso</h2>
                </div>
                <div class="chart-body">
                    <canvas id="gauge-canvas" role="img" aria-label="Medidor de taxa de sucesso"></canvas>
                </div>
            </div>

            <!-- Session Counter -->
            <div class="chart-card" id="chart-counter">
                <div class="chart-header">
                    <span class="section-head-accent"></span>
                    <h2>Eventos na Sessão</h2>
                </div>
                <div class="chart-body counter-body">
                    <div class="session-counter" id="session-counter">0</div>
                    <div class="counter-label">eventos desde que abriu</div>
                </div>
            </div>
        </div>

        <!-- Bar Chart -->
        <div class="chart-card chart-full-width" id="chart-bar">
            <div class="chart-header">
                <span class="section-head-accent"></span>
                <h2>Pagamentos por Hora (24h)</h2>
            </div>
            <div class="chart-body">
                <canvas id="bar-canvas" role="img" aria-label="Gráfico de barras de pagamentos por hora nas últimas 24 horas"></canvas>
            </div>
        </div>

        <!-- DLQ Section -->
        <div class="chart-card chart-full-width" id="chart-dlq">
            <div class="chart-header">
                <span class="section-head-accent"></span>
                <h2>Dead Letter Queue</h2>
            </div>
            <div class="chart-body dlq-body">
                <div class="dlq-content">
                    <span class="dlq-icon">⚠️</span>
                    <span class="dlq-count" id="dlq-count">0</span>
                    <span class="dlq-label">mensagens não processadas</span>
                </div>
            </div>
        </div>

        <!-- Loading overlay -->
        <div id="loading-overlay" class="loading-overlay">
            <div class="loading-spinner"></div>
            <span>Carregando métricas...</span>
        </div>

    </div>

    <script src="dashboard.js"></script>
</body>
</html>
```

---

## Estrutura de Dados (JavaScript)

### Estado Global (dashboard.js)

```javascript
const state = {
    // Dados das métricas
    metrics: {
        total_processed: 0,
        by_status: { pending: 0, confirmed: 0, failed: 0, refunded: 0 },
        success_rate: 0,
        dlq_count: 0
    },
    // Contador de eventos da sessão
    sessionEventCount: 0,
    // Flag de conexão SSE
    isConnected: false,
    // Canvas references
    canvases: {
        donut: null,
        gauge: null,
        bar: null
    },
    // Dados mockados/derivados para bar chart (24 buckets)
    hourlyBuckets: new Array(24).fill(0),
};
```

### Formato da API /api/metrics

```json
{
    "total_processed": 150,
    "by_status": {
        "pending": 10,
        "confirmed": 100,
        "failed": 30,
        "refunded": 10
    },
    "success_rate": 71.43,
    "dlq_count": 2
}
```

### Transformação de Dados (Bar Chart)

Os dados do bar chart (24 buckets horários) são **derivados localmente** a partir dos dados disponíveis:

1. **Estratégia primária**: Se o `total_processed` for > 0, distribuir proporcionalmente pelos buckets com base na distribuição por status, adicionando ruído aleatório controlado para simular variação horária.
2. **Estratégia fallback**: Se não houver dados, exibir barras vazias com placeholder "Aguardando dados..."

> **Nota**: Em versão futura, o backend pode expor um endpoint `/api/metrics/historical` com dados reais por hora. Por enquanto, a visualização demonstra a capacidade de renderização e o layout.

---

## Design dos Gráficos (Canvas API)

### Donut Chart

```
Dimensões: 240x240 (em telas > 768px), responsivo via devicePixelRatio

Algoritmo:
1. Calcular total = sum(by_status)
2. Para cada status, calcular ângulo: (count / total) * 2π
3. Desenhar arcos com ctx.arc(), innerRadius=60, outerRadius=100
4. Aplicar cores do design system:
   - pending:   #fbbf24
   - confirmed: #34d399
   - failed:    #ef4444
   - refunded:  #60a5fa
5. Animação: começar com ângulo 0 e animar até o ângulo final (800ms ease-out)
6. Exibir total no centro do donut
```

### Gauge Semi-Circular

```
Dimensões: 240x180 (em telas > 768px)

Algoritmo:
1. Desenhar arco de fundo (0% a 100% — semi-círculo, ângulo de π a 2π)
2. Desenhar arco preenchido proporcional à taxa de sucesso
3. Cores baseadas em thresholds:
   - < 70%:  #ef4444 (vermelho)
   - 70-90%: #fbbf24 (amarelo)
   - > 90%:  #34d399 (verde)
4. Exibir percentual no centro do gauge com fonte grande (Plus Jakarta Sans 800)
5. Animação: preencher o arco com ease-out (800ms)
6. Tick marks em 0%, 25%, 50%, 75%, 100%
```

### Bar Chart

```
Dimensões: largura total do container, altura 280px

Algoritmo:
1. 24 barras igualmente espaçadas
2. Calcular altura máxima para escalonamento
3. Para cada bucket, desenhar retângulo com gradiente linear
4. Rótulos do eixo X a cada 3 horas (0:00, 3:00, 6:00, ...)
5. Rótulo do eixo Y com valores máximos
6. Cor: gradient accent (#7c5cfc → #a78bfa)
7. Borda inferior e linhas de grade sutis (#2d3140)
```

### Session Counter

```
Não usa Canvas — é um elemento DOM com transição CSS.
- Valor numérico grande (Plus Jakarta Sans 800, 3rem)
- Animação de "pulo" quando incrementa (keyframe scale 1→1.15→1)
- Card com gradiente accent sutil no fundo
```

---

## CSS Adicional (a adicionar no style.css ou dashboard.html <style>)

### Grid de Gráficos

```css
.charts-grid {
    display: grid;
    grid-template-columns: 1fr 1fr 1fr;
    gap: 16px;
    margin-bottom: 24px;
}

@media (max-width: 768px) {
    .charts-grid {
        grid-template-columns: 1fr;
    }
}

.chart-card {
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
    overflow: hidden;
    transition: border-color 0.2s var(--ease-out);
}

.chart-header {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 16px 20px 0;
    margin-bottom: 12px;
}

.chart-header h2 {
    font-family: 'Plus Jakarta Sans', sans-serif;
    font-size: 0.9rem;
    font-weight: 700;
    color: var(--color-text);
}

.chart-body {
    padding: 0 16px 16px;
    display: flex;
    flex-direction: column;
    align-items: center;
}

.chart-full-width {
    grid-column: 1 / -1;
}
```

### Session Counter & DLQ

```css
.session-counter {
    font-family: 'Plus Jakarta Sans', sans-serif;
    font-size: 3.5rem;
    font-weight: 800;
    color: var(--color-accent-light);
    line-height: 1;
    transition: transform 0.15s var(--ease-spring);
}

.counter-label {
    font-size: 0.82rem;
    color: var(--color-text-muted);
    margin-top: 4px;
}

.dlq-body {
    padding: 20px;
}

.dlq-content {
    display: flex;
    align-items: center;
    gap: 12px;
    font-size: 1.1rem;
}

.dlq-count {
    font-family: 'Plus Jakarta Sans', sans-serif;
    font-size: 2rem;
    font-weight: 800;
    color: var(--color-warning);
}

.dlq-label {
    color: var(--color-text-secondary);
}
```

---

## Integração CSS — Design System

Todas as cores utilizadas nos gráficos Canvas devem ser obtidas das variáveis CSS do `style.css` via `getComputedStyle()`:

```javascript
function getCSSVar(name) {
    return getComputedStyle(document.documentElement)
        .getPropertyValue(name).trim();
}

// Uso:
const accentColor = getCSSVar('--color-accent');         // #7c5cfc
const successColor = getCSSVar('--color-success');        // #34d399
const errorColor = getCSSVar('--color-error');            // #ef4444
const warningColor = getCSSVar('--color-warning');        // #fbbf24
const infoColor = getCSSVar('--color-info');              // #60a5fa
const textColor = getCSSVar('--color-text');              // #e8eaed
const mutedColor = getCSSVar('--color-text-muted');       // #6b7280
const borderColor = getCSSVar('--color-border');          // #2d3140
```

---

## Comunicação com Backend

### Polling (10s)

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
    } finally {
        clearTimeout(timeoutId);
    }
}

// Auto-refresh
let pollingInterval = setInterval(async () => {
    try {
        const data = await fetchMetrics();
        updateCharts(data);
        hideError();
    } catch (err) {
        showError('API de métricas indisponível');
    }
}, 10000);
```

### SSE

```javascript
function connectSSE() {
    const es = new EventSource('/api/events');

    es.addEventListener('payment', (e) => {
        state.sessionEventCount++;
        updateSessionCounter();
        // Trigger early metrics refresh
        refreshMetrics();
    });

    es.addEventListener('heartbeat', () => {
        updateConnectionStatus(true);
    });

    es.onerror = () => { /* EventSource auto-reconnects */ };
}
```

---

## Tratamento de Estados

### Loading

```
[Estado Inicial]
- Overlay "Carregando métricas..." sobre a área de gráficos
- Canvas não desenhado
- Polling iniciado imediatamente

[Dados Carregados]
- Overlay desaparece com fade
- Canvas desenhados com animação
- SSE inicia
```

### Error

```
[Erro na API]
- Gráficos mantêm último estado válido (se houver)
- Banner de erro sutil no topo da área de gráficos
- "API de métricas indisponível — tentando novamente em 10s"
- Novo polling continua ocorrendo

[Erro no SSE]
- Status alterado para "Desconectado"
- Polling continua normalmente (compensa ausência de SSE)
- Reconexão automática via EventSource
```

---

## Acessibilidade

| Elemento              | Atributo                 | Valor                                                        |
|-----------------------|--------------------------|--------------------------------------------------------------|
| Canvas Donut          | `role="img"`             | —                                                            |
| Canvas Donut          | `aria-label`             | "Gráfico de distribuição de pagamentos por status"           |
| Canvas Gauge          | `role="img"`             | —                                                            |
| Canvas Gauge          | `aria-label`             | "Medidor de taxa de sucesso: 71.4 por cento"                 |
| Canvas Bar            | `role="img"`             | —                                                            |
| Canvas Bar            | `aria-label`             | "Gráfico de barras de pagamentos por hora nas últimas 24 horas" |
| Botão fechar (se houver)| `aria-label`          | "Fechar"                                                     |
| Links de navegação    | Texto descritivo         | "Dashboard", "Producer", "Gráficos"                           |
| Status conexão        | `aria-live="polite"`     | —                                                            |

---

## Considerações de Performance

1. **Canvas resolution**: Usar `window.devicePixelRatio` para renderização nítida em displays Retina.
2. **Animation frames**: Usar `requestAnimationFrame` para animações suaves.
3. **Debounce polling**: Não acumular requisições se a resposta demorar mais que 10s.
4. **Canvas cleanup**: Limpar (`clearRect`) antes de redesenhar para evitar artefatos.
5. **Resize handling**: Redesenharlayout em `window resize` com debounce de 200ms.
