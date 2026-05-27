# 07 — Hardening

## 1. Resiliência

### 1.1 Retries com Backoff Exponencial

O **Retry Handler** é responsável por executar a função de processamento com
retry em caso de falha. A configuração padrão é:

| Tentativa | Delay Base | Jitter (±25%) | Delay Efetivo     |
|-----------|------------|---------------|-------------------|
| 1ª        | 0ms        | 0ms           | 0ms (imediato)    |
| 2ª        | 100ms      | 75–125ms      | 75–125ms          |
| 3ª        | 300ms      | 225–375ms     | 225–375ms         |

**Progressão real**: 1x, 3x, 9x o delay base de 100ms.

```go
delay := baseDelay * time.Duration(1 << (attempt - 1)) // 1x, 2x, 4x
delay = addJitter(delay, 0.25)
```

**Jitter**: aplicado em todas as tentativas exceto a primeira. O jitter é
aditivo (±25%) para evitar sincronização de retries (thundering herd).

**Timeout total**: não deve exceder 30s por mensagem. O contexto da sessão
Kafka tem `GRACEFUL_SHUTDOWN_TIMEOUT` como deadline.

**Comportamento**: se o contexto for cancelado durante o backoff, o retry
aborta imediatamente (sem esperar o delay restante).

### 1.2 Timeouts Explícitos

Toda operação de I/O deve ter timeout explícito:

| Operação              | Timeout | Justificativa                          |
|-----------------------|---------|----------------------------------------|
| Redis `IsProcessed`   | 5s      | Leitura simples, deve ser rápida        |
| Redis `MarkProcessed` | 5s      | Escrita simples, deve ser rápida        |
| Redis `UpdateStatus`  | 5s      | Pipeline, tolerância maior              |
| DynamoDB `PutItem`    | 10s     | Operação de escrita, latência variável  |
| DLQ `SendMessage`     | 10s     | Publicação Kafka, latência variável     |
| Processamento total   | 30s     | Soma de todas as operações + retries    |

**Implementação**:

```go
func (h *Handler) processMessage(ctx context.Context, msg *sarama.ConsumerMessage) error {
    // Cada operação usa context.WithTimeout derivado do ctx pai
    // para garantir timeouts individuais sem cancelar o pai.
    
    // Exemplo:
    checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    processed, err := h.idempotency.IsProcessed(checkCtx, event.PaymentID)
    // ...
}
```

### 1.3 Circuit Breaker para DynamoDB

O DynamoDB é o ponto mais vulnerável a degradação. Deve ser protegido com
circuit breaker.

**Estratégia**: Implementação simples baseada em contagem de erros consecutivos.

| Estado      | Condição                        | Ação                                   |
|-------------|---------------------------------|----------------------------------------|
| Fechado     | Erros consecutivos < 5          | Chamadas normais                        |
| Aberto      | 5+ erros consecutivos           | Rejeita chamadas imediatamente (fail fast) |
| Meio-Aberto | Após 30s no estado aberto       | Permite 1 chamada de teste             |

**Implementação**: Usar `github.com/sony/gobreaker` ou implementação própria.

**Ao abrir o circuit breaker**:
- Log crítico com alerta.
- Retry não é mais tentado (mensagem vai direto para DLQ após `IsProcessed`).
- Métrica `circuit_breaker_open` é setada para 1.

**Ao fechar o circuit breaker**:
- Log info de recuperação.
- Métrica `circuit_breaker_open` é setada para 0.

### 1.4 DLQ (Dead Letter Queue)

Tópico dedicado `payment.events.dlq` com 1 partição.

**Quando publicar na DLQ**:
1. Payload inválido (erro de validação) — erro permanente, sem retry.
2. Exaustão de retries após 3 tentativas.
3. Circuit breaker aberto (não há retry, vai direto).

