// Payment Monitor Dashboard — Graphics
// Canvas API charts with no external dependencies.
// Hardening: AbortController, retry/backoff, isPolling flag, schema validation,
//            debounce SSE refresh (2s) + resize (200ms), animation frame cancel.

(function () {
    'use strict';

    /* ================================================================
       STATE
       ================================================================ */
    var state = {
        metrics: null,
        sessionEventCount: 0,
        isConnected: false,
        hasData: false
    };

    /* ================================================================
       DOM REFERENCES
       ================================================================ */
    var connectionStatus = document.getElementById('connection-status');
    var errorBanner = document.getElementById('error-banner');
    var errorMessage = document.getElementById('error-message');
    var errorRetry = document.getElementById('error-retry');
    var loadingOverlay = document.getElementById('loading-overlay');
    var donutCanvas = document.getElementById('donut-canvas');
    var gaugeCanvas = document.getElementById('gauge-canvas');
    var barCanvas = document.getElementById('bar-canvas');
    var sessionCounterEl = document.getElementById('session-counter');
    var dlqCountEl = document.getElementById('dlq-count');
    var donutLegend = document.getElementById('donut-legend');
    var barTooltip = document.getElementById('bar-tooltip');
    var ttLabel = document.getElementById('tt-label');
    var ttValue = document.getElementById('tt-value');

    /* ================================================================
       UTILITIES
       ================================================================ */

    /** Read a CSS custom property from :root */
    function getCSSVar(name) {
        return getComputedStyle(document.documentElement)
            .getPropertyValue(name).trim();
    }

    /** Setup canvas for Retina displays */
    function setupCanvas(canvas, logicalWidth, logicalHeight) {
        var dpr = window.devicePixelRatio || 1;
        // Cap at 2x for performance on high-DPI screens
        if (dpr > 2 && navigator.deviceMemory && navigator.deviceMemory < 4) {
            dpr = 2;
        }
        canvas.width = logicalWidth * dpr;
        canvas.height = logicalHeight * dpr;
        canvas.style.width = logicalWidth + 'px';
        canvas.style.height = logicalHeight + 'px';
        var ctx = canvas.getContext('2d');
        ctx.scale(dpr, dpr);
        return ctx;
    }

    /** Get canvas context with proper DPI scaling.
     *  Uses setTransform to enforce consistent DPR scaling,
     *  regardless of whether setupCanvas has been called.
     */
    function getCtx(canvas) {
        if (!canvas) return null;
        var ctx = canvas.getContext('2d');
        var dpr = window.devicePixelRatio || 1;
        if (dpr > 2 && navigator.deviceMemory && navigator.deviceMemory < 4) {
            dpr = 2;
        }
        // Apply DPR transform — this is idempotent because setTransform
        // replaces any previous transform. Safe to call even if setupCanvas
        // already applied scale via ctx.scale().
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        return ctx;
    }

    /** CSS color to rgba for Canvas compositing */
    function hexToRgba(hex, alpha) {
        if (alpha === undefined) alpha = 1;
        var r = parseInt(hex.slice(1, 3), 16);
        var g = parseInt(hex.slice(3, 5), 16);
        var b = parseInt(hex.slice(5, 7), 16);
        return 'rgba(' + r + ',' + g + ',' + b + ',' + alpha + ')';
    }

    /** Check if user prefers reduced motion */
    function prefersReducedMotion() {
        return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    }

    /** Animation frame IDs per chart — prevents one chart cancelling another */
    var animFrames = {
        donut: null,
        gauge: null
    };

    function cancelAnimation(animRef) {
        var key = animRef || null;
        if (key) {
            // Cancel a specific animation by key
            if (animFrames[key]) {
                cancelAnimationFrame(animFrames[key]);
                animFrames[key] = null;
            }
            return;
        }
        // Cancel all animations if no specific key given
        for (var k in animFrames) {
            if (animFrames[k]) {
                cancelAnimationFrame(animFrames[k]);
                animFrames[k] = null;
            }
        }
    }

    /** Animate a chart render function over duration
     *  @param {string} animKey — key in animFrames (e.g. 'donut', 'gauge')
     */
    function animateChart(renderFn, duration, onComplete, animKey) {
        cancelAnimation(animKey);
        if (prefersReducedMotion()) {
            console.debug('[Anim] reduced motion — render immediate para', animKey);
            renderFn(1);
            if (onComplete) onComplete();
            return;
        }
        var start = performance.now();
        var self = this;
        function frame(timestamp) {
            var progress = Math.min((timestamp - start) / duration, 1);
            // Ease-out cubic
            var eased = 1 - Math.pow(1 - progress, 3);
            renderFn(eased);
            if (progress < 1) {
                animFrames[animKey] = requestAnimationFrame(frame);
            } else {
                console.debug('[Anim] animação completada para', animKey);
                animFrames[animKey] = null;
                if (onComplete) onComplete();
            }
        }
        console.debug('[Anim] iniciando animação para', animKey);
        animFrames[animKey] = requestAnimationFrame(frame);
    }

    /* ================================================================
       COLOR MAP — Status to CSS variable
       ================================================================ */
    var STATUS_COLORS = {
        pending:   getCSSVar('--color-warning'),   // #fbbf24
        confirmed: getCSSVar('--color-success'),   // #34d399
        failed:    getCSSVar('--color-error'),     // #ef4444
        refunded:  getCSSVar('--color-info')       // #60a5fa
    };

    var STATUS_LABELS = {
        pending:   'Pendentes',
        confirmed: 'Confirmados',
        failed:    'Falhas',
        refunded:  'Reembolsados'
    };

    /* ================================================================
       VALIDATION
       ================================================================ */

    function validateMetrics(data) {
        if (!data || typeof data !== 'object') return false;
        if (typeof data.total_processed !== 'number') return false;
        if (!data.by_status || typeof data.by_status !== 'object') return false;
        if (typeof data.success_rate !== 'number') return false;
        if (typeof data.dlq_count !== 'number') return false;
        return true;
    }

    /* ================================================================
       FETCH WITH TIMEOUT + ABORTCONTROLLER
       ================================================================ */

    async function fetchMetrics() {
        var controller = new AbortController();
        var timeoutId = setTimeout(function () { controller.abort(); }, 5000);

        try {
            var resp = await fetch('/api/metrics', {
                signal: controller.signal
            });
            if (!resp.ok) throw new Error('HTTP ' + resp.status);
            var data = await resp.json();

            if (!validateMetrics(data)) {
                console.error('[Dashboard] Dados de métricas inválidos:', data);
                throw new Error('Formato de dados inesperado');
            }

            console.info('[Dashboard] Métricas carregadas:', {
                total: data.total_processed,
                successRate: data.success_rate,
                dlq: data.dlq_count
            });
            return data;
        } catch (err) {
            if (err.name === 'AbortError') {
                console.warn('[Dashboard] Métricas: requisição abortada por timeout');
                throw new Error('Timeout de 5s');
            }
            throw err;
        } finally {
            clearTimeout(timeoutId);
        }
    }

    /* ================================================================
       RETRY WITH BACKOFF — Initial Load
       ================================================================ */

    async function loadInitialMetrics() {
        var retries = 3;
        for (var i = 0; i < retries; i++) {
            try {
                var data = await fetchMetrics();
                state.metrics = data;
                state.hasData = true;
                hideLoading();
                hideError();
                updateAllCharts(data);
                console.info('[Dashboard] Carga inicial concluída');
                return;
            } catch (err) {
                console.warn('[Dashboard] Tentativa ' + (i + 1) + '/' + retries + ' falhou:', err.message);
                if (i === retries - 1) {
                    showError('API de métricas indisponível após múltiplas tentativas');
                    hideLoading();
                    return;
                }
                // Backoff exponencial: 1s, 2s, 4s
                await new Promise(function (r) { return setTimeout(r, 1000 * Math.pow(2, i)); });
            }
        }
    }

    /* ================================================================
       POLLING — Every 10s with isPolling flag
       ================================================================ */

    var isPolling = false;

    async function pollMetrics() {
        if (isPolling) return;
        isPolling = true;
        try {
            var data = await fetchMetrics();
            state.metrics = data;
            state.hasData = true;
            hideError();
            console.debug('[Poll] Novos dados recebidos, chamando updateAllCharts. by_status:',
                JSON.stringify(data.by_status));
            updateAllCharts(data);
        } catch (err) {
            console.warn('[Dashboard] API de métricas indisponível:', err.message);
            showError('API de métricas indisponível — tentando novamente em 10s');
            // Keep last valid state displayed
        } finally {
            isPolling = false;
        }
    }

    function startPolling() {
        setInterval(pollMetrics, 10000);
    }

    /* ================================================================
       SSE CONNECTION
       ================================================================ */

    var eventSource = null;
    var heartbeatTimeoutId = null;
    var sseRefreshTimeout = null;

    function setConnected(connected) {
        state.isConnected = connected;
        connectionStatus.textContent = connected ? '\uD83D\uDFE2 Conectado' : '\uD83D\uDD34 Desconectado';
        connectionStatus.className = connected ? 'connected' : 'disconnected';
    }

    function resetHeartbeatTimeout() {
        if (heartbeatTimeoutId) clearTimeout(heartbeatTimeoutId);
        heartbeatTimeoutId = setTimeout(function () {
            setConnected(false);
            console.warn('[Dashboard] SSE heartbeat perdido');
        }, 45000);
    }

    function connectSSE() {
        if (eventSource) {
            eventSource.close();
        }

        eventSource = new EventSource('/api/events');

        eventSource.addEventListener('payment', function (e) {
            try {
                var eventData = JSON.parse(e.data);
                state.sessionEventCount++;
                updateSessionCounter();

                // Debounce SSE refresh: wait 2s after last event
                // Resets the timer on each new event so we always poll
                // 2 seconds after the most recent event.
                if (sseRefreshTimeout) clearTimeout(sseRefreshTimeout);
                sseRefreshTimeout = setTimeout(function () {
                    sseRefreshTimeout = null;
                    pollMetrics();
                }, 2000);

                console.debug('[Dashboard] Evento SSE recebido:', eventData.payment_id);
            } catch (err) {
                console.error('[Dashboard] Falha ao processar evento SSE:', err);
            }
        });

        eventSource.addEventListener('heartbeat', function () {
            setConnected(true);
            resetHeartbeatTimeout();
        });

        eventSource.onopen = function () {
            setConnected(true);
            resetHeartbeatTimeout();
            console.info('[Dashboard] SSE conectado');
        };

        eventSource.onerror = function () {
            // EventSource auto-reconnects; heartbeat timeout handles stale connections
            console.warn('[Dashboard] SSE desconectado (reconexão automática)');
        };

        resetHeartbeatTimeout();
    }

    /* ================================================================
       SESSION COUNTER (DOM)
       ================================================================ */

    function updateSessionCounter() {
        sessionCounterEl.textContent = state.sessionEventCount;
        // Bounce animation
        sessionCounterEl.classList.remove('bounce');
        // Force reflow for animation restart
        void sessionCounterEl.offsetWidth;
        sessionCounterEl.classList.add('bounce');
    }

    /* ================================================================
       DLQ COUNTER
       ================================================================ */

    function updateDLQCount(count) {
        dlqCountEl.textContent = count;
        if (count > 0) {
            dlqCountEl.className = 'dlq-count';
        } else {
            dlqCountEl.className = 'dlq-count zero';
        }
    }

    /* ================================================================
       HOURLY BUCKETS GENERATION (synthetic)
       ================================================================ */

    function generateHourlyBuckets(metrics) {
        var buckets = new Array(24).fill(0);
        var total = metrics.total_processed || 0;
        if (total === 0) return buckets;

        var now = new Date();
        var currentHour = now.getHours();

        for (var i = 0; i < 24; i++) {
            var hour = (currentHour - 23 + i + 24) % 24;
            var isBusinessHour = hour >= 8 && hour <= 18;
            var baseWeight = isBusinessHour ? 1.5 : 0.5;
            var randomFactor = 0.7 + Math.random() * 0.6;
            buckets[i] = Math.round((total / 24) * baseWeight * randomFactor);
        }

        return buckets;
    }

    /* ================================================================
       CHART: DONUT
       ================================================================ */

    var donutChartWidth = 240;
    var donutChartHeight = 240;

    function drawDonutChart(canvas, byStatus) {
        if (!canvas) return;

        console.debug('[Donut] drawDonutChart chamado com byStatus:', JSON.stringify(byStatus));

        var ctx = getCtx(canvas);
        var dpr = window.devicePixelRatio || 1;
        if (dpr > 2 && navigator.deviceMemory && navigator.deviceMemory < 4) {
            dpr = 2;
        }

        var total = 0;
        var statuses = ['pending', 'confirmed', 'failed', 'refunded'];
        for (var s = 0; s < statuses.length; s++) {
            total += (byStatus[statuses[s]] || 0);
        }

        console.debug('[Donut] total calculado:', total);

        if (total === 0) {
            // Draw empty donut
            ctx.clearRect(0, 0, donutChartWidth, donutChartHeight);
            var cx = donutChartWidth / 2;
            var cy = donutChartHeight / 2;
            var outerR = 90;
            var innerR = 55;

            ctx.beginPath();
            ctx.arc(cx, cy, outerR, 0, Math.PI * 2);
            ctx.arc(cx, cy, innerR, 0, Math.PI * 2, true);
            ctx.closePath();
            ctx.fillStyle = hexToRgba(getCSSVar('--color-border'), 0.3);
            ctx.fill();

            ctx.fillStyle = getCSSVar('--color-text-muted');
            ctx.font = '600 16px "Plus Jakarta Sans", sans-serif';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            ctx.fillText('Sem dados', cx, cy);

            donutLegend.innerHTML = '<span class="legend-item" style="color:' + getCSSVar('--color-text-muted') + '">Nenhum pagamento registrado</span>';
            return;
        }

        // Build segment data
        var segments = [];
        for (var si = 0; si < statuses.length; si++) {
            var key = statuses[si];
            var val = byStatus[key] || 0;
            if (val > 0) {
                segments.push({
                    key: key,
                    value: val,
                    color: STATUS_COLORS[key] || getCSSVar('--color-text-muted'),
                    label: STATUS_LABELS[key] || key
                });
            }
        }

        // Sort largest first for visual clarity
        segments.sort(function (a, b) { return b.value - a.value; });

        var segTotal = segments.reduce(function (acc, seg) { return acc + seg.value; }, 0);

        var cx2 = donutChartWidth / 2;
        var cy2 = donutChartHeight / 2;
        var outerR2 = 90;
        var innerR2 = 55;

        // Calculate total arc angles per segment
        var startAngle = -Math.PI / 2;
        var segAngles = [];
        for (var sj = 0; sj < segments.length; sj++) {
            var angle = (segments[sj].value / segTotal) * Math.PI * 2;
            segAngles.push({
                start: startAngle,
                end: startAngle + angle,
                color: segments[sj].color,
                key: segments[sj].key,
                value: segments[sj].value,
                label: segments[sj].label
            });
            startAngle += angle;
        }

        // Clear canvas immediately (synchronous) before starting animation.
        // This ensures stale pixels from previous renders are removed even
        // if the animation callback is delayed or cancelled.
        ctx.clearRect(0, 0, donutChartWidth, donutChartHeight);

        // Animate drawing
        function renderDonut(progress) {
            ctx.clearRect(0, 0, donutChartWidth, donutChartHeight);

            for (var si2 = 0; si2 < segAngles.length; si2++) {
                var seg = segAngles[si2];
                var arcEnd = seg.start + (seg.end - seg.start) * progress;

                ctx.beginPath();
                ctx.arc(cx2, cy2, outerR2, seg.start, arcEnd);
                ctx.arc(cx2, cy2, innerR2, arcEnd, seg.start, true);
                ctx.closePath();
                ctx.fillStyle = seg.color;
                ctx.fill();

                // Subtle stroke between segments
                ctx.strokeStyle = getCSSVar('--color-bg');
                ctx.lineWidth = 2;
                ctx.stroke();
            }

            // Center total text
            if (progress >= 0.9) {
                ctx.fillStyle = getCSSVar('--color-text');
                ctx.font = '800 28px "Plus Jakarta Sans", sans-serif';
                ctx.textAlign = 'center';
                ctx.textBaseline = 'middle';
                ctx.fillText(String(segTotal), cx2, cy2 - 8);

                ctx.fillStyle = getCSSVar('--color-text-muted');
                ctx.font = '500 11px "DM Sans", sans-serif';
                ctx.fillText('Total', cx2, cy2 + 18);
            }
        }

        animateChart(renderDonut, 800, null, 'donut');

        // Update legend
        var legendHTML = '';
        for (var sl = 0; sl < segments.length; sl++) {
            var seg2 = segments[sl];
            var pct = ((seg2.value / segTotal) * 100).toFixed(1);
            legendHTML +=
                '<span class="legend-item">' +
                '<span class="legend-dot" style="background:' + seg2.color + '"></span>' +
                seg2.label + ': ' +
                '<span class="legend-value">' + seg2.value + '</span>' +
                ' (' + pct + '%)' +
                '</span>';
        }
        donutLegend.innerHTML = legendHTML;

        // Update aria-label
        var ariaParts = ['Gráfico de distribuição de pagamentos por status. Total: ' + segTotal + '. '];
        for (var sa = 0; sa < segments.length; sa++) {
            var seg3 = segments[sa];
            var pct2 = ((seg3.value / segTotal) * 100).toFixed(1);
            ariaParts.push(seg3.label + ': ' + seg3.value + ' (' + pct2 + '%). ');
        }
        canvas.setAttribute('aria-label', ariaParts.join(''));
    }

    function drawEmptyDonut(canvas) {
        if (!canvas) return;
        var ctx = getCtx(canvas);
        var cx = donutChartWidth / 2;
        var cy = donutChartHeight / 2;
        ctx.clearRect(0, 0, donutChartWidth, donutChartHeight);

        ctx.beginPath();
        ctx.arc(cx, cy, 90, 0, Math.PI * 2);
        ctx.arc(cx, cy, 55, 0, Math.PI * 2, true);
        ctx.closePath();
        ctx.fillStyle = hexToRgba(getCSSVar('--color-border'), 0.3);
        ctx.fill();

        ctx.fillStyle = getCSSVar('--color-text-muted');
        ctx.font = '600 14px "Plus Jakarta Sans", sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText('Sem dados', cx, cy);
    }

    /* ================================================================
       CHART: GAUGE (Semi-Circular)
       ================================================================ */

    var gaugeChartWidth = 240;
    var gaugeChartHeight = 200;

    function drawGaugeChart(canvas, rate) {
        if (!canvas) return;

        var ctx = getCtx(canvas);
        ctx.clearRect(0, 0, gaugeChartWidth, gaugeChartHeight);

        var cx = gaugeChartWidth / 2;
        var cy = gaugeChartHeight * 0.75;
        var radius = 85;
        var lineWidth = 18;

        // Color based on threshold
        var gaugeColor;
        if (rate < 70) {
            gaugeColor = getCSSVar('--color-error');
        } else if (rate < 90) {
            gaugeColor = getCSSVar('--color-warning');
        } else {
            gaugeColor = getCSSVar('--color-success');
        }

        var bgColor = hexToRgba(getCSSVar('--color-border'), 0.4);
        var textColor = getCSSVar('--color-text');
        var mutedColor = getCSSVar('--color-text-muted');

        // Draw background arc (full semi-circle)
        ctx.beginPath();
        ctx.arc(cx, cy, radius, Math.PI, 2 * Math.PI);
        ctx.strokeStyle = bgColor;
        ctx.lineWidth = lineWidth;
        ctx.lineCap = 'round';
        ctx.stroke();

        // Draw tick marks
        var tickPositions = [0, 25, 50, 75, 100];
        ctx.fillStyle = mutedColor;
        ctx.font = '500 10px "DM Sans", sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'top';

        for (var t = 0; t < tickPositions.length; t++) {
            var pct = tickPositions[t] / 100;
            var angle = Math.PI + pct * Math.PI;
            var tx = cx + (radius + 4) * Math.cos(angle);
            var ty = cy + (radius + 4) * Math.sin(angle);
            ctx.fillText(tickPositions[t] + '%', tx, ty + 4);
        }

    // Animate filled arc
        function renderGauge(progress) {
            // Clear and redraw each frame
            ctx.clearRect(0, 0, gaugeChartWidth, gaugeChartHeight);

            // Background arc
            ctx.beginPath();
            ctx.arc(cx, cy, radius, Math.PI, 2 * Math.PI);
            ctx.strokeStyle = bgColor;
            ctx.lineWidth = lineWidth;
            ctx.lineCap = 'round';
            ctx.stroke();

            // Filled portion
            var fillAngle = Math.PI + (rate / 100) * Math.PI * progress;
            ctx.beginPath();
            ctx.arc(cx, cy, radius, Math.PI, fillAngle);
            ctx.strokeStyle = gaugeColor;
            ctx.lineWidth = lineWidth;
            ctx.lineCap = 'round';
            ctx.stroke();

            // Draw tick marks again (in case of progressive clear)
            for (var t2 = 0; t2 < tickPositions.length; t2++) {
                var pct2 = tickPositions[t2] / 100;
                var angle2 = Math.PI + pct2 * Math.PI;
                var tx2 = cx + (radius + 4) * Math.cos(angle2);
                var ty2 = cy + (radius + 4) * Math.sin(angle2);
                ctx.fillStyle = mutedColor;
                ctx.font = '500 10px "DM Sans", sans-serif';
                ctx.textAlign = 'center';
                ctx.textBaseline = 'top';
                ctx.fillText(tickPositions[t2] + '%', tx2, ty2 + 4);
            }

            // Center percentage
            if (progress >= 0.9 || progress === 1) {
                ctx.fillStyle = gaugeColor;
                ctx.font = '800 36px "Plus Jakarta Sans", sans-serif';
                ctx.textAlign = 'center';
                ctx.textBaseline = 'middle';
                ctx.fillText(rate.toFixed(1) + '%', cx, cy - 12);

                ctx.fillStyle = mutedColor;
                ctx.font = '500 11px "DM Sans", sans-serif';
                ctx.fillText('Taxa de Sucesso', cx, cy + 30);
            }
        }

        animateChart(renderGauge, 800, null, 'gauge');

        // Update aria-label
        canvas.setAttribute('aria-label', 'Medidor de taxa de sucesso: ' + rate.toFixed(1) + ' por cento');
    }

    /* ================================================================
       CHART: BAR CHART (24h)
       ================================================================ */

    var barChartWidth = 800;
    var barChartHeight = 280;

    // Will be set dynamically
    var currentBarBuckets = [];

    function drawBarChart(canvas, buckets) {
        if (!canvas) return;

        currentBarBuckets = buckets;
        var ctx = getCtx(canvas);

        var width = canvas.clientWidth || barChartWidth;
        var height = barChartHeight;

        ctx.clearRect(0, 0, width, height);

        var hasData = buckets.some(function (v) { return v > 0; });

        if (!hasData) {
            ctx.fillStyle = getCSSVar('--color-text-muted');
            ctx.font = '500 16px "DM Sans", sans-serif';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            ctx.fillText('Aguardando dados históricos...', width / 2, height / 2);
            return;
        }

        var padding = { top: 20, right: 20, bottom: 40, left: 50 };
        var chartW = width - padding.left - padding.right;
        var chartH = height - padding.top - padding.bottom;
        var barCount = 24;
        var barWidth = (chartW / barCount) * 0.7;
        var barGap = (chartW / barCount) * 0.3;

        var maxVal = Math.max.apply(null, buckets);
        if (maxVal === 0) maxVal = 1;

        // Colors
        var accentColor = getCSSVar('--color-accent');
        var accentLightColor = getCSSVar('--color-accent-light');
        var borderColor = getCSSVar('--color-border');
        var textColor = getCSSVar('--color-text');
        var mutedColor2 = getCSSVar('--color-text-muted');

        // Draw horizontal grid lines
        var gridLines = 4;
        ctx.strokeStyle = hexToRgba(borderColor, 0.5);
        ctx.lineWidth = 1;
        for (var g = 0; g <= gridLines; g++) {
            var y2 = padding.top + (chartH / gridLines) * g;
            ctx.beginPath();
            ctx.moveTo(padding.left, y2);
            ctx.lineTo(width - padding.right, y2);
            ctx.stroke();

            // Y-axis labels
            var val = Math.round(maxVal - (maxVal / gridLines) * g);
            ctx.fillStyle = mutedColor2;
            ctx.font = '500 10px "DM Sans", sans-serif';
            ctx.textAlign = 'right';
            ctx.textBaseline = 'middle';
            ctx.fillText(String(val), padding.left - 8, y2);
        }

        // Draw bars
        var now = new Date();
        var currentHour = now.getHours();

        for (var b = 0; b < barCount; b++) {
            var barH = (buckets[b] / maxVal) * chartH;
            var x = padding.left + (chartW / barCount) * b + barGap / 2;
            var y3 = padding.top + chartH - barH;

            // Linear gradient for each bar
            var grad = ctx.createLinearGradient(x, y3, x, padding.top + chartH);
            grad.addColorStop(0, accentColor);
            grad.addColorStop(1, accentLightColor);

            ctx.fillStyle = grad;
            // Rounded top corners
            var r = Math.min(barWidth / 2, 3);
            ctx.beginPath();
            ctx.moveTo(x, padding.top + chartH);
            ctx.lineTo(x, y3 + r);
            ctx.quadraticCurveTo(x, y3, x + r, y3);
            ctx.lineTo(x + barWidth - r, y3);
            ctx.quadraticCurveTo(x + barWidth, y3, x + barWidth, y3 + r);
            ctx.lineTo(x + barWidth, padding.top + chartH);
            ctx.closePath();
            ctx.fill();
        }

        // X-axis labels (every 3 hours)
        ctx.fillStyle = mutedColor2;
        ctx.font = '500 10px "DM Sans", sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'top';

        for (var l = 0; l < barCount; l += 3) {
            var hour = (currentHour - 23 + l + 24) % 24;
            var hourLabel = String(hour).padStart(2, '0') + ':00';
            var xl = padding.left + (chartW / barCount) * l + (chartW / barCount) / 2;
            ctx.fillText(hourLabel, xl, padding.top + chartH + 10);
        }

        // Y-axis label
        ctx.save();
        ctx.translate(12, padding.top + chartH / 2);
        ctx.rotate(-Math.PI / 2);
        ctx.fillStyle = mutedColor2;
        ctx.font = '500 10px "DM Sans", sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText('Pagamentos', 0, 0);
        ctx.restore();
    }

    /* ================================================================
       BAR CHART HOVER TOOLTIP
       ================================================================ */

    function setupBarTooltip() {
        if (!barCanvas) return;

        barCanvas.addEventListener('mousemove', function (e) {
            var rect = barCanvas.getBoundingClientRect();
            var mouseX = e.clientX - rect.left;
            var mouseY = e.clientY - rect.top;

            var width = barCanvas.clientWidth || barChartWidth;
            var height = barChartHeight;
            var padding = { top: 20, right: 20, bottom: 40, left: 50 };
            var chartW = width - padding.left - padding.right;
            var barCount = 24;

            if (mouseX < padding.left || mouseX > width - padding.right ||
                mouseY < padding.top || mouseY > height - padding.bottom) {
                barTooltip.classList.remove('visible');
                return;
            }

            var barIndex = Math.floor(((mouseX - padding.left) / chartW) * barCount);
            if (barIndex < 0 || barIndex >= barCount) {
                barTooltip.classList.remove('visible');
                return;
            }

            // Calculate the hour
            var now = new Date();
            var currentHour = now.getHours();
            var hour = (currentHour - 23 + barIndex + 24) % 24;
            var hourLabel = String(hour).padStart(2, '0') + ':00';
            var value = currentBarBuckets[barIndex] || 0;

            ttLabel.textContent = hourLabel;
            ttValue.textContent = value + ' pagamentos';

            barTooltip.style.left = (e.clientX + 14) + 'px';
            barTooltip.style.top = (e.clientY - 10) + 'px';
            barTooltip.classList.add('visible');
        });

        barCanvas.addEventListener('mouseleave', function () {
            barTooltip.classList.remove('visible');
        });
    }

    /* ================================================================
       UPDATE ALL CHARTS
       ================================================================ */

    function updateAllCharts(metrics) {
        // Donut
        drawDonutChart(donutCanvas, metrics.by_status);

        // Gauge
        drawGaugeChart(gaugeCanvas, metrics.success_rate);

        // DLQ
        updateDLQCount(metrics.dlq_count);

        // Bar chart (synthetic data)
        var buckets = generateHourlyBuckets(metrics);
        drawBarChart(barCanvas, buckets);
    }

    /* ================================================================
       ERROR / LOADING UI
       ================================================================ */

    function showError(msg) {
        errorMessage.textContent = msg || 'API de métricas indisponível';
        errorBanner.classList.add('visible');
        errorRetry.textContent = new Date().toLocaleTimeString('pt-BR');
    }

    function hideError() {
        errorBanner.classList.remove('visible');
    }

    function hideLoading() {
        loadingOverlay.classList.add('hidden');
    }

    /* ================================================================
       RESIZE HANDLER (debounced 200ms)
       ================================================================ */

    var resizeTimeout = null;

    function handleResize() {
        if (resizeTimeout) clearTimeout(resizeTimeout);
        resizeTimeout = setTimeout(function () {
            // Re-setup bar canvas for new container width
            resizeBarCanvas();
            if (state.metrics) {
                updateAllCharts(state.metrics);
            }
        }, 200);
    }

    /* ================================================================
       CANVAS INIT SETUP
       ================================================================ */

    function setupCanvases() {
        // Donut: fixed size
        setupCanvas(donutCanvas, donutChartWidth, donutChartHeight);

        // Gauge: fixed size
        setupCanvas(gaugeCanvas, gaugeChartWidth, gaugeChartHeight);

        // Bar: dynamic width based on container
        resizeBarCanvas();
    }

    function resizeBarCanvas() {
        if (!barCanvas) return;
        var container = barCanvas.parentElement;
        var containerWidth = container.clientWidth - 32; // account for chart-body padding
        if (containerWidth < 300) containerWidth = 300;
        setupCanvas(barCanvas, containerWidth, barChartHeight);
    }

    /* ================================================================
       INIT
       ================================================================ */

    function checkCanvasSupport() {
        var canvases = [donutCanvas, gaugeCanvas, barCanvas];
        for (var i = 0; i < canvases.length; i++) {
            if (!canvases[i] || !canvases[i].getContext) {
                var container = document.getElementById('charts-grid');
                if (container) {
                    container.innerHTML =
                        '<div class="chart-card chart-full-width" style="padding:40px;text-align:center;">' +
                        '<p style="color:' + getCSSVar('--color-error') + ';font-size:1rem;font-weight:600;">' +
                        'Navegador não suporta Canvas</p>' +
                        '<p style="color:' + getCSSVar('--color-text-muted') + ';margin-top:8px;">' +
                        'Atualize seu navegador para visualizar os gráficos.</p></div>';
                }
                return false;
            }
        }
        return true;
    }

    function init() {
        console.info('[Dashboard] Inicializando...');

        if (!checkCanvasSupport()) return;

        setupCanvases();
        setupBarTooltip();

        // Show initial empty charts
        drawEmptyDonut(donutCanvas);

        // Connect SSE
        connectSSE();

        // Load initial data with retry
        loadInitialMetrics();

        // Start polling
        startPolling();

        // Resize handler
        window.addEventListener('resize', handleResize);

        // Cleanup on page unload
        window.addEventListener('beforeunload', function () {
            if (eventSource) eventSource.close();
            if (resizeTimeout) clearTimeout(resizeTimeout);
            if (heartbeatTimeoutId) clearTimeout(heartbeatTimeoutId);
            if (sseRefreshTimeout) clearTimeout(sseRefreshTimeout);
            cancelAnimation();
        });
    }

    // Start when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

})();
