# 07 — Hardening

## Resiliência

### Timeout na Chamada HTTP ao Producer Service

- Timeout total de 10s por chamada HTTP (`context.WithTimeout`)
- Timeout de conexão de 5s (`net.Dialer.Timeout`)
- Timeout de idle connection de 30s para reuso de conexões HTTP keep-alive

```go
// HTTP client configurado no NewHandlers
h.httpClient = &http.Client{
    Timeout: 10 * time.Second,
    Transport: &http.Transport{
        DialContext: (&net.Dialer{
            Timeout: 5 * time.Second,
        }).DialContext,
        MaxIdleConns:    10,
        IdleConnTimeout: 30 * time.Second,
    },
}

// Handler usa context com timeout
ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
defer cancel()
req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
resp, err := h.httpClient.Do(req)
```

### Producer Service Indisponível

- Se o Producer Service não responder (timeout, conexão recusada, DNS), o
  handler retorna 502 com mensagem clara
- Não há retry automático no handler (o usuário pode tentar novamente via frontend)
- A UI e a dashboard continuam funcionando normalmente (apenas publish afetado)

### Producer Service Retorna Erro

- Se o Producer Service retornar HTTP 4xx/5xx, a UI ecoa a resposta para o cliente
- O body de erro do Producer é retransmitido sem alteração
- Log estruturado de erro com `payment_id`, status code e resposta

### Payload Máximo

- `http.MaxBytesReader(w, r.Body, 100*1024)` — 100KB (antes de encaminhar ao Producer)
- Se excedido, Go retorna 413 automaticamente

## Concorrência

### Rate Limiting

Implementar rate limiter simples no handler:

```go
var rateLimitMap sync.Map

func rateLimitMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/api/publish" || r.URL.Path == "/api/publish/bulk" {
            ip := r.RemoteAddr
            if v, ok := rateLimitMap.Load(ip); ok {
                lastTime := v.(time.Time)
                if time.Since(lastTime) < 200*time.Millisecond {
                    writeError(w, http.StatusTooManyRequests, "too many requests")
                    return
                }
            }
            rateLimitMap.Store(ip, time.Now())
        }
        next.ServeHTTP(w, r)
    })
}
```

**Considerações**:
- Mapa simples sem cleanup (para ferramenta dev, uso limitado)
- Se necessário em produção, substituir por `golang.org/x/time/rate` Limiter
- Rate limit aplica apenas a `/api/publish*`

### Goroutine Safety

- `http.Client` do stdlib é thread-safe e reusável entre goroutines
- Cada request HTTP usa seu próprio `context.Context` (sem compartilhamento)
- Rate limit usa `sync.Map` (thread-safe)
- Nenhuma goroutine extra é criada no handler

## Observabilidade

### Logs Estruturados

```go
// Sucesso (eco da resposta do Producer)
h.logger.Info("event published via UI",
    "payment_id", event.PaymentID,
    "status", event.Status,
    "amount", event.Amount,
    "currency", event.Currency,
    "producer_status", resp.StatusCode,
    "remote_addr", r.RemoteAddr,
)

// Erro de conexão com Producer
h.logger.Error("producer service call failed",
    "payment_id", event.PaymentID,
    "producer_url", h.producerURL,
    "error", err,
    "remote_addr", r.RemoteAddr,
)

// Erro retornado pelo Producer
h.logger.Error("producer returned error",
    "payment_id", event.PaymentID,
    "producer_status", resp.StatusCode,
    "producer_body", string(respBody),
    "remote_addr", r.RemoteAddr,
)
```

### Métricas

Sugestão para versão futura (não implementar agora):
- `ui_publish_requests_total` (counter)
- `ui_publish_errors_total` (counter) — com label `error_type` (timeout, connection, producer_error)
- `ui_publish_duration_seconds` (histogram) — latência da chamada HTTP ao Producer

### Tracing

O rastreamento da origem do evento é feito pelo header `source: cli-producer`
que o Producer Service adiciona em todas as mensagens Kafka. A UI pode
adicionar headers de tracing (ex: `X-Request-ID`, `X-Trace-ID`) nas chamadas
HTTP ao Producer para correlação de logs.

## Segurança Operacional

### Validação de Payload

A UI faz uma validação estrutural básica (JSON bem formado, limites de
tamanho). A validação semântica completa (UUID, ISO 4217, campos
obrigatórios) é delegada ao Producer Service.

### Validação de Payload (Frontend)

Antes de enviar ao servidor:
- UUID v4 regex se preenchido
- Amount > 0
- Currency exatamente 3 letras
- Description < 255 caracteres

### Proteção contra Payload Grande

- `http.MaxBytesReader`: 100KB (proteção contra payload gigante na UI)
- Frontend limita description a 255 caracteres (maxlength)

### Headers de Segurança

Já aplicados pelo middleware existente (`securityHeadersMiddleware`):
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Referrer-Policy: no-referrer`

### CORS

Não necessário (mesma origem — frontend e backend servidos na mesma porta 8081).

### Secrets

- `UI_PRODUCER_URL` configurada via variável de ambiente
- Nenhuma senha ou token no código
- Redis password já configurada via env var (existente)

## Produção

### Graceful Shutdown

- Não há mais `kafkaProducer.Close()` no shutdown
- O shutdown existente (15s timeout) continua cobrindo o server HTTP
- Requests HTTP em andamento para o Producer Service são canceladas via
  contexto do request (`r.Context()`)

### Readiness / Liveness

- O endpoint `/healthz` já existente verifica Redis e DynamoDB
- Não verifica o Producer Service (publish não crítico para health check)
- Se Producer Service estiver down, o health check ainda retorna OK

### Comportamento sob Falha Parcial

| Falta                   | Impacto                                       | Comportamento Esperado          |
|-------------------------|-----------------------------------------------|---------------------------------|
| Producer Service down   | Publish retorna 502                           | UI continua, dashboard normal   |
| Producer timeout        | Publish retorna 502                           | UI continua, dashboard normal   |
| Producer retorna erro   | Erro ecoado ao cliente (4xx/5xx)              | UI continua, dashboard normal   |
| Redis down              | Dashboard sem SSE, publish ainda funciona     | Publish via Producer ainda OK   |
| DynamoDB down           | Histórico indisponível, publish funciona      | Publish via Producer ainda OK   |

### Rollback

Se necessário reverter para Kafka embutido:
1. Reverter `config.go` (remover `ProducerURL`)
2. Reverter `server.go` (voltar `NewServer` com `sarama.SyncProducer`)
3. Reverter `handlers.go` (voltar `HandlePublish` com `sarama`)
4. Reverter `main.go` (voltar conexão Kafka)
5. Reverter `docker-compose.yml` (voltar `KAFKA_BROKERS` no `payment-ui`)
6. Remover `producer.html`, `producer.js` se não desejados
7. Build e deploy
