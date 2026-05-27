# 07 — Hardening

## 1. Resiliência

### 1.1 Timeouts Explícitos

Toda operação de I/O da UI deve ter timeout explícito:

| Operação                    | Timeout | Justificativa                             |
|-----------------------------|---------|-------------------------------------------|
| Redis Ping (health)         | 2s      | Verificação rápida de conectividade       |
| Redis SCAN (list payments)  | 5s      | Pode ter muitas chaves                    |
| Redis HGETALL               | 5s      | Leitura simples                           |
| Redis Pub/Sub (publish)     | 1s      | Não bloquear o consumer                   |
| DynamoDB Query (history)    | 10s     | Consulta com possível paginação           |
| HTTP ReadTimeout            | 10s     | Tempo máximo para ler request             |
| HTTP WriteTimeout           | 30s     | SSE connections precisam de timeout longo |

**Implementação**:

```go
// Cada handler cria contexto com timeout derivado
func (h *Handlers) handleListPayments(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()
    // ...
}

func (h *Handlers) handlePaymentHistory(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    // ...
}
```

O `r.Context()` já é cancelado quando o cliente desconecta, garantindo que
goroutines não fiquem presas.

### 1.2 Retry para Operações de Leitura

Operações de leitura (Redis, DynamoDB) não devem ter retry explícito — se
falharem, retornam erro para a UI. A UI (frontend) pode tentar novamente
com backoff simples (ex: 1s, 2s, 4s — máximo 3 tentativas).

```javascript
async function fetchWithRetry(url, options, maxRetries = 3) {
    for (let i = 0; i < maxRetries; i++) {
        try {
            const resp = await fetch(url, options);
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            return await resp.json();
        } catch (err) {
            if (i === maxRetries - 1) throw err;
            await new Promise(r => setTimeout(r, 1000 * Math.pow(2, i)));
        }
    }
}
```

### 1.3 Circuit Breaker

Não implementar circuit breaker na UI (é uma ferramenta de leitura/debug).
Se Redis ou DynamoDB estão fora, a UI mostra erro e o operador sabe que o
sistema downstream está com problema.

### 1.4 DLQ

A UI não publica em DLQ. Ela apenas lê dados existentes. O tópico DLQ é
gerenciado pelo consumer (spec 0001).

### 1.5 Graceful Shutdown

**Sequência**:

```
1. SIGTERM/SIGINT recebido
2. context.WithTimeout(15s) criado para shutdown
3. EventBus.Close() → fecha todos os subscribers
4. http.Server.Shutdown() → para de aceitar conexões, aguarda handlers ativos
5. SSE handlers recebem r.Context().Done() e finalizam
6. Redis client close (se aplicável)
7. Se timeout expirar, http.Server.Close() força fechamento
8. Processo exit(0)
```

**Implementação**:

```go
func main() {
    // ...
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        <-sigCh
        logger.Info("shutting down UI server...")
        ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer cancel()
        if err := server.Shutdown(ctx); err != nil {
            logger.Error("shutdown error", "error", err)
        }
    }()

    if err := server.Start(); err != nil && err != http.ErrServerClosed {
        logger.Error("server error", "error", err)
        os.Exit(1)
    }
}
```

---

## 2. Concorrência

### 2.1 Event Bus Thread-Safe

O `EventBus` deve ser seguro para uso concorrente:

```go
type EventBus struct {
    mu          sync.RWMutex
    subscribers map[string]chan *models.PaymentEvent
    // ...
}

func (eb *EventBus) Subscribe() (<-chan *models.PaymentEvent, func()) {
    eb.mu.Lock()
    defer eb.mu.Unlock()
    // ...
}

func (eb *EventBus) unsubscribe(id string) {
    eb.mu.Lock()
    defer eb.mu.Unlock()
    // ...
}
```

### 2.2 Publicação Não Bloqueante

A publicação no Event Bus não deve bloquear o publisher (consumer):

```go
// listenRedis distribui eventos para subscribers sem bloquear
select {
case sub <- event:
default:
    // Subscriber lento — descarta
    eb.logger.Warn("dropping event for slow subscriber", "subscriber_id", id)
}
```

### 2.3 Conexões SSE Simultâneas

Cada conexão SSE cria uma goroutine e um subscriber no Event Bus. O limite
de conexões simultâneas é controlado pelo HTTP server (default
`http.Server` não tem limite — aplicar `net.Listener` com semáforo se
necessário).

