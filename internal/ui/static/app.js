// Payment Consumer UI - Application Logic

(function () {
    'use strict';

    // --- State ---
    let allPayments = [];
    let currentHistoryPaymentId = null;

    // --- DOM References ---
    const connectionStatus = document.getElementById('connection-status');
    const feedContainer = document.getElementById('feed-container');
    const paymentsTbody = document.querySelector('#payments-table tbody');
    const metricsSection = document.getElementById('metrics');
    const filterPaymentId = document.getElementById('filter-payment-id');
    const filterStatus = document.getElementById('filter-status');
    const historyModal = document.getElementById('history-modal');
    const modalPaymentId = document.getElementById('modal-payment-id');
    const historyTbody = document.querySelector('#history-table tbody');
    const closeModal = document.querySelector('.close');

    // --- State ---
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

    // --- SSE Connection ---
    let eventSource = null;
    let isConnected = false;

    function updateConnectionStatus(connected) {
        isConnected = connected;
        connectionStatus.textContent = connected ? '🟢 Conectado' : '🔴 Desconectado';
        connectionStatus.className = connected ? 'connected' : 'disconnected';
    }

    function connectSSE() {
        if (eventSource) {
            eventSource.close();
        }

        eventSource = new EventSource('/api/events');
        var heartbeatTimeout = null;

        function resetHeartbeatTimeout() {
            if (heartbeatTimeout) clearTimeout(heartbeatTimeout);
            heartbeatTimeout = setTimeout(function () {
                updateConnectionStatus(false);
            }, 45000);
        }

        eventSource.addEventListener('payment', function (e) {
            try {
                var event = JSON.parse(e.data);
                addToFeed(event);
                updatePayment(event);
                refreshMetrics();
            } catch (err) {
                console.error('Falha ao processar evento SSE:', err);
            }
        });

        eventSource.addEventListener('heartbeat', function () {
            updateConnectionStatus(true);
            resetHeartbeatTimeout();
        });

        eventSource.onopen = function () {
            updateConnectionStatus(true);
            resetHeartbeatTimeout();
        };

        eventSource.onerror = function () {
            // Don't set disconnected immediately — EventSource auto-reconnects
            // The heartbeat timeout will handle stale connections
        };

        // Initial heartbeat timeout
        resetHeartbeatTimeout();
    }

    // --- Retry with Backoff ---
    async function fetchWithRetry(url, options, maxRetries) {
        if (maxRetries === undefined) maxRetries = 3;
        for (var i = 0; i < maxRetries; i++) {
            try {
                var resp = await fetch(url, options);
                if (!resp.ok) {
                    throw new Error('HTTP ' + resp.status);
                }
                return await resp.json();
            } catch (err) {
                if (i === maxRetries - 1) throw err;
                await new Promise(function (r) { return setTimeout(r, 1000 * Math.pow(2, i)); });
            }
        }
    }

    // --- Load Initial Data ---
    async function loadInitialData() {
        try {
            var results = await Promise.allSettled([
                fetchWithRetry('/api/payments'),
                fetchWithRetry('/api/metrics')
            ]);

            if (results[0].status === 'fulfilled') {
                allPayments = results[0].value;
                renderPaymentsTable(allPayments);
            } else {
                console.error('Failed to load payments:', results[0].reason);
            }

            if (results[1].status === 'fulfilled') {
                renderMetrics(results[1].value);
            } else {
                console.error('Failed to load metrics:', results[1].reason);
            }
        } catch (err) {
            console.error('Failed to load initial data:', err);
        }
    }

    // --- Feed ---
    function addToFeed(event) {
        var div = document.createElement('div');
        div.className = 'feed-event';

        var statusSpan = document.createElement('span');
        statusSpan.className = 'event-status ' + event.status;
        statusSpan.textContent = (statusMap[event.status] || event.status).toUpperCase();

        var text = document.createTextNode(
            ' [' + event.payment_id + '] ' +
            (event.amount !== undefined ? event.amount.toFixed(2) + ' ' + (event.currency || '') : '') +
            ' - ' + (event.description || '') +
            ' @ ' + (event.timestamp || '')
        );

        div.appendChild(statusSpan);
        div.appendChild(text);
        feedContainer.insertBefore(div, feedContainer.firstChild);

        // Limit feed to 200 events
        while (feedContainer.children.length > 200) {
            feedContainer.removeChild(feedContainer.lastChild);
        }
    }

    // --- Payments Table ---
    function renderPaymentsTable(payments) {
        paymentsTbody.innerHTML = '';

        if (!payments || payments.length === 0) {
            var tr = document.createElement('tr');
            var td = document.createElement('td');
            td.colSpan = 4;
            td.textContent = 'Nenhum pagamento encontrado.';
            td.style.textAlign = 'center';
            td.style.color = '#999';
            tr.appendChild(td);
            paymentsTbody.appendChild(tr);
            return;
        }

        payments.forEach(function (p) {
            var tr = document.createElement('tr');

            // Payment ID
            var tdId = document.createElement('td');
            tdId.textContent = p.payment_id;
            tr.appendChild(tdId);

            // Status
            var tdStatus = document.createElement('td');
            var badge = document.createElement('span');
            badge.className = 'status-badge ' + (p.status || 'pending');
            badge.textContent = statusMap[p.status] || p.status || '—';
            tdStatus.appendChild(badge);
            tr.appendChild(tdStatus);

            // Updated At
            var tdUpdated = document.createElement('td');
            tdUpdated.textContent = p.updated_at || '-';
            tr.appendChild(tdUpdated);

            // Actions
            var tdActions = document.createElement('td');
            var btn = document.createElement('button');
            btn.className = 'btn-history';
            btn.textContent = 'Ver Hist\u00F3rico';
            btn.addEventListener('click', function () {
                showHistory(p.payment_id);
            });
            tdActions.appendChild(btn);
            tr.appendChild(tdActions);

            paymentsTbody.appendChild(tr);
        });
    }

    function updatePayment(event) {
        var found = false;
        for (var i = 0; i < allPayments.length; i++) {
            if (allPayments[i].payment_id === event.payment_id) {
                allPayments[i].status = event.status;
                allPayments[i].updated_at = event.timestamp || event.processed_at;
                found = true;
                break;
            }
        }
        if (!found) {
            allPayments.push({
                payment_id: event.payment_id,
                status: event.status,
                updated_at: event.timestamp || event.processed_at
            });
        }
        applyFilters();
    }

    // --- Filters ---
    function applyFilters() {
        var idFilter = filterPaymentId.value.toLowerCase().trim();
        var statusFilter = filterStatus.value;

        var filtered = allPayments.filter(function (p) {
            if (idFilter && (p.payment_id || '').toLowerCase().indexOf(idFilter) === -1) {
                return false;
            }
            if (statusFilter && p.status !== statusFilter) {
                return false;
            }
            return true;
        });

        renderPaymentsTable(filtered);
    }

    filterPaymentId.addEventListener('input', applyFilters);
    filterStatus.addEventListener('change', applyFilters);

    // --- History Modal ---
    async function showHistory(paymentId) {
        currentHistoryPaymentId = paymentId;
        modalPaymentId.textContent = 'Hist\u00F3rico: ' + paymentId;
        historyTbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#999;">Carregando...</td></tr>';
        historyModal.classList.remove('hidden');

        try {
            var history = await fetchWithRetry('/api/payments/' + encodeURIComponent(paymentId) + '/history');
            renderHistoryTable(history);
        } catch (err) {
            historyTbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#f44336;">Falha ao carregar hist\u00F3rico: ' + err.message + '</td></tr>';
            console.error('Falha ao carregar hist\u00F3rico:', err);
        }
    }

    function renderHistoryTable(history) {
        historyTbody.innerHTML = '';

        if (!history || history.length === 0) {
            var tr = document.createElement('tr');
            var td = document.createElement('td');
            td.colSpan = 7;
            td.textContent = 'Nenhum registro de hist\u00F3rico encontrado.';
            td.style.textAlign = 'center';
            td.style.color = '#999';
            tr.appendChild(td);
            historyTbody.appendChild(tr);
            return;
        }

        history.forEach(function (h) {
            var tr = document.createElement('tr');

            var tdTs = document.createElement('td');
            tdTs.textContent = h.timestamp || '-';
            tr.appendChild(tdTs);

            var tdStatus = document.createElement('td');
            var badge = document.createElement('span');
            badge.className = 'status-badge ' + (h.status || '');
            badge.textContent = statusMap[h.status] || h.status || '-';
            tdStatus.appendChild(badge);
            tr.appendChild(tdStatus);

            var tdAmt = document.createElement('td');
            tdAmt.textContent = h.amount !== undefined && h.amount !== null ? h.amount.toFixed(2) : '-';
            tr.appendChild(tdAmt);

            var tdCur = document.createElement('td');
            tdCur.textContent = h.currency || '-';
            tr.appendChild(tdCur);

            var tdDesc = document.createElement('td');
            tdDesc.textContent = h.description || '-';
            tr.appendChild(tdDesc);

            var tdProc = document.createElement('td');
            tdProc.textContent = h.processed_at || '-';
            tr.appendChild(tdProc);

            var tdTrace = document.createElement('td');
            tdTrace.textContent = h.trace_id || '-';
            tdTrace.style.fontFamily = 'monospace';
            tdTrace.style.fontSize = '0.8rem';
            tr.appendChild(tdTrace);

            historyTbody.appendChild(tr);
        });
    }

    // Close modal
    function closeHistoryModal() {
        historyModal.classList.add('hidden');
        currentHistoryPaymentId = null;
    }

    closeModal.addEventListener('click', closeHistoryModal);
    historyModal.addEventListener('click', function (e) {
        if (e.target === historyModal) {
            closeHistoryModal();
        }
    });
    document.addEventListener('keydown', function (e) {
        if (e.key === 'Escape' && !historyModal.classList.contains('hidden')) {
            closeHistoryModal();
        }
    });

    // --- Metrics ---
    function renderMetrics(metrics) {
        if (!metrics) {
            metricsSection.innerHTML = '<div class="metric-card"><h3>M\u00E9tricas</h3><div class="metric-value">N/A</div></div>';
            return;
        }

        metricsSection.innerHTML = '';

        // Total
        var totalCard = createMetricCard('Total Processado', metrics.total_processed !== undefined ? String(metrics.total_processed) : '0', '');
        metricsSection.appendChild(totalCard);

        // By status
        if (metrics.by_status) {
            Object.keys(metrics.by_status).forEach(function (status) {
                var label = statusLabelMap[status] || (status.charAt(0).toUpperCase() + status.slice(1));
                var card = createMetricCard(label, String(metrics.by_status[status]), status);
                metricsSection.appendChild(card);
            });
        }

        // Success rate
        var rateCard = createMetricCard('Taxa de Sucesso', (metrics.success_rate !== undefined ? metrics.success_rate.toFixed(1) : '0') + '%', '');
        metricsSection.appendChild(rateCard);

        // DLQ Count
        var dlqCard = createMetricCard('DLQ Contagem', metrics.dlq_count !== undefined ? String(metrics.dlq_count) : '0', '');
        metricsSection.appendChild(dlqCard);
    }

    function createMetricCard(title, value, statusClass) {
        var div = document.createElement('div');
        div.className = 'metric-card';

        var h3 = document.createElement('h3');
        h3.textContent = title;
        div.appendChild(h3);

        var valDiv = document.createElement('div');
        valDiv.className = 'metric-value' + (statusClass ? ' ' + statusClass : '');
        valDiv.textContent = value;
        div.appendChild(valDiv);

        return div;
    }

    function refreshMetrics() {
        fetchWithRetry('/api/metrics').then(function (metrics) {
            renderMetrics(metrics);
        }).catch(function (err) {
            console.error('Failed to refresh metrics:', err);
        });
    }

    // --- Init ---
    updateConnectionStatus(false);
    connectSSE();
    loadInitialData();

    // Periodic metrics refresh (fallback if SSE misses)
    setInterval(function () {
        if (!isConnected) {
            refreshMetrics();
        }
    }, 30000);

})();
