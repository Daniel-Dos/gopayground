# 04 — Plano de Implementação

## Etapas

### Etapa 1 — Adicionar ProducerURL na Config

**Arquivo**: `internal/config/config.go`

1. Adicionar campo `ProducerURL string` na struct `UIConfig`:
   ```go
   ProducerURL string `env:"UI_PRODUCER_URL" default:"http://localhost:8082"`
   ```

**Critério de aceite**: `go build ./cmd/ui/` compila sem erros

### Etapa 2 — Remover SyncProducer do Server e Injetar ProducerURL

**Arquivos**: `internal/ui/server.go`, `cmd/ui/main.go`

1. Em `internal/ui/server.go`:
   - Remover parâmetro `kafkaProducer sarama.SyncProducer` do `NewServer()`
   - Remover import de `sarama`
   - Passar `cfg.UI.ProducerURL` para `NewHandlers()`
   - O Server cria um `http.Client` interno com timeout

2. Em `cmd/ui/main.go`:
   - **Remover** todo o bloco de conexão Kafka SyncProducer
   - **Remover** `createKafkaProducer()`, `sarama.SyncProducer`, `producer.Service`
   - **Remover** `defer kafkaProducer.Close()`
   - Chamar `ui.NewServer(cfg, rdb, dynamoClient, logger)` sem produtor

**Critério de aceite**: `go build ./cmd/ui/` compila sem dependência de `sarama`

### Etapa 3 — Implementar Handler HandlePublish (HTTP Client)

**Arquivo**: `internal/ui/handlers.go`

1. Adicionar campos ao `Handlers`:
   ```go
   producerURL  string
   httpClient   *http.Client
   ```

2. Adicionar método `HandlePublish(w http.ResponseWriter, r *http.Request)`
3. Fluxo:
   - `r.Body = http.MaxBytesReader(w, r.Body, 100*1024)`
   - `json.NewDecoder(r.Body).Decode(&event)`
   - Se `event.PaymentID` vazio, gerar com `uuid.New().String()`
   - Se `event.Timestamp` vazio, preencher com `time.Now().UTC().Format(time.RFC3339)`
   - Fazer HTTP POST para `h.producerURL + "/publish"` com o JSON do evento
   - Timeout de 10s via `context.WithTimeout`
   - Encaminhar resposta do Producer para o cliente
4. Se Producer Service retornar erro ou estiver indisponível, retornar 502

### Etapa 4 — Implementar Handler HandlePublishBulk

**Arquivo**: `internal/ui/handlers.go`

1. Adicionar método `HandlePublishBulk(w http.ResponseWriter, r *http.Request)`
2. Ler `count` do body JSON (default 10, max 50)
3. Encaminhar requisição para `h.producerURL + "/publish/bulk"`
4. Devolver resposta do Producer diretamente ao cliente

### Etapa 5 — Registrar Rotas no Server

**Arquivo**: `internal/ui/server.go`

Adicionar no `mux`:

```go
mux.HandleFunc("POST /api/publish", handlers.HandlePublish)
mux.HandleFunc("POST /api/publish/bulk", handlers.HandlePublishBulk)
```

A rota `GET /producer` será servida automaticamente pelo `http.FileServer`
já que `producer.html` estará dentro da pasta `static/`.

### Etapa 6 — Criar producer.html

**Arquivo**: `internal/ui/static/producer.html`

1. Estrutura HTML conforme seção 03-design.md
2. Incluir header com navegação (Dashboard ↔ Producer)
3. Formulário com todos os campos
4. Preview JSON dinâmico
5. Tabela de histórico
6. Container de toast

### Etapa 7 — Criar producer.js

**Arquivo**: `internal/ui/static/producer.js`

1. Lógica de auto-preenchimento (timestamp, UUID preview)
2. Validação frontend de todos os campos
3. Preview JSON atualizado em tempo real
4. Submit do formulário via fetch
5. Bulk publish via fetch
6. Tabela de histórico (array JS, max 50)
7. Toasts de feedback visual
8. Atualizar `index.html` para incluir link para `/producer`

### Etapa 7.5 — Atualizar Makefile e Dockerfile para copy-docs

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

### Etapa 8 — Adicionar Estilos

**Arquivo**: `internal/ui/static/style.css`

Adicionar classes CSS para:
- `.form-field`, `.form-field label`, `.form-field input`, etc.
- `.preview-box`
- `.btn`, `.btn-primary`, `.btn-secondary`
- `.field-error`
- `.toast`, `.toast.success`, `.toast.error`
- `.header-nav`

### Etapa 9 — Atualizar index.html

**Arquivo**: `internal/ui/static/index.html`

Adicionar link de navegação no header:

```html
<nav class="header-nav">
    <a href="/" class="active">🏠 Dashboard</a>
    <a href="/producer">📝 Producer</a>
    <a href="/docs/">📖 Documentação</a>
</nav>
```

### Etapa 10 — Atualizar docker-compose.yml

**Arquivo**: `docker-compose.yml`

Adicionar ao serviço `payment-ui`:

```yaml
environment:
  - UI_PRODUCER_URL=http://producer:8082
```

> Nota: Remover `KAFKA_BROKERS` e `KAFKA_TOPIC` do serviço `payment-ui`
> (essas variáveis agora são configuradas no serviço `producer`).

### Etapa 11 — Testes

**Arquivo**: `internal/ui/handlers_test.go`

Adicionar testes:

1. `TestHandlePublish_Valid` — submeter evento válido, mock HTTP server do Producer retorna 200, verificar 200
2. `TestHandlePublish_InvalidPayload` — JSON mal formado, verificar 400
3. `TestHandlePublish_ProducerDown` — Producer Service offline, verificar 502
4. `TestHandlePublish_ProducerError` — Producer retorna 400/422, verificar eco do erro
5. `TestHandlePublishBulk` — submeter count=3, mock Producer retorna 200 com 3 resultados

## Dependências

- Nenhuma nova dependência Go (usa apenas stdlib `net/http` para HTTP client, reusa `models`, `validator`)
- Nenhuma dependência de `sarama` na UI (removeu completamente)
- Nenhuma nova dependência frontend (HTML+CSS+JS vanilla)

## Ordem de Implementação Sugerida

```
1. config.go     →  Adicionar ProducerURL
2. handlers.go   →  Adicionar campos producerURL/httpClient
3. server.go     →  NewServer sem SyncProducer, registrar rotas
4. main.go       →  Remover conexão Kafka, chamar NewServer simplificado
5. handlers.go   →  Implementar HandlePublish + HandlePublishBulk (HTTP)
6. style.css     →  Adicionar estilos do formulário
7. producer.js   →  Lógica frontend
8. producer.html →  Página HTML
9. index.html    →  Adicionar navegação
10. docker-compose.yml → Env var UI_PRODUCER_URL
11. handlers_test.go → Testes com mock HTTP server
```

## Checklist de Build

```bash
go build ./cmd/ui/          # deve compilar (sem sarama)
go vet ./internal/ui/       # sem warnings
go test ./internal/ui/      # todos os testes passam
```