**Proteção**: Adicionar semáforo para limitar conexões SSE simultâneas:

```go
var sseSemaphore = make(chan struct{}, 100) // max 100 SSE connections

func (h *Handlers) handleSSE(w http.ResponseWriter, r *http.Request) {
    select {
    case sseSemaphore <- struct{}{}:
        defer func() { <-sseSemaphore }()
    default:
        http.Error(w, "too many connections", http.StatusServiceUnavailable)
        return
    }
    // ...
}
```

### 2.4 Proteção contra Goroutine Leaks

- **SSE handlers**: o `r.Context().Done()` garante que a goroutine é
  finalizada quando o cliente desconecta.
- **Event Bus listenRedis**: goroutine única, finalizada no `Close()`.
- **HTTP server**: `Shutdown` aguarda handlers ativos finalizarem.
- **Sem goroutines explícitas nos handlers**: handlers são síncronos.

### 2.5 Race Conditions

- **Subscribers map**: protegido por `sync.RWMutex`.
- **Request data**: cada request HTTP é isolado (sem estado compartilhado).
- **Frontend**: JavaScript single-threaded (event loop) — sem race conditions.

---

## 3. Observabilidade

### 3.1 Logs Estruturados (slog)

Formato: JSON, sempre com `time`, `level`, `msg`, `service`.

| Evento                        | Level | Campos Adicionais                                     |
|-------------------------------|-------|-------------------------------------------------------|
| Servidor iniciado             | INFO  | `port`                                                 |
| Shutdown iniciado             | INFO  | —                                                     |
| Request HTTP                  | INFO  | `method`, `path`, `status`, `duration`                |
| SSE cliente conectado         | DEBUG | `remote_addr`                                         |
| SSE cliente desconectado      | DEBUG | `remote_addr`                                         |
| Evento publicado no Redis     | DEBUG | `payment_id`                                           |
| Evento distribuído p/ subscriber| DEBUG | `subscriber_id`, `payment_id`                        |
| Evento descartado (buffer cheio) | WARN | `subscriber_id`                                      |
| Redis scan error              | ERROR | `error`                                                |
| Redis hgetall error           | WARN  | `key`, `error`                                         |
| DynamoDB query error          | ERROR | `payment_id`, `error`                                  |
| DynamoDB unmarshal error      | WARN  | `payment_id`, `error`                                  |
| Health check falhou           | WARN  | `component` (redis)                                    |
| Semáforo SSE cheio            | WARN  | `max_connections`                                      |

**Exemplo**:
```json
{
    "time": "2026-05-24T10:30:00.123Z",
    "level": "INFO",
    "msg": "request",
    "service": "payment-ui",
    "method": "GET",
    "path": "/api/payments",
    "status": 200,
    "duration": "45ms"
}
```

### 3.2 Métricas

A UI não exporta métricas via OTel (é uma ferramenta auxiliar). Métricas do
sistema de pagamentos são expostas pelo consumer (spec 0001).

Métricas da UI podem ser expostas via endpoint `/api/metrics` (já definido
nos requisitos), que reflete métricas do Redis/DynamoDB — não métricas do
próprio servidor.

### 3.3 Tracing

A UI não gera tracing distribuído. Ela apenas exibe o `trace_id` dos eventos
registrados no DynamoDB. Se houver necessidade futura de tracing na UI,
pode-se adicionar OTel middleware.

### 3.4 Health Check Endpoint

| Endpoint   | Porta | Comportamento                                           |
|------------|-------|---------------------------------------------------------|
| `/healthz` | 8081  | Retorna 200 se Redis responde ping.                     |
|            |       | Retorna 503 se Redis está fora.                         |

Não há `/readyz` (a UI não tem dependências lentas — Redis é verificado no
healthz).

### 3.5 Monitoramento de Conexões SSE

Adicionar métrica interna (contador) para número de conexões SSE ativas:

```go
var (
    sseConnections    int64
    sseTotalConnections int64
)

// No handler SSE:
atomic.AddInt64(&sseConnections, 1)
defer atomic.AddInt64(&sseConnections, -1)
atomic.AddInt64(&sseTotalConnections, 1)
```

Estes valores podem ser expostos em `/api/metrics` ou logados
periodicamente.

---

## 4. Segurança Operacional

### 4.1 Gerenciamento de Secrets