**Headers obrigatórios**:
```go
[]sarama.RecordHeader{
    {Key: "original_topic",       Value: "payment.events"},
    {Key: "original_partition",   Value: "0"},
    {Key: "original_offset",      Value: "12345"},
    {Key: "error_count",          Value: "3"},
    {Key: "last_error",           Value: "dynamodb timeout: context deadline exceeded"},
    {Key: "dlq_timestamp",        Value: "2026-05-24T10:30:00Z"},
    {Key: "trace_id",             Value: "abc123..."},
}
```

**Consideração de falha da DLQ**: Se o DLQ Producer falha (Kafka broker
indisponível), a mensagem é perdida. Um log crítico é emitido. Não há
bloqueio do consumer para evitar parada total do pipeline.

**Monitoramento**: Alerta quando `dlq_published` > 0 em intervalo de 5 min.

### 1.5 Graceful Shutdown

**Sequência**:

```
1. SIGTERM/SIGINT recebido
2. ConsumerGroup.Close() é chamado
3. Sarama sinaliza session.Close() para handlers ativos
4. Handler.ConsumeClaim() recebe session.Context().Done()
5. Workers em andamento têm GRACEFUL_SHUTDOWN_TIMEOUT (30s) para finalizar
6. Se timeout expirar, processo termina (mensagens não finalizadas são
   rebalanceadas para outro consumer no grupo)
7. Redis client close
8. DynamoDB client close (stateless)
9. Kafka sync producer close (flush pendentes)
10. OTel TracerProvider.Shutdown()
11. OTel MeterProvider.Shutdown()
12. Processo exit(0)
```

**Implementação**:

```go
func main() {
    // ...
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        <-sigCh
        logger.Info("shutting down gracefully...")

        shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.GracefulShutdownTimeout)
        defer cancel()

        // 1. Parar consumer group
        if err := consumerGroup.Close(); err != nil {
            logger.Error("consumer group close error", "error", err)
        }

        // 2. Fechar dependências
        rdb.Close()
        syncProducer.Close()
        tp.Shutdown(shutdownCtx)
        mp.Shutdown(shutdownCtx)

        os.Exit(0)
    }()

    // Loop de consumo
    for {
        if err := consumerGroup.Consume(ctx, topics, handler); err != nil {
            logger.Error("consume error", "error", err)
        }
        if ctx.Err() != nil {
            break
        }
    }
}
```

---

## 2. Concorrência

### 2.1 Worker Pool Configurável

O número máximo de mensagens processadas simultaneamente é controlado por um
semáforo baseado em `chan struct{}` com buffer.

```go
type Handler struct {
    semaphore chan struct{}
    // ...
}

func NewHandler(..., workerCount int) *Handler {
    return &Handler{
        semaphore: make(chan struct{}, workerCount),
        // ...
    }
}

func (h *Handler) processMessage(ctx context.Context, msg *sarama.ConsumerMessage) error {
    // Adquire semáforo
    select {
    case h.semaphore <- struct{}{}:
    case <-ctx.Done():
        return ctx.Err()
    }
    defer func() { <-h.semaphore }()

    // ... processamento
}
```

**Valor default**: `WORKER_COUNT=10`.

**Regra de dimensionamento**: `worker_count = 2 * número_de_partições` é um
bom ponto de partida. Para 3 partições, 6-10 workers.

### 2.2 Contexto com Cancelamento

Toda operação de I/O deve aceitar `context.Context`. O contexto raiz para cada
mensagem é derivado de `session.Context()`:

```go
func (h *Handler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
    for msg := range claim.Messages() {
        // Deriva contexto da sessão
        msgCtx := session.Context()
        
        // Adiciona tracing
        msgCtx, span := h.tracer.Start(msgCtx, "process_message",
            trace.WithAttributes(
                attribute.String("payment_id", /* extrair do payload */),
                attribute.Int64("offset", msg.Offset),
                attribute.String("partition", strconv.Itoa(int(msg.Partition))),
            ),
        )

        err := h.processMessage(msgCtx, msg)
        span.End()

        if err != nil {
            // Não commita → mensagem será reentregue
            h.logger.ErrorContext(msgCtx, "message processing failed", "error", err)
        } else {
            session.MarkMessage(msg, "")
        }
    }
    return nil
}
```

