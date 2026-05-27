# 04 — Plano de Implementação

## Etapas

### Etapa 1 — Injetar SyncProducer no Server e Handlers

**Arquivos**: `cmd/ui/main.go`, `internal/ui/server.go`, `internal/ui/handlers.go`

1. Em `cmd/ui/main.go`:
   - Criar `sarama.NewConfig()` com `Producer.Return.Successes = true`
   - Conectar `sarama.NewSyncProducer(brokers, config)`
   - Tratar erro graceful: log.Warn + `kafkaProducer = nil`
   - Passar `kafkaProducer` como parâmetro para `ui.NewServer()`
   - Garantir `defer kafkaProducer.Close()`

2. Em `internal/ui/server.go`:
   - Adicionar parâmetro `kafkaProducer sarama.SyncProducer` ao `NewServer()`
   - Passar para `NewHandlers()`

3. Em `internal/ui/handlers.go`:
   - Adicionar campos `kafkaProducer sarama.SyncProducer` e `kafkaTopic string`
   - Atualizar `NewHandlers()` para aceitar e armazenar os novos parâmetros

**Critério de aceite**: `go build ./cmd/ui/` compila sem erros

### Etapa 2 — Implementar Handler HandlePublish

**Arquivo**: `internal/ui/handlers.go`

1. Adicionar método `HandlePublish(w http.ResponseWriter, r *http.Request)`
2. Fluxo:
   - `r.Body = http.MaxBytesReader(w, r.Body, 100*1024)`
   - `json.NewDecoder(r.Body).Decode(&event)`
   - Se `event.PaymentID` vazio, gerar com `uuid.New().String()`
   - Se `event.Timestamp` vazio, preencher com `time.Now().UTC().Format(time.RFC3339)`
   - Validar com `h.validator.Validate(ctx, data)`
   - Publicar no Kafka com `h.kafkaProducer.SendMessage(msg)`
   - Publicar no EventBus com `h.eventBus.Publish(ctx, event)` (não-bloqueante)
   - Retornar JSON com status, partition, offset
3. Se `kafkaProducer` for `nil`, retornar 502 com mensagem "Kafka não disponível"

### Etapa 3 — Implementar Handler HandlePublishBulk

**Arquivo**: `internal/ui/handlers.go`

1. Adicionar método `HandlePublishBulk(w http.ResponseWriter, r *http.Request)`
2. Ler `count` do body JSON (default 10, max 50)
3. Chamar `producer.GenerateBulkEvents(count)` e iterar publicando cada um
4. Coletar resultados e retornar array no response

### Etapa 4 — Registrar Rotas no Server

**Arquivo**: `internal/ui/server.go`

Adicionar no `mux`:

```go
mux.HandleFunc("POST /api/publish", handlers.HandlePublish)
mux.HandleFunc("POST /api/publish/bulk", handlers.HandlePublishBulk)
```

A rota `GET /producer` será servida automaticamente pelo `http.FileServer`
já que `producer.html` estará dentro da pasta `static/`.

### Etapa 5 — Criar producer.html

**Arquivo**: `internal/ui/static/producer.html`

1. Estrutura HTML conforme seção 03-design.md
2. Incluir header com navegação (Dashboard ↔ Producer)
3. Formulário com todos os campos
4. Preview JSON dinâmico
5. Tabela de histórico
6. Container de toast

### Etapa 6 — Criar producer.js

**Arquivo**: `internal/ui/static/producer.js`

1. Lógica de auto-preenchimento (timestamp, UUID preview)
2. Validação frontend de todos os campos
3. Preview JSON atualizado em tempo real
4. Submit do formulário via fetch
5. Bulk publish via fetch
6. Tabela de histórico (array JS, max 50)
7. Toasts de feedback visual
8. Atualizar `index.html` para incluir link para `/producer`

### Etapa 6.5 — Atualizar Makefile e Dockerfile para copy-docs

**Arquivos**: `Makefile`, `Dockerfile.ui`

Adicionar target `copy-docs` no Makefile antes de `build-ui` e `run-ui`:

```makefile
copy-docs:
	@mkdir -p internal/ui/static/docs
	@cp -r docs/* internal/ui/static/docs/ 2>/dev/null || true

build-ui: copy-docs
	...

run-ui: copy-docs
	...
```

No `Dockerfile.ui`, adicionar o passo de cópia antes do build Go:

```dockerfile
RUN mkdir -p internal/ui/static/docs && cp -r docs/* internal/ui/static/docs/ 2>/dev/null || true
RUN go build -o /app/ui ./cmd/ui
```

Isso garante que a documentação seja embutida no binário via
`//go:embed static/*` e servida em `/docs/`.

### Etapa 7 — Adicionar Estilos

**Arquivo**: `internal/ui/static/style.css`

Adicionar classes CSS para:
- `.form-field`, `.form-field label`, `.form-field input`, etc.
- `.preview-box`
- `.btn`, `.btn-primary`, `.btn-secondary`
- `.field-error`
- `.toast`, `.toast.success`, `.toast.error`
- `.header-nav`

### Etapa 8 — Atualizar index.html

**Arquivo**: `internal/ui/static/index.html`

Adicionar link de navegação no header:

```html
<nav class="header-nav">
    <a href="/" class="active">🏠 Dashboard</a>
    <a href="/producer">📝 Producer</a>
    <a href="/docs/">📖 Documentação</a>
</nav>
```

### Etapa 9 — Atualizar docker-compose.yml

**Arquivo**: `docker-compose.yml`

Adicionar ao serviço `payment-ui`:

```yaml
environment:
  - KAFKA_BROKERS=kafka:9092
  - KAFKA_TOPIC=payment.events
```

### Etapa 10 — Testes

**Arquivo**: `internal/ui/handlers_test.go`

Adicionar testes:

1. `TestHandlePublish_Valid` — submeter evento válido, verificar 200
2. `TestHandlePublish_InvalidPayload` — JSON mal formado, verificar 400
3. `TestHandlePublish_ValidationError` — amount negativo, verificar 400
4. `TestHandlePublish_KafkaDown` — mock producer retorna erro, verificar 502
5. `TestHandlePublishBulk` — submeter count=3, verificar 200 com 3 resultados

## Dependências

- Nenhuma nova dependência Go (reusa sarama, validator, models)
- Nenhuma nova dependência frontend (HTML+CSS+JS vanilla)

## Ordem de Implementação Sugerida

```
1. handlers.go  →  Adicionar campos kafkaProducer/kafkaTopic
2. server.go    →  Injetar kafkaProducer, registrar rotas
3. main.go      →  Conectar Kafka, passar para NewServer
4. handler.go   →  Implementar HandlePublish + HandlePublishBulk
5. style.css    →  Adicionar estilos do formulário
6. producer.js  →  Lógica frontend
7. producer.html → Página HTML
8. index.html   →  Adicionar navegação
9. docker-compose.yml → Env vars Kafka
10. handlers_test.go → Testes
```

## Checklist de Build

```bash
go build ./cmd/ui/          # deve compilar
go vet ./internal/ui/       # sem warnings
go test ./internal/ui/      # todos os testes passam
```