- **Senha do Redis**: lida via variável de ambiente `REDIS_PASSWORD`.
- **AWS credentials**: gerenciadas via SDK default credential chain.
- **Nunca hardcoded**: nenhum secret no código fonte.
- **Não logar**: senhas, tokens, ou dados sensíveis.

### 4.2 Payload Validation

- **Entrada da API**: query params `payment_id` e `status` são strings
  simples. Validar tamanho máximo (payment_id: 64 chars, status: 16 chars).
- **Path parameter**: `payment_id` validado (não vazio, sem caracteres de
  controle).
- **Saída da API**: dados do Redis/DynamoDB são serializados como JSON.
  Amount é float64 (formatar no frontend para 2 casas decimais).
- **SSE data**: eventos são JSON válidos do modelo `PaymentEvent`.

### 4.3 Tratamento de Erro Seguro

```go
// Erros de API: mensagens genéricas, sem expor detalhes internos
func writeError(w http.ResponseWriter, status int, message string) {
    writeJSON(w, status, map[string]string{"error": message})
}

// Uso:
writeError(w, http.StatusInternalServerError, "failed to fetch payments")
// Não: "redis scan error: connection refused"
```

### 4.4 Path Traversal

O `embed.FS` é seguro contra path traversal — arquivos fora do diretório
embutido não podem ser acessados. O `http.FileServer` com `embed.FS` não
serve arquivos fora do FS virtual. Isso protege tanto os assets da UI
quanto a documentação embutida em `static/docs/`.

### 4.5 Documentação Estática

A documentação servida em `/docs/` consiste exclusivamente de arquivos
estáticos (HTML, imagens, diagramas) pré-gerados durante o build. Não há
processamento server-side (templates, SSR, includes) — cada requisição
retorna o arquivo diretamente do `embed.FS`. Isso elimina riscos de
injeção server-side via documentação.

**Considerações de cache**:
- Assets de documentação podem ser cacheados pelo navegador (ETag/Last-Modified
  são gerenciados pelo `http.FileServer`)
- Não definir `Cache-Control` agressivo para HTMLs de documentação (para
  permitir atualizações rápidas em dev)
- Imagens e diagramas podem usar cache mais longo (1h+)

### 4.6 Headers de Segurança

Adicionar headers de segurança nas respostas HTTP:

```go
func securityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("Referrer-Policy", "no-referrer")
        next.ServeHTTP(w, r)
    })
}
```

### 4.7 CORS

Não necessário — frontend e backend servidos na mesma origem (mesma porta).

---

## 5. Produção

### 5.1 Configuração Recomendada

| Parâmetro                    | Desenvolvimento | Produção (se aplicável) | Justificativa              |
|------------------------------|-----------------|-------------------------|----------------------------|
| `UI_PORT`                    | 8081            | 8081                    | Porta padrão               |
| `UI_EVENT_BUS_BUFFER`        | 256             | 512                     | Throughput maior           |
| `UI_READ_TIMEOUT`            | 10s             | 5s                      | Segurança                  |
| `UI_WRITE_TIMEOUT`           | 30s             | 60s                     | SSE precisa de timeout longo|
| `REDIS_ADDR`                 | localhost:6379  | cluster endpoint        | Produção usa cluster       |
| Log level                    | DEBUG           | INFO                    | Volume de logs             |

### 5.2 Comportamento sob Falha Parcial

| Cenário                      | Comportamento Esperado                                    |
|------------------------------|-----------------------------------------------------------|
| Redis indisponível           | Payments list e metrics retornam erro 500. Health check 503. |
| DynamoDB indisponível        | Histórico retorna erro 500. Demais funcionalidades ok.    |
| Consumer não está rodando    | UI funciona (dados estáticos do Redis/DynamoDB). SSE vazio. |
| Kafka indisponível           | UI funciona (DLQ count retorna 0). SSE vazio.             |
| Alta latência de Redis       | Timeout de 5s, request falha, UI mostra erro amigável.    |
| Navegador desconecta         | SSE handler finaliza via `r.Context().Done()`.             |

### 5.3 Rollback Seguro

- **Static files**: embutidos no binário — rollback = deploy da versão
  anterior.
- **API**: compatível com versões anteriores do Redis/DynamoDB (schema
  existente).
- **Models**: compartilhados com o consumer — mudanças requerem coordenação.

### 5.4 Dependências Externas