### 2.3 Proteção contra Goroutine Leaks

- **Uso de `sync.WaitGroup`**: para rastrear goroutines ativas no shutdown.
- **Semáforo**: garante que não mais que `WORKER_COUNT` goroutines estejam
  ativas simultaneamente.
- **Contexto monitorado**: goroutines escutam `ctx.Done()` e abortam.
- **`defer` em toda goroutine**: garante liberação de recursos mesmo em panic.

### 2.4 Race Conditions

- **Idempotência atômica**: `SET NX` no Redis é atômico. Duas goroutines
  concorrentes com mesmo `payment_id`: apenas uma vence. A segunda recebe
  `false` e retorna erro (que é tratado como "já processado" no handler).
- **DynamoDB ConditionalExpression**: `attribute_not_exists(payment_id)` evita
  duplicatas mesmo em escrita concorrente.
- **Sem mutex compartilhado**: cada mensagem é independente. Não há estado
  compartilhado entre goroutines.
- **Logging**: `slog` é thread-safe.

---

## 3. Observabilidade

### 3.1 Logs Estruturados (slog)

Formato: JSON, sempre com `time`, `level`, `msg`, `service`.

**Eventos obrigatórios com campos mínimos**:

| Evento                        | Level | Campos Adicionais                                               |
|-------------------------------|-------|-----------------------------------------------------------------|
| Mensagem recebida do Kafka    | DEBUG | `payment_id`, `offset`, `partition`, `trace_id`                 |
| Payload validado              | DEBUG | `payment_id`, `trace_id`                                        |
| Payload inválido              | WARN  | `payment_id`, `error`, `trace_id`                               |
| Idempotency hit (já processado)| INFO | `payment_id`, `trace_id`                                        |
| Idempotency check falhou      | WARN  | `payment_id`, `error`, `trace_id` (fallback otimista)           |
| Marcação de idempotência      | DEBUG | `payment_id`, `trace_id`                                        |
| Status atualizado no Redis    | DEBUG | `payment_id`, `status`, `trace_id`                              |
| Histórico registrado DynamoDB | DEBUG | `payment_id`, `trace_id`                                        |
| Retry tentativa               | WARN  | `payment_id`, `attempt`, `max_attempts`, `error`, `trace_id`    |
| Retry exaurido                | ERROR | `payment_id`, `attempts`, `last_error`, `trace_id`              |
| DLG publicado                 | ERROR | `payment_id`, `original_offset`, `last_error`, `trace_id`       |
| DLQ falhou                    | CRIT  | `payment_id`, `error`, `trace_id`                               |
| Graceful shutdown             | INFO  | `timeout`                                                       |
| Consumer group rebalance      | INFO  | `group_id`, `partition`                                         |

**Exemplo de log**:
```json
{
  "time": "2026-05-24T10:30:00.123Z",
  "level": "WARN",
  "msg": "retry attempt failed",
  "service": "payment-consumer",
  "payment_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "attempt": 2,
  "max_attempts": 3,
  "error": "dynamodb put error: operation timeout",
  "trace_id": "0af7651916cd43dd8448eb211c80319c"
}
```

**Proibido logar**:
- Valor completo de `amount` (pode logar mascarado: `***`).
- Número completo de cartão de crédito (não faz parte do schema, mas por
  segurança, qualquer campo que pareça sensível).
- Senhas, tokens, secrets.

### 3.2 Tracing Distribuído (OpenTelemetry)

**Span hierarchy por mensagem**:

