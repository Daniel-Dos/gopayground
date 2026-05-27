# 03 — Design

## Visão Geral da Arquitetura

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Browser (Cliente)                              │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                          Producer UI (producer.html)                    │  │
│  │                                                                         │  │
│  │  ┌──────────────────────────────────────┐  ┌────────────────────────┐  │  │
│  │  │         Formulário                    │  │  Preview JSON          │  │  │
│  │  │  ┌────────────────────────────────┐  │  │  {                     │  │  │
│  │  │  │ Payment ID: [uuid...        ]  │  │  │   "payment_id": "...", │  │  │
│  │  │  │ Status:     [confirmed     ▼]  │  │  │   "status": "conf...", │  │  │
│  │  │  │ Amount:     [150.00         ]  │  │  │   "amount": 150.00,    │  │  │
│  │  │  │ Currency:   [BRL            ]  │  │  │   "currency": "BRL",   │  │  │
│  │  │  │ Description:[Pedido #123    ]  │  │  │   ...                  │  │  │
│  │  │  │ Timestamp:  [2026-05-25...  ]  │  │  │   }                    │  │  │
│  │  │  └────────────────────────────────┘  │  └────────────────────────┘  │  │
│  │  │                                                                     │  │
│  │  │  [📤 Publicar]  [🎲 Publicar 10 Aleatórios]                       │  │
│  │  └──────────────────────────────────────────────────────────────────────┘  │
│  │                                                                             │
│  │  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │  │  Últimas Publicações (sessão)                                        │  │
│  │  │  ┌──────┬──────────┬────────┬──────────┬────────────────┬─────────┐ │  │
│  │  │  │  ID  │ Status   │ Amount │ Currency │ Timestamp      │ Result  │ │  │
│  │  │  ├──────┼──────────┼────────┼──────────┼────────────────┼─────────┤ │  │
│  │  │  │ abc… │ ✅ Conf. │ 150.00 │ BRL      │ 2026-05-25…    │ ✓ p:0   │ │  │
│  │  │  │ def… │ ❌ Falha │ 200.00 │ USD      │ 2026-05-25…    │ ✗ erro  │ │  │
│  │  │  └──────┴──────────┴────────┴──────────┴────────────────┴─────────┘ │  │
│  │  └──────────────────────────────────────────────────────────────────────┘  │
│  │                                                                             │
│  │  ┌──────────────────────────────────────────┐                              │
│  │  │  [🏠 Dashboard] [📝 Producer] [📖 Docs] │ ← Navegação                  │
│  │  └──────────────────────────────────────────┘                              │
│  └─────────────────────────────────┬─────────────────────────────────────────┘
│                                    │
│           ┌────────────────────────┼────────────────────────────┐
│           │ GET /producer          │ POST /api/publish          │
│           │ (static HTML)          │ (JSON PaymentEvent)        │
│           ▼                        ▼                            │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────────┐│
│  │                  Go HTTP Server (:8081)                      ││
│  │                                                              ││
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌────────────┐ ││
│  │  │ Static File      │  │ HandlePublish    │  │ Handlers   │ ││
│  │  │ Server           │  │ POST /api/publish│  │ existentes │ ││
│  │  │ (index.html,     │  │                  │  │ (SSE,      │ ││
│  │  │  producer.html,  │  │ 1. Validar       │  │  payments, │ ││
│  │  │  app.js,         │  │ 2. Publicar Kafka│  │  metrics,  │ ││
│  │  │  style.css)      │  │ 3. Publicar Redis│  │  etc.)     │ ││
│  │  │                  │  │ 4. Retornar 200  │  │            │ ││
│  │  └──────────────────┘  └────────┬─────────┘  └────────────┘ ││
│  │                                 │                            ││
│  │                    ┌────────────┴────────────┐               ││
│  │                    │   EventBus (Redis Pub/  │               ││
│  │                    │   Sub: payment:events)  │               ││
│  │                    └────────────┬────────────┘               ││
│  │                                 │                            ││
│  └─────────────────────────────────┼────────────────────────────┘│
│                                    │                              │
│                    ┌───────────────┴───────────────┐              │
│                    ▼                               ▼              │
│            ┌──────────────┐               ┌──────────────┐       │
│            │    Kafka     │               │  Redis       │       │
│            │ payment.events│              │  Pub/Sub     │       │
│            └──────┬───────┘               └──────┬───────┘       │
│                   │                              │                │
│                   ▼                              ▼                │
│            ┌──────────────┐               ┌──────────────┐       │
│            │  Consumer    │               │  SSE Stream  │       │
│            │  (processa)  │               │  (dashboard) │       │
│            └──────────────┘               └──────────────┘       │
└──────────────────────────────────────────────────────────────────┘
```

## Endpoints da API (novos)

| Método | Rota                    | Descrição                                   | Formato Resposta            |
|--------|-------------------------|---------------------------------------------|-----------------------------|
| POST   | `/api/publish`          | Publica um evento de pagamento no Kafka     | `application/json` (objeto) |
| POST   | `/api/publish/bulk`     | Publica N eventos aleatórios (default 10)   | `application/json` (array)  |
| GET    | `/producer`             | Serve `producer.html`                       | `text/html`                 |

### POST /api/publish

Publica um único `PaymentEvent` no Kafka e no EventBus (Redis Pub/Sub).

**Request Body:**
```json
{
  "payment_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "status": "confirmed",
  "amount": 150.00,
  "currency": "BRL",
  "description": "Pedido #12345",
  "timestamp": "2026-05-25T10:00:00Z"
}
```

**Response 200 (sucesso):**
```json
{
  "status": "success",
  "payment_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "partition": 0,
  "offset": 42
}
```

**Response 400 (erro de validação):**
```json
{
  "status": "error",
  "error": "validation error: Key: 'PaymentEvent.Amount' Error:...",
  "payment_id": "invalid"
}
```

**Response 502 (erro Kafka):**
```json
{
  "status": "error",
  "error": "kafka not available: ...",
  "payment_id": "a1b2c3d4-..."
}
```

**Implementação no handler:**

```go
func (h *Handlers) HandlePublish(w http.ResponseWriter, r *http.Request) {
    // 1. Limitar tamanho do corpo
    r.Body = http.MaxBytesReader(w, r.Body, 100*1024) // 100KB

    // 2. Ler e validar
    var event models.PaymentEvent
    if err := json.NewDecoder(r.Body).Decode(&event); err != nil { ... }

    // 3. Validar com validator existente
    data, _ := json.Marshal(event)
    validatedEvent, err := h.validator.Validate(ctx, data)
    if err != nil { writeError(w, 400, err.Error()); return }

    // 4. Gerar PaymentID se vazio
    if validatedEvent.PaymentID == "" {
        validatedEvent.PaymentID = uuid.New().String()
    }

    // 5. Preencher timestamp se vazio
    if validatedEvent.Timestamp == "" {
        validatedEvent.Timestamp = time.Now().UTC().Format(time.RFC3339)
    }

    // 6. Publicar no Kafka (com timeout)
    kafkaCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    eventData, _ := json.Marshal(validatedEvent)
    msg := &sarama.ProducerMessage{
        Topic: h.kafkaTopic,
        Key:   sarama.StringEncoder(validatedEvent.PaymentID),
        Value: sarama.ByteEncoder(eventData),
        Headers: []sarama.RecordHeader{
            {Key: []byte("source"), Value: []byte("producer-ui")},
            {Key: []byte("timestamp"), Value: []byte(time.Now().UTC().Format(time.RFC3339))},
        },
    }
    partition, offset, err := h.kafkaProducer.SendMessage(msg)
    if err != nil { writeError(w, 502, "kafka error: "+err.Error()); return }

    // 7. Publicar no EventBus (Redis Pub/Sub) para SSE
    if pubErr := h.eventBus.Publish(ctx, validatedEvent); pubErr != nil {
        h.logger.Warn("eventbus publish failed after kafka success",
            "payment_id", validatedEvent.PaymentID, "error", pubErr)
        // Não falha a request — Kafka já publicou
    }

    // 8. Responder sucesso
    writeJSON(w, 200, map[string]interface{}{
        "status":     "success",
        "payment_id": validatedEvent.PaymentID,
        "partition":  partition,
        "offset":     offset,
    })
}
```

### POST /api/publish/bulk

Gera e publica N eventos aleatórios (default 10, máximo 50).

**Request Body (opcional):**
```json
{
  "count": 10
}
```

**Response 200:**
```json
{
  "status": "success",
  "published": 10,
  "results": [
    { "payment_id": "...", "partition": 0, "offset": 43 },
    { "payment_id": "...", "partition": 1, "offset": 12 },
    ...
  ]
}
```

**Implementação**: reusa `producer.GenerateBulkEvents(count)` e itera
chamando o mesmo fluxo de `HandlePublish`.

## Modificações no Backend

### 1. Server (`internal/ui/server.go`)

O construtor `NewServer` deve receber um `sarama.SyncProducer` adicional:

```go
type Server struct {
    httpServer  *http.Server
    eventBus    *EventBus
    handlers    *Handlers
    logger      *slog.Logger
}

func NewServer(
    cfg config.Config,
    rdb *redis.Client,
    dynamoClient *dynamodb.Client,
    kafkaProducer sarama.SyncProducer,  // NOVO
    logger *slog.Logger,
) *Server {
    eventBus := NewEventBus(rdb, "payment:events", cfg.UIEventBusBuffer, logger)
    handlers := NewHandlers(rdb, dynamoClient, cfg.DynamoDBTable, eventBus, kafkaProducer, cfg.KafkaTopic, logger)
    // ...
}
```

Novas rotas a registrar:

```go
mux.HandleFunc("POST /api/publish", handlers.HandlePublish)
mux.HandleFunc("POST /api/publish/bulk", handlers.HandlePublishBulk)
```

### 2. Handlers (`internal/ui/handlers.go`)

Adicionar campos ao `Handlers`:

```go
type Handlers struct {
    redis        *redis.Client
    dynamo       DynamoDBQueryAPI
    dynamoTbl    string
    eventBus     *EventBus
    kafkaProducer sarama.SyncProducer  // NOVO
    kafkaTopic    string                // NOVO
    logger       *slog.Logger
}
```

Adicionar métodos:

- `HandlePublish(w, r)` — publica 1 evento
- `HandlePublishBulk(w, r)` — publica N eventos aleatórios

### 3. Config (`internal/config/config.go`)

O config já possui `KafkaBrokers` e `KafkaTopic`. O `cmd/ui/main.go` precisará
conectar ao Kafka:

```go
// Conectar Kafka (sync producer)
kafkaConfig := sarama.NewConfig()
kafkaConfig.Producer.Return.Successes = true
kafkaConfig.Producer.Timeout = 10 * time.Second
kafkaConfig.Net.DialTimeout = 5 * time.Second
kafkaConfig.Producer.MaxMessageBytes = 100 * 1024

kafkaProducer, err := sarama.NewSyncProducer(strings.Split(cfg.KafkaBrokers, ","), kafkaConfig)
if err != nil {
    logger.Warn("kafka not available, producer UI will be limited", "error", err)
    kafkaProducer = nil // Handlers tratam nil gracefulmente
}
defer func() {
    if kafkaProducer != nil { kafkaProducer.Close() }
}()

server := ui.NewServer(cfg, rdb, dynamoClient, kafkaProducer, logger)
```

**Importante**: Se Kafka não estiver disponível, o servidor **não deve falhar**
— o handler deve retornar 502 com mensagem clara.

## Frontend (`producer.html` + JS inline)

### Estrutura HTML

```html
<!DOCTYPE html>
<html lang="pt-BR">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Publicar Pagamento</title>
    <link rel="stylesheet" href="style.css">
</head>
<body>
    <div class="dashboard">
        <!-- HEADER com navegação -->
        <header>
            <div class="header-brand">...</div>
            <nav>
                <a href="/">🏠 Dashboard</a>
                <a href="/producer" class="active">📝 Producer</a>
                <a href="/docs/">📖 Documentação</a>
            </nav>
        </header>

        <!-- TOAST container -->
        <div id="toast-container"></div>

        <!-- FORMULÁRIO -->
        <section class="dashboard-section">
            <h2>📤 Publicar Pagamento</h2>
            <form id="publish-form">
                <!-- Payment ID -->
                <div class="form-field">
                    <label>ID do Pagamento (UUID)</label>
                    <input type="text" id="field-payment-id" placeholder="Auto-gerado se vazio">
                    <span class="field-error" id="error-payment-id"></span>
                </div>

                <!-- Status -->
                <div class="form-field">
                    <label>Situação</label>
                    <select id="field-status">
                        <option value="pending">Pendente</option>
                        <option value="confirmed" selected>Confirmado</option>
                        <option value="failed">Falhou</option>
                        <option value="refunded">Reembolsado</option>
                    </select>
                </div>

                <!-- Amount -->
                <div class="form-field">
                    <label>Valor</label>
                    <input type="number" id="field-amount" step="0.01" min="0.01" value="100.00">
                    <span class="field-error" id="error-amount"></span>
                </div>

                <!-- Currency -->
                <div class="form-field">
                    <label>Moeda</label>
                    <input type="text" id="field-currency" maxlength="3" value="BRL" style="text-transform:uppercase">
                    <span class="field-error" id="error-currency"></span>
                </div>

                <!-- Description -->
                <div class="form-field">
                    <label>Descrição</label>
                    <textarea id="field-description" maxlength="255" rows="2"></textarea>
                    <span class="field-error" id="error-description"></span>
                </div>

                <!-- Timestamp -->
                <div class="form-field">
                    <label>Timestamp (auto-preenchido)</label>
                    <input type="text" id="field-timestamp">
                    <span class="field-error" id="error-timestamp"></span>
                </div>

                <!-- Preview -->
                <div class="form-field">
                    <label>Pré-visualização (JSON)</label>
                    <pre id="preview-json" class="preview-box">{
    "payment_id": "",
    "status": "confirmed",
    ...
}</pre>
                </div>

                <!-- Buttons -->
                <div class="form-actions">
                    <button type="submit" id="btn-publish" class="btn btn-primary">
                        📤 Publicar
                    </button>
                    <button type="button" id="btn-bulk" class="btn btn-secondary">
                        🎲 Publicar 10 Aleatórios
                    </button>
                </div>
            </form>
        </section>

        <!-- TABELA DE HISTÓRICO -->
        <section class="dashboard-section">
            <h2>Últimas Publicações</h2>
            <div class="table-wrap">
                <table id="history-table">
                    <thead>
                        <tr>
                            <th>ID do Pagamento</th>
                            <th>Situação</th>
                            <th>Valor</th>
                            <th>Moeda</th>
                            <th>Timestamp</th>
                            <th>Resultado</th>
                        </tr>
                    </thead>
                    <tbody></tbody>
                </table>
            </div>
        </section>
    </div>

    <script src="producer.js"></script>
</body>
</html>
```

### Lógica JS (`producer.js`)

Responsabilidades:

1. **Auto-preenchimento**:
   - Timestamp com hora atual (RFC3339) ao carregar
   - Se Payment ID vazio, gerar UUID v4 no preview

2. **Pré-visualização dinâmica**:
   - Atualizar `<pre id="preview-json">` em tempo real conforme formulário muda
   - Mostrar o JSON que será enviado

3. **Validação frontend**:
   - Validar cada campo onChange e onSubmit
   - Exibir mensagens de erro inline em `<span class="field-error">`
   - Impedir submit se inválido

4. **Submit**:
   - `POST /api/publish` com JSON
   - Desabilitar botão durante envio
   - Exibir toast de sucesso (verde) ou erro (vermelho)
   - Adicionar linha na tabela de histórico

5. **Bulk**:
   - `POST /api/publish/bulk` com `{"count": 10}`
   - Aguardar resposta
   - Adicionar múltiplas linhas na tabela

6. **Tabela de histórico**:
   - Armazenar em array JS (sessionStorage?)
   - Máximo 50 eventos (FIFO)
   - Renderizar do mais recente para o mais antigo

7. **Toasts**:
   - Container fixo no canto superior direito
   - Auto-destruir após 5 segundos
   - Animação de fade-out

### Estilos (adicional em `style.css`)

Adicionar classes:

```css
/* Form fields */
.form-field { margin-bottom: 1rem; }
.form-field label { display: block; margin-bottom: 0.25rem; ... }
.form-field input, .form-field select, .form-field textarea { ... }

/* Preview box */
.preview-box { ... }

/* Buttons */
.btn { padding: 0.6rem 1.2rem; border: none; border-radius: 8px; cursor: pointer; }
.btn-primary { background: var(--accent); color: #fff; }
.btn-secondary { background: var(--surface); color: var(--text); }

/* Error messages */
.field-error { color: #f44336; font-size: 0.8rem; display: none; }

/* Toast */
.toast { position: fixed; top: 20px; right: 20px; ... }
.toast.success { border-left: 4px solid #4caf50; }
.toast.error { border-left: 4px solid #f44336; }

/* Navigation */
.header-nav { display: flex; gap: 1rem; }
.header-nav a { ... }
.header-nav a.active { ... }
```

## Estrutura de Pastas (alterações)

```
internal/
  ui/
    server.go              # + Injeção kafkaProducer nas rotas
    handlers.go            # + HandlePublish, HandlePublishBulk
    handlers_test.go       # + Testes dos novos handlers
      static/
        docs/                # Documentação do projeto (copiada de /docs/ no build)
        producer.html        # NOVO: página de publicação
        producer.js          # NOVO: lógica JS da página
        style.css            # + Estilos do formulário, preview, toasts
        index.html           # + Link de navegação para /producer e /docs/

cmd/
  ui/
    main.go                # + Conexão Kafka SyncProducer

config/
  config.go                # (já tem KafkaBrokers, KafkaTopic)

docker-compose.yml         # + Env vars Kafka no payment-ui
```

## Configuração Docker

Adicionar ao serviço `payment-ui` no `docker-compose.yml`:

```yaml
payment-ui:
  environment:
    - KAFKA_BROKERS=kafka:9092
    - KAFKA_TOPIC=payment.events
```

## Tratamento de Erros

| Cenário                          | HTTP Status | Resposta                               |
|----------------------------------|-------------|----------------------------------------|
| Payload inválido (JSON mal formado) | 400     | `{"status":"error","error":"..."}`     |
| Validação falhou (campos inválidos) | 400     | `{"status":"error","error":"..."}`     |
| Kafka indisponível               | 502         | `{"status":"error","error":"kafka..."}`|
| Payload muito grande (>100KB)    | 413         | `{"status":"error","error":"too large"}`|
| Rate limit excedido              | 429         | `{"status":"error","error":"too many requests"}`|