| Dependência | Versão Mínima | Justificativa                                |
|-------------|---------------|----------------------------------------------|
| Go          | 1.22          | `http.ServeMux` com method patterns, `embed` |
| go-redis    | 9.5.1         | Já existente no projeto                      |
| AWS SDK v2  | 1.26.0        | Já existente no projeto                      |

### 5.5 Considerações de Deployment

- **Single binary**: copiar binário para o container/imagem.
- **Porta**: `8081` não conflita com consumer (porta `8080` para health).
- **Network**: em docker-compose, UI deve estar na mesma network que Redis
  e DynamoDB.
- **Recursos**: CPU request 100m, limit 200m. Memória request 32Mi, limit
  64Mi. (UI é leve.)

---

## 6. Plano de Testes de Hardening

### 6.1 Testes de Caos

| Teste                          | Descrição                                                  |
|--------------------------------|------------------------------------------------------------|
| Redis failover                 | Matar Redis → UI deve mostrar erro nos endpoints.          |
| DynamoDB timeout               | Simular latência alta no DynamoDB → timeout respeitado.    |
| Conexões SSE simultâneas       | 100 clients SSE conectam simultaneamente → sem crash.      |
| Cliente SSE lento              | Cliente que não consome eventos → buffer enche, eventos    |
|                                | descartados, publisher não bloqueia.                       |
| Carga alta de eventos          | 1000 eventos/segundo publicados → buffer enche, descarte.  |

### 6.2 Testes de Longa Duração

- Executar UI por 24h com carga constante de eventos (10 eventos/s).
- Verificar:
  - Sem crescimento de memória.
  - Sem goroutine leaks (número de goroutines estável).
  - Conexões SSE estáveis (sem queda inesperada).

### 6.3 Testes de Estresse

| Cenário                        | Carga            | Duração | Métrica Alvo              |
|--------------------------------|------------------|---------|---------------------------|
| Carga normal SSE               | 10 eventos/s     | 1h      | Sem perda de eventos      |
| Pico de eventos                | 500 eventos/s    | 5 min   | Buffer enche, sem crash   |
| 50 connections SSE             | 50 clients       | 10 min  | Todas recebem eventos     |
| Múltiplos requests API         | 100 req/s        | 5 min   | p99 < 500ms               |

---

## 7. Checklist de Hardening

### Resiliência
- [ ] Timeouts explícitos para Redis (5s) e DynamoDB (10s).
- [ ] HTTP ReadTimeout (10s) e WriteTimeout (30s).
- [ ] SSE handlers finalizam quando cliente desconecta (`r.Context().Done()`).
- [ ] EventBus com buffer e política de descarte (non-blocking publish).
- [ ] Graceful shutdown com timeout de 15s.
- [ ] Frontend com retry nas chamadas de API (3 tentativas, backoff).

### Concorrência
- [ ] EventBus thread-safe (sync.RWMutex para mapa de subscribers).
- [ ] Semáforo para limitar conexões SSE simultâneas (100).
- [ ] Publicação não bloqueante no EventBus (select com default).
- [ ] Testes com `-race` passando.
- [ ] Sem goroutines sem contexto de cancelamento.

### Observabilidade
- [ ] Logs estruturados (slog, JSON) com campos padronizados.
- [ ] Logging de todas as requisições HTTP (method, path, status, duration).
- [ ] Logging de eventos descartados (buffer cheio).
- [ ] Health check endpoint (`/healthz`).
- [ ] Contagem de conexões SSE ativas (exposta ou logada).

### Segurança
- [ ] Path traversal prevenido via embed.FS (assets e documentação).
- [ ] Headers de segurança: X-Content-Type-Options, X-Frame-Options, Referrer-Policy.
- [ ] Erros de API genéricos (sem expor detalhes internos).
- [ ] Validação de entrada: tamanho máximo de query params.
- [ ] Sem autenticação documentado (ferramenta dev, não expor publicamente).
- [ ] Documentação estática (sem processamento server-side, sem injeção via docs).
- [ ] Documentação acessível em `/docs/` sem expor arquivos fora do diretório embutido.

### Produção
- [ ] Configuração recomendada documentada.
- [ ] Comportamento sob falha parcial documentado.
- [ ] Rollback seguro (compatibilidade de schema).
- [ ] Dockerfile multi-stage leve (< 50 MB).
- [ ] Resource limits definidos (CPU/Memória).

---