```
process_message (painel)
├── validator.validate
├── idempotency.is_processed
├── idempotency.mark_processed
├── retry.do
│   ├── [tentativa 1]
│   │   ├── status.update
│   │   └── history.record
│   ├── [tentativa 2]
│   │   ├── status.update
│   │   └── history.record
│   └── [tentativa 3]
│       ├── status.update
│       └── history.record
└── dlq.publish
```

**Atributos obrigatórios em spans**:
- `payment_id`
- `messaging.system`: `kafka`
- `messaging.destination`: `payment.events`
- `messaging.kafka.partition`
- `messaging.kafka.offset`
- `messaging.consumer_group`: `payment-consumer-group`

**Sampling**: 100% em desenvolvimento, 10% em produção (configurável).

### 3.3 Métricas (OpenTelemetry)

**Métricas custom**:

| Nome                                        | Tipo        | Atributos                    | Descrição                              |
|---------------------------------------------|-------------|------------------------------|----------------------------------------|
| `payment.consumer.messages_received`        | Counter     | `partition`                  | Total de mensagens recebidas            |
| `payment.consumer.messages_processed`       | Counter     | `status` (success/error)     | Total de mensagens processadas          |
| `payment.consumer.processing_duration`      | Histogram   | `status`, `has_retry`        | Duração do processamento (ms)           |
| `payment.consumer.retry_attempts`           | Counter     | `attempt`                    | Total de tentativas de retry            |
| `payment.consumer.dlq_published`            | Counter     | `reason` (validation/retry)  | Total de mensagens publicadas na DLQ    |
| `payment.consumer.idempotency_hits`         | Counter     | —                            | Total de mensagens duplicadas ignoradas |
| `payment.consumer.circuit_breaker_open`     | Gauge       | `target` (dynamodb/redis)    | Estado do circuit breaker (0/1)         |
| `payment.consumer.worker_pool_usage`        | Gauge       | —                            | Workers ocupados / total                |
| `payment.consumer.kafka_lag`                | Gauge       | `partition`                  | Lag do consumer group (se exposto)      |

**Buckets do histograma `processing_duration`**: `[1, 5, 10, 25, 50, 100, 250, 500, 1000]` ms.

### 3.4 Health Check Endpoints

Dois endpoints HTTP expostos para Kubernetes (ou orquestrador):

| Endpoint        | Porta | Comportamento                                              |
|-----------------|-------|------------------------------------------------------------|
| `/healthz`      | 8080  | Retorna 200 se o consumer está rodando e Kafka conectado.  |
| `/readyz`       | 8080  | Retorna 200 se o consumer está apto a receber tráfego      |
|                 |       | (consumer group registrado, Redis ok, DynamoDB ok).        |

**Implementação**:

```go
func healthHandler(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
    // Verifica Redis
    if err := rdb.Ping(r.Context()).Err(); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]string{"status": "redis_down"})
        return
    }
    // Verifica DynamoDB (describe table)
    if _, err := dynamoClient.DescribeTable(r.Context(), &dynamodb.DescribeTableInput{
        TableName: aws.String(cfg.DynamoDBTable),
    }); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]string{"status": "dynamodb_down"})
        return
    }
    json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

---

## 4. Segurança Operacional

### 4.1 Gerenciamento de Secrets

- **Senha do Redis**: lida via variável de ambiente `REDIS_PASSWORD`.
- **AWS credentials**: gerenciadas via SDK default credential chain (IAM roles,
  env vars, ~/.aws/credentials).
- **Nunca hardcoded**: nenhum secret no código fonte.
- **Produção**: usar AWS Secrets Manager ou Kubernetes Secrets injetados como
  env vars.

### 4.2 Payload Validation contra Injeção

- `go-playground/validator` valida tipos e formatos.
- Campos string são limitados em tamanho e charset (ASCII, sem caracteres de
  controle).
- Tamanho máximo do payload: 10 KB (rejeitar mensagens maiores antes do
  parsing).
- `json.Unmarshal` usa `json.Decoder` com `DisallowUnknownFields`? **Decisão**:
  **não** usar. Permitir campos extras para tolerância evolutiva do schema.

```go
// Limitar tamanho do payload
const maxPayloadSize = 10 * 1024 // 10 KB

