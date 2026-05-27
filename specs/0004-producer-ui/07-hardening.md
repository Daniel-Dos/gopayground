# 07 — Hardening

## Resiliência

### Timeout na Publicação Kafka

- `sarama.Config.Producer.Timeout = 10 * time.Second`
- `sarama.Config.Net.DialTimeout = 5 * time.Second`
- Context com timeout de 10s no handler `HandlePublish`

```go
kafkaCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
defer cancel()
msg := &sarama.ProducerMessage{...}
partition, offset, err := h.kafkaProducer.SendMessage(msg)
```

### Kafka Indisponível

- Se `kafkaProducer` for `nil` (falha na conexão inicial), todos os handlers de
  publish retornam 502 imediatamente
- Não há retry no handler (o usuário pode tentar novamente via frontend)
- A UI e a dashboard continuam funcionando normalmente (apenas publish afetado)

### EventBus Indisponível

- Publicação no EventBus é feita com timeout curto (2s) via `context.WithTimeout`
- Se falhar, apenas log.Warn + continua (evento já foi para o Kafka)
- O consumer vai processar e eventualmente o evento aparecerá na dashboard

### Payload Máximo

- `http.MaxBytesReader(w, r.Body, 100*1024)` — 100KB
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

- `sarama.SyncProducer` é thread-safe (documentado)
- `EventBus.Publish` usa Redis Pub/Sub (thread-safe)
- Rate limit usa `sync.Map` (thread-safe)
- Nenhuma goroutine extra é criada no handler

## Observabilidade

### Logs Estruturados

```go
// Sucesso
h.logger.Info("event published via UI",
    "payment_id", event.PaymentID,
    "status", event.Status,
    "amount", event.Amount,
    "currency", event.Currency,
    "partition", partition,
    "offset", offset,
    "remote_addr", r.RemoteAddr,
)

// Erro Kafka
h.logger.Error("kafka publish failed",
    "payment_id", event.PaymentID,
    "error", err,
    "remote_addr", r.RemoteAddr,
)

// Erro EventBus (não crítico)
h.logger.Warn("eventbus publish failed after kafka success",
    "payment_id", event.PaymentID,
    "error", pubErr,
)
```

### Métricas

Sugestão para versão futura (não implementar agora):
- `producer_events_published_total` (counter)
- `producer_events_failed_total` (counter)
- `producer_publish_duration_seconds` (histogram)

### Tracing

O header `source: producer-ui` já é adicionado nas mensagens Kafka,
permitindo rastrear a origem do evento no consumer.

## Segurança Operacional

### Validação de Payload (Backend)

Reutilizar o `validator.Validator` existente que valida:
- `payment_id`: obrigatório, UUID v4
- `status`: obrigatório, one of (pending, confirmed, failed, refunded)
- `amount`: obrigatório, > 0
- `currency`: obrigatório, len 3, uppercase
- `description`: opcional, max 255, printable ASCII
- `timestamp`: obrigatório, RFC3339

### Validação de Payload (Frontend)

Antes de enviar ao servidor:
- UUID v4 regex se preenchido
- Amount > 0
- Currency exatamente 3 letras
- Description < 255 caracteres

### Proteção contra Payload Grande

- `http.MaxBytesReader`: 100KB
- Frontend limita description a 255 caracteres (maxlength)
- Validação rejeita description > 255 antes de chegar ao Kafka

### Headers de Segurança

Já aplicados pelo middleware existente (`securityHeadersMiddleware`):
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Referrer-Policy: no-referrer`

### CORS

Não necessário (mesma origem — frontend e backend servidos na mesma porta 8081).

### Secrets

- Kafka brokers e tópico configurados via variáveis de ambiente
- Nenhuma senha ou token no código
- Redis password já configurada via env var (existente)

## Produção

### Graceful Shutdown

- O `kafkaProducer.Close()` é chamado no defer do `main()`
- Chamado antes do `server.Shutdown()` garantir que novas publicações parem
- O shutdown já existente (15s timeout) cobre o fechamento do producer

### Readiness / Liveness

- O endpoint `/healthz` já existente verifica Redis
- Em versão futura, pode-se adicionar verificação de conectividade Kafka
- Se Kafka estiver down, o health check ainda retorna OK (apenas publish afetado)

### Comportamento sob Falha Parcial

| Falta            | Impacto                                      | Comportamento Esperado          |
|------------------|----------------------------------------------|---------------------------------|
| Kafka down       | Publish retorna 502                          | UI continua, dashboard normal   |
| Redis down       | EventBus fallha (warning), Kafka OK          | Dashboard sem SSE, publish OK   |
| DynamoDB down    | Nenhum (não usado no publish)                | Normal                          |
| Kafka + Redis    | Publish retorna 502                          | UI operacional, sem publish     |

### Rollback

Se necessário reverter:
1. Reverter alterações em `server.go`, `handlers.go`, `main.go`
2. Remover `producer.html`, `producer.js`
3. Reverter `index.html` (remover link de navegação)
4. Reverter `docker-compose.yml`
5. Build e deploy
