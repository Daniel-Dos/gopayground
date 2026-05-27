// Payment Producer UI — Application Logic
// Português-Brasil, vanilla JS, zero frameworks

(function () {
    'use strict';

    // --- State ---
    var historyKey = 'producer:publications';
    var publications = loadHistory();

    // --- DOM References ---
    var form = document.getElementById('publish-form');
    var paymentIdInput = document.getElementById('payment-id');
    var statusSelect = document.getElementById('status');
    var amountInput = document.getElementById('amount');
    var currencyInput = document.getElementById('currency');
    var descriptionInput = document.getElementById('description');
    var previewJson = document.getElementById('preview-json');
    var btnPublish = document.getElementById('btn-publish');
    var btnPublishBulk = document.getElementById('btn-publish-bulk');
    var btnClearHistory = document.getElementById('btn-clear-history');
    var resultsTbody = document.getElementById('results-tbody');
    var toastContainer = document.getElementById('toast-container');

    var statusMap = {
        pending: 'Pendente',
        confirmed: 'Confirmado',
        failed: 'Falhou',
        refunded: 'Reembolsado'
    };

    // --- UUID Validation ---
    var uuidRe = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

    function isValidUUID(str) {
        return uuidRe.test(str);
    }

    // --- Load / Save History ---
    function loadHistory() {
        try {
            var data = sessionStorage.getItem(historyKey);
            return data ? JSON.parse(data) : [];
        } catch (e) {
            return [];
        }
    }

    function saveHistory() {
        try {
            sessionStorage.setItem(historyKey, JSON.stringify(publications));
        } catch (e) {
            // sessionStorage full or unavailable — silently ignore
        }
    }

    function addToHistory(item) {
        publications.unshift(item);
        // Keep max 50 entries
        if (publications.length > 50) {
            publications = publications.slice(0, 50);
        }
        saveHistory();
        renderResultsTable();
    }

    function clearHistory() {
        publications = [];
        saveHistory();
        renderResultsTable();
    }

    // --- Build Preview JSON ---
    function buildPreview() {
        var paymentId = paymentIdInput.value.trim() || '<auto-gerado>';
        var status = statusSelect.value;
        var amount = parseFloat(amountInput.value) || 0;
        var currency = currencyInput.value.trim().toUpperCase() || 'BRL';
        var description = descriptionInput.value.trim() || '';
        var timestamp = new Date().toISOString();

        var preview = {
            payment_id: paymentId,
            status: status,
            amount: amount,
            currency: currency,
            description: description,
            timestamp: timestamp
        };

        renderPreview(preview);
        return preview;
    }

    function renderPreview(preview) {
        var json = JSON.stringify(preview, null, 2);
        // Syntax-highlight the JSON
        var highlighted = json.replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"([^"]+)":/g, '<span class="preview-key">"$1"</span>:')
            .replace(/: "([^"]+)"/g, function (match, value) {
                // Color by value type
                if (value === '<auto-gerado>') {
                    return ': <span class="preview-special">"' + value + '"</span>';
                }
                return ': <span class="preview-string">"' + value + '"</span>';
            })
            .replace(/: (\d+\.?\d*)/g, ': <span class="preview-number">$1</span>')
            .replace(/: (true|false)/g, ': <span class="preview-boolean">$1</span>')
            .replace(/: (null)/g, ': <span class="preview-null">$1</span>');

        previewJson.innerHTML = '<code>' + highlighted + '</code>';
    }

    // --- Form validation ---
    function validate() {
        var errors = [];

        var paymentId = paymentIdInput.value.trim();
        if (paymentId && !isValidUUID(paymentId)) {
            errors.push('ID do pagamento inválido. Deve ser um UUID válido (formato xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx).');
        }

        var amount = parseFloat(amountInput.value);
        if (isNaN(amount) || amount <= 0) {
            errors.push('Valor deve ser maior que zero.');
        }

        var currency = currencyInput.value.trim().toUpperCase();
        if (currency.length !== 3) {
            errors.push('Moeda deve ter exatamente 3 letras (ex: BRL, USD, EUR).');
        }

        var description = descriptionInput.value;
        if (description.length > 255) {
            errors.push('Descrição muito longa (máximo 255 caracteres).');
        }

        return errors;
    }

    // --- Toast notifications ---
    function showToast(message, type, duration) {
        if (duration === undefined) duration = 4500;

        var toast = document.createElement('div');
        toast.className = 'toast toast-' + type;
        toast.textContent = message;

        // Close button
        var closeBtn = document.createElement('button');
        closeBtn.className = 'toast-close';
        closeBtn.innerHTML = '&times;';
        closeBtn.setAttribute('aria-label', 'Fechar');
        closeBtn.addEventListener('click', function () {
            dismissToast(toast);
        });
        toast.appendChild(closeBtn);

        toastContainer.appendChild(toast);

        // Trigger enter animation
        requestAnimationFrame(function () {
            toast.classList.add('toast-enter');
        });

        // Auto dismiss
        if (duration > 0) {
            setTimeout(function () {
                dismissToast(toast);
            }, duration);
        }
    }

    function dismissToast(toast) {
        if (toast.classList.contains('toast-leave')) return;
        toast.classList.remove('toast-enter');
        toast.classList.add('toast-leave');
        setTimeout(function () {
            if (toast.parentNode) {
                toast.parentNode.removeChild(toast);
            }
        }, 300);
    }

    // --- Publish event ---
    async function publishEvent(data) {
        var resp = await fetch('/api/publish', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data)
        });

        var result = await resp.json();

        if (!resp.ok) {
            throw new Error(result.error || 'Erro ' + resp.status);
        }

        return result;
    }

    // --- Publish bulk ---
    async function publishBulk(count) {
        var resp = await fetch('/api/publish/bulk', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ count: count })
        });

        var results = await resp.json();

        if (!resp.ok) {
            throw new Error(results.error || 'Erro ' + resp.status);
        }

        return results;
    }

    // --- Handle single publish ---
    async function handlePublish() {
        var errors = validate();
        if (errors.length > 0) {
            showToast(errors.join(' | '), 'error', 6000);
            return;
        }

        var paymentId = paymentIdInput.value.trim() || '';
        var status = statusSelect.value;
        var amount = parseFloat(amountInput.value);
        var currency = currencyInput.value.trim().toUpperCase();
        var description = descriptionInput.value.trim();

        var data = {
            payment_id: paymentId || undefined,
            status: status,
            amount: amount,
            currency: currency,
            description: description || undefined
        };

        // Clean undefined fields for cleaner JSON
        if (data.payment_id === undefined) delete data.payment_id;
        if (data.description === undefined) delete data.description;

        btnPublish.disabled = true;
        btnPublish.innerHTML = '<span class="btn-spinner"></span> Publicando...';

        try {
            var result = await publishEvent(data);

            addToHistory({
                payment_id: result.payment_id,
                status: status,
                amount: amount,
                currency: currency,
                partition: result.partition,
                offset: result.offset,
                timestamp: new Date().toISOString()
            });

            showToast('Pagamento ' + result.payment_id.slice(0, 8) + '... publicado com sucesso!', 'success');

            // Clear form (keep currency default)
            paymentIdInput.value = '';
            amountInput.value = '';
            descriptionInput.value = '';
            buildPreview();

        } catch (err) {
            showToast('Falha ao publicar: ' + err.message, 'error', 6000);
        } finally {
            btnPublish.disabled = false;
            btnPublish.innerHTML = '<span class="btn-icon">&#9654;</span> Publicar';
        }
    }

    // --- Handle bulk publish ---
    async function handlePublishBulk() {
        btnPublishBulk.disabled = true;
        btnPublishBulk.innerHTML = '<span class="btn-spinner"></span> Publicando...';

        try {
            var results = await publishBulk(10);

            var successCount = 0;
            var failCount = 0;

            results.forEach(function (item) {
                if (item.error) {
                    failCount++;
                } else {
                    successCount++;
                    addToHistory({
                        payment_id: item.payment_id,
                        status: item.status || 'confirmed',
                        amount: 0,
                        currency: 'BRL',
                        partition: item.partition,
                        offset: item.offset,
                        timestamp: new Date().toISOString()
                    });
                }
            });

            var msg = successCount + ' evento(s) publicado(s) com sucesso.';
            if (failCount > 0) {
                msg += ' ' + failCount + ' falha(s).';
            }
            showToast(msg, failCount > 0 ? 'warning' : 'success', 5000);

        } catch (err) {
            showToast('Falha ao publicar lote: ' + err.message, 'error', 6000);
        } finally {
            btnPublishBulk.disabled = false;
            btnPublishBulk.innerHTML = '<span class="btn-icon">&#9851;</span> Publicar 10 Aleatórios';
        }
    }

    // --- Render results table ---
    function renderResultsTable() {
        resultsTbody.innerHTML = '';

        if (!publications || publications.length === 0) {
            var tr = document.createElement('tr');
            var td = document.createElement('td');
            td.colSpan = 6;
            td.className = 'empty-cell';
            td.textContent = 'Nenhuma publicação ainda.';
            tr.appendChild(td);
            resultsTbody.appendChild(tr);
            return;
        }

        publications.forEach(function (p) {
            var tr = document.createElement('tr');

            // Payment ID
            var tdId = document.createElement('td');
            tdId.className = 'cell-mono';
            tdId.textContent = p.payment_id || '-';
            tr.appendChild(tdId);

            // Status
            var tdStatus = document.createElement('td');
            var badge = document.createElement('span');
            badge.className = 'status-badge ' + (p.status || '');
            badge.textContent = statusMap[p.status] || p.status || '-';
            tdStatus.appendChild(badge);
            tr.appendChild(tdStatus);

            // Amount
            var tdAmount = document.createElement('td');
            tdAmount.className = 'cell-mono';
            tdAmount.textContent = p.amount !== undefined && p.amount !== null && p.amount > 0
                ? p.currency + ' ' + p.amount.toFixed(2)
                : '-';
            tr.appendChild(tdAmount);

            // Partition
            var tdPart = document.createElement('td');
            tdPart.className = 'cell-mono';
            tdPart.textContent = p.partition !== undefined ? p.partition : '-';
            tr.appendChild(tdPart);

            // Offset
            var tdOff = document.createElement('td');
            tdOff.className = 'cell-mono';
            tdOff.textContent = p.offset !== undefined ? p.offset : '-';
            tr.appendChild(tdOff);

            // Timestamp
            var tdTs = document.createElement('td');
            tdTs.textContent = formatTimestamp(p.timestamp);
            tr.appendChild(tdTs);

            resultsTbody.appendChild(tr);
        });
    }

    function formatTimestamp(ts) {
        if (!ts) return '-';
        try {
            var d = new Date(ts);
            return d.toLocaleString('pt-BR', {
                day: '2-digit',
                month: '2-digit',
                year: 'numeric',
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit'
            });
        } catch (e) {
            return ts;
        }
    }

    // --- Event Listeners ---
    // Live preview on input
    form.addEventListener('input', buildPreview);
    form.addEventListener('change', buildPreview);

    // Publish buttons
    btnPublish.addEventListener('click', handlePublish);
    btnPublishBulk.addEventListener('click', function () { handlePublishBulk(); });

    // Clear history
    btnClearHistory.addEventListener('click', function () {
        if (publications.length === 0) return;
        if (confirm('Tem certeza que deseja limpar o histórico de publicações?')) {
            clearHistory();
            showToast('Histórico limpo.', 'info');
        }
    });

    // Keyboard shortcut: Ctrl+Enter to publish
    form.addEventListener('keydown', function (e) {
        if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
            e.preventDefault();
            handlePublish();
        }
    });

    // --- Init ---
    renderResultsTable();
    buildPreview();

})();