func (pv *paymentValidator) Validate(ctx context.Context, data []byte) (*models.PaymentEvent, error) {
    if len(data) > maxPayloadSize {
        return nil, fmt.Errorf("payload too large: %d bytes (max %d)", len(data), maxPayloadSize)
    }
    // ...
}
```

### 4.3 Tratamento de Erro Seguro

- Erros são logados com contexto mas **sem expor detalhes internos**.
- Erros retornados para componentes superiores são **encadeados** (`%w`)
  mas sanitizados antes de log externo.
- Erro de validação: retorna apenas "validation error", sem expor schema
  interno.

### 4.4 Isolation Level

- Consumer group `payment-consumer-group` isola o consumo entre instâncias.
- Cada mensagem é processada em contexto isolado.
- Sem estado compartilhado entre mensagens (exceto Redis/DynamoDB).

---

## 5. Produção

### 5.1 Configuração Recomendada para Produção

| Parâmetro                    | Produção      | Desenvolvimento | Justificativa                      |
|------------------------------|---------------|-----------------|------------------------------------|
| `WORKER_COUNT`               | 20            | 5               | Throughput                          |
| `RETRY_MAX_ATTEMPTS`         | 3             | 2               | Latência vs resiliência             |
| `IDEMPOTENCY_TTL_HOURS`      | 24            | 1               | Cobertura                           |
| `STATUS_TTL_HOURS`           | 168 (7 dias)  | 1               | Janela de consulta                  |
| `GRACEFUL_SHUTDOWN_TIMEOUT`  | 60s           | 10s             | Drain seguro                        |
| OTel sampler                 | ParentBased   | AlwaysOn        | Custo vs observabilidade            |
| Log level                    | INFO          | DEBUG           | Volume de logs                      |
| Consumo de CPU request/limit | 500m/1000m    | —               | Kubernetes resource management      |
| Memória request/limit        | 256Mi/512Mi   | —               | Kubernetes resource management      |
| Réplicas mínimas             | 2             | 1               | Alta disponibilidade                |

### 5.2 Comportamento sob Falha Parcial

| Cenário                         | Comportamento Esperado                                           |
|---------------------------------|------------------------------------------------------------------|
| 1 broker Kafka cai              | Consumer reconecta para brokers restantes.                        |
| 2+ brokers Kafka caem           | Consumer pode ficar indisponível. Aplicação reporta erro.         |
| Redis indisponível              | Idempotência falha (fallback otimista). Status não atualizado.    |
| DynamoDB indisponível           | Retry até exaustão → DLQ. Status no Redis atualizado.            |
| Consumer reinicia               | Consumer group rebalanceia. Mensagens em processamento são        |
|                                 | reentregues para outro consumer.                                  |
| Todas as réplicas reiniciam     | Kafka mantém mensagens. Quando consumer voltar, retoma do        |
|                                 | último offset commitado.                                          |

### 5.3 Rollback Seguro

- **Sem migrations de schema**: DynamoDB é schemaless. Tabela `payment_history`
  aceita novos atributos sem alteração.
- **Rollback de código**: basta reverter o deployment. A versão anterior pode
  ler mensagens com campos extras (ignorados pelo Unmarshal).
- **Compatibility**: o contrato de mensagem é evolutivo (adição de campos).
  Remoção de campos requer coordenação entre produtor e consumidor.

### 5.4 Dependências Externas

| Dependência | Versão Mínima | Justificativa                                |
|-------------|---------------|----------------------------------------------|
| Go          | 1.22          | range-over-func, melhorias de performance    |
| Sarama      | 1.43.0        | Suporte a Kafka 3.6, consumer groups         |
| go-redis    | 9.5.1         | Redis 7 compatível, pipeline, cluster        |
| AWS SDK v2  | 1.26.0        | DynamoDB, config, credenciais                |
| OTel        | 1.26.0        | Tracing, métricas, SDK estável               |
| Validator   | 10.19.0       | Validação declarativa, custom validators     |
| Testify     | 1.9.0         | Asserts, mocks                               |
| Gobreaker   | 2.1.0         | Circuit breaker (opcional)                   |

---

## 6. Plano de Testes de Hardening

### 6.1 Testes de Caos

| Teste                          | Descrição                                                      |
|--------------------------------|----------------------------------------------------------------|
| Kafka broker kill              | Matar 1 broker → consumer deve reconectar.                     |
| Redis failover                 | Simular failover do Redis → consumer deve operar sem idempotência. |
| DynamoDB throttling            | Simular throttling (ProvisionedThroughputExceeded) → retry funciona. |
| Rede instável                  | Simular latência de rede (50ms–500ms) → timeouts respeitados.  |
| Partição Kafka offline         | Remover líder de 1 partição → consumer rebalanceia.            |

### 6.2 Testes de Longa Duração

- Executar consumer por 24h com carga constante de 500 msg/s.
- Verificar:
  - Sem crescimento de memória.
  - Sem goroutine leaks.
  - Throughput estável.
  - Latência p99 estável < 100ms.
  - Logs sem erros inesperados.

### 6.3 Testes de Estresse

| Cenário                        | Carga          | Duração | Métrica Alvo           |
|--------------------------------|----------------|---------|------------------------|
| Carga normal                   | 1.000 msg/s    | 1h      | p99 < 100ms            |
| Pico                            | 5.000 msg/s    | 5 min   | p99 < 500ms            |
| Carga + falha DynamoDB         | 1.000 msg/s    | 5 min   | DLQ alimentado corretamente |
| Carga + falha Redis            | 1.000 msg/s    | 5 min   | Fallback otimista ativo |

---

## 7. Checklist de Hardening

### Resiliência
- [ ] Retry com backoff exponencial implementado com jitter.
- [ ] Timeouts explícitos para Redis (5s) e DynamoDB (10s).
- [ ] Circuit breaker para DynamoDB implementado.
- [ ] DLQ com tópico dedicado e headers informativos.
- [ ] Graceful shutdown com drain de mensagens e timeout configurável.
- [ ] Kafka consumer configurado com `Consumer.Retry.Backoff`.

### Concorrência
- [ ] Semáforo com buffer limitando workers simultâneos.
- [ ] Contexto com cancelamento sendo propagado para todas as operações.
- [ ] `sync.WaitGroup` para rastrear goroutines no shutdown.
- [ ] Testes com `-race` passando.
- [ ] `defer` em toda goroutine para liberação de recursos.

### Observabilidade
- [ ] Logs estruturados (slog, formato JSON) com campos padronizados.
- [ ] Tracing distribuído (OTel) com spans para cada operação.
- [ ] Métricas custom exportadas via OTel.
- [ ] Health check endpoints `/healthz` e `/readyz`.
- [ ] Métrica de lag do Kafka exposta (ou logada).

### Segurança
- [ ] Payload com limite de tamanho (10 KB).
- [ ] Validação contra injeção (charset, caracteres de controle).
- [ ] Logs sem dados sensíveis.
- [ ] Secrets via variáveis de ambiente ou secret manager.
- [ ] Tratamento de erro seguro (sem expor detalhes internos).

### Produção
- [ ] Configuração recomendada para produção documentada.
- [ ] Comportamento sob falha parcial documentado.
- [ ] Rollback seguro (compatibilidade de schema).
- [ ] Dockerfile multi-stage.
- [ ] Resource limits (CPU/memória) configurados.
