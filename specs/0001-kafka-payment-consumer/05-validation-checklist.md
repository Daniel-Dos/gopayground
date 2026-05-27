# 05 — Checklist de Validação

## Instruções

Este checklist deve ser executado pelo **Senior Engineer** durante a implementação
e validado pelo **Hardening Engineer** antes da liberação para produção.

---

## 1. Testes Unitários por Pacote

### 1.1 Models (`internal/models/`)

- [ ] `PaymentEvent` é serializado/deserializado corretamente de/para JSON.
- [ ] `PaymentHistory` é gerado a partir de `PaymentEvent` com todos os campos.
- [ ] `PaymentHistory.ProcessedAt` é preenchido com UTC.
- [ ] `PaymentHistory.TraceID` é preenchido (mesmo que vazio, sem panic).

### 1.2 Config (`config/`)

- [ ] Valores default são aplicados quando env vars não estão setadas.
- [ ] Valores de env vars sobrescrevem defaults.
- [ ] Timeout default é 30s.
- [ ] Worker count default é 10.

### 1.3 Validator (`internal/validator/`)

- [ ] Payload JSON válido é aceito e retorna `*PaymentEvent` correto.
- [ ] Payload com `payment_id` inválido (não UUID) → erro.
- [ ] Payload com `status` inválido (`invalid_status`) → erro.
- [ ] Payload com `amount` = 0 → erro.
- [ ] Payload com `amount` negativo → erro.
- [ ] Payload com `currency` de 2 letras → erro.
- [ ] Payload com `currency` minúsculo (`brl`) → erro (deve ser uppercase).
- [ ] Payload com `timestamp` em formato inválido → erro.
- [ ] Payload com `timestamp` futuro (> 5 min de skew) → erro.
- [ ] Payload com `description` > 255 caracteres → erro.
- [ ] Payload com `description` contendo caracteres de controle → erro.
- [ ] Payload com campos extras (ignorados) → aceito.
- [ ] JSON mal formatado → erro de unmarshal.
- [ ] Payload vazio (`{}`) → erro de campos required.

### 1.4 Idempotency (`internal/idempotency/`)

- [ ] `IsProcessed` retorna `false` para chave inexistente.
- [ ] `IsProcessed` retorna `true` após `MarkProcessed`.
- [ ] `MarkProcessed` retorna erro se chave já existe (concorrência).
- [ ] `MarkProcessed` define TTL correto (24h por default).
- [ ] Contexto cancelado → erro propagado.
- [ ] Redis client retorna erro → erro propagado.

### 1.5 Status Updater (`internal/status/`)

- [ ] `UpdateStatus` cria chave `payment:<id>` no Redis.
- [ ] `UpdateStatus` armazena `payment_id`, `status`, `updated_at`.
- [ ] `UpdateStatus` sobrescreve status quando chamado novamente.
- [ ] TTL é configurado corretamente.
- [ ] Operação usa pipeline (atomicidade).
- [ ] Contexto cancelado → erro propagado.

### 1.6 History Recorder (`internal/history/`)

- [ ] `RecordHistory` insere item no DynamoDB com todos os atributos.
- [ ] `RecordHistory` preenche `processed_at` com UTC.
- [ ] `RecordHistory` preenche `trace_id` do span atual.
- [ ] `RecordHistory` não retorna erro se item já existe (ConditionalCheckFailed).
- [ ] `RecordHistory` retorna erro se DynamoDB está indisponível.
- [ ] `RecordHistory` retorna erro se contexto é cancelado.

### 1.7 Retry Handler (`internal/retry/`)

- [ ] Sucesso na primeira tentativa → retorna sem delay.
- [ ] Sucesso na segunda tentativa → executa 1 delay.
- [ ] Sucesso na terceira tentativa → executa 2 delays.
- [ ] Falha em todas tentativas → retorna `ErrExhausted`.
- [ ] Backoff segue progressão: 100ms, 300ms, 900ms (com jitter).
- [ ] Jitter não produz delay negativo ou zero.
- [ ] Contexto cancelado durante backoff → retorna `context.Canceled` imediatamente.
- [ ] Se `maxAttempts=1`, executa exatamente 1 vez sem retry.

### 1.8 DLQ Producer (`internal/dlq/`)

- [ ] Mensagem publicada no tópico DLQ correto.
- [ ] Headers originais são preservados.
- [ ] Headers adicionais estão presentes: `original_topic`, `error_count`,
      `last_error`, `dlq_timestamp`.
- [ ] Key original é preservada.
- [ ] Erro do Kafka producer é propagado.
- [ ] Payload original (Value) não é alterado.

### 1.9 Consumer Handler (`internal/consumer/`)

- [ ] Mensagem válida → chama validator → idempotency → status → history.
- [ ] Mensagem duplicada → idempotency skip → commit → não chama status/history.
- [ ] Payload inválido → DLQ → não chama idempotency/status/history.
- [ ] Falha de status/history → retry → se exaurir → DLQ.
- [ ] Todas métricas são registradas (`messagesReceived`, `messagesProcessed`, etc).
- [ ] Span é criado para cada mensagem com atributo `payment_id`.
- [ ] Semáforo respeita limite de workers configurado.
- [ ] Graceful shutdown: `ConsumeClaim` retorna quando session context é cancelado.

### 1.10 Telemetry (`pkg/telemetry/`)

- [ ] TracerProvider é inicializado sem erro.
- [ ] MeterProvider é inicializado sem erro.
- [ ] Shutdown não retorna erro.
- [ ] Providers registrados como global.

---

## 2. Testes de Integração

### 2.1 Kafka → Consumer

- [ ] Mensagem publicada no Kafka é consumida e processada.
- [ ] Mensagem com payload inválido vai para DLQ.
- [ ] Mensagem duplicada é ignorada (idempotência).
- [ ] Consumer group rebalanceamento não causa perda de mensagens.
- [ ] Consumo com 2+ consumers em paralelo.

### 2.2 Redis

- [ ] Idempotency key expira após TTL.
- [ ] Status key é criada com TTL.
- [ ] Redis reinicia durante operação → consumer se recupera.

### 2.3 DynamoDB

- [ ] Histórico é persistido com sucesso.
- [ ] Item duplicado (mesmo `payment_id` + `timestamp`) não gera erro.
- [ ] DynamoDB Local funciona para testes.

---

## 3. Testes de Concorrência

### 3.1 Race Detector

- [ ] Todos os testes passam com `-race` flag.
- [ ] Nenhum data race em acesso compartilhado.

### 3.2 Worker Pool

- [ ] Semáforo limita número máximo de goroutines simultâneas.
- [ ] Liberação correta do semáforo após processamento (defer).
- [ ] Não há goroutine leaks após shutdown.

### 3.3 Idempotência Concorrente

- [ ] Duas goroutines processando mesmo `payment_id` simultaneamente:
      apenas uma vence o `SET NX`.

---

## 4. Testes de Graceful Shutdown

### 4.1 Sinal SIGTERM

- [ ] Consumer para de consumir novas mensagens.
- [ ] Mensagens em andamento são finalizadas (dentro do timeout).
- [ ] Mensagens não finalizadas são rebalanceadas para outro consumer.
- [ ] Nenhuma mensagem é perdida no shutdown.

### 4.2 Timeout

- [ ] Se shutdown excede 30s, processo termina forçadamente.
- [ ] Log de aviso é emitido quando shutdown está demorando.

---

## 5. Testes de Falha

### 5.1 Falha do Kafka Broker

- [ ] Consumer tenta reconexão automática.
- [ ] Mensagens não são perdidas durante desconexão.
- [ ] Log de erro é emitido.

### 5.2 Falha do Redis

- [ ] `IsProcessed` falha → fallback otimista (processa sem idempotência).
- [ ] `MarkProcessed` falha → log de warning, continua.
- [ ] `UpdateStatus` falha → log de erro, continua (não bloqueante).
- [ ] Mensagem não vai para DLQ por falha do Redis (a menos que DynamoDB também falhe).

### 5.3 Falha do DynamoDB

- [ ] `RecordHistory` falha → retry é acionado.
- [ ] Se retry exaurir → mensagem vai para DLQ.
- [ ] Se Redis está ok e DynamoDB falha → status no Redis é atualizado (inconsistência parcial).

### 5.4 Falha do DLQ Producer

- [ ] Se DLQ falha, erro é logado, mensagem é perdida (ack).
- [ ] Decisão consciente: perder mensagem vs travar o consumer.

---

## 6. Testes de Throughput e Performance

### 6.1 Throughput

- [ ] 1.000 mensagens/s sustentado com workers=10, Redis ok, DynamoDB ok.
- [ ] 2.000 mensagens/s com workers=20.
- [ ] Mensagens com falha não degradam throughput geral.

### 6.2 Latência

- [ ] p50 < 20ms.
- [ ] p99 < 100ms.
- [ ] p999 < 500ms.

### 6.3 Consumo de Recursos

- [ ] CPU < 70% em pico de 1.000 msg/s.
- [ ] Memória < 512 MB.
- [ ] Número de goroutines estável (não cresce indefinidamente).

---

## 7. Testes de Observabilidade

### 7.1 Logs

- [ ] Logs são JSON estruturados.
- [ ] Toda mensagem processada tem log com `payment_id`, `trace_id`, `status`.
- [ ] Erros têm stack trace ou causa.
- [ ] DLQ publish tem log com `original_offset`, `last_error`.
- [ ] Logs não contêm dados sensíveis (amount completo, description).

### 7.2 Tracing

- [ ] Cada mensagem tem span com `payment_id` como atributo.
- [ ] Operações Redis e DynamoDB são spans filhos.
- [ ] Retry gera spans de tentativa (events).
- [ ] DLQ publish tem span separado.

### 7.3 Métricas

- [ ] `payment.consumer.messages_received` incrementa.
- [ ] `payment.consumer.messages_processed` com atributo `status`.
- [ ] `payment.consumer.processing_duration` com buckets adequados.
- [ ] `payment.consumer.retry_attempts` incrementa por tentativa.
- [ ] `payment.consumer.dlq_published` incrementa.
- [ ] `payment.consumer.idempotency_hits` incrementa.

---

## 8. Testes de Segurança

### 8.1 Payload Validation

- [ ] Injeção de script no campo `description` é rejeitada.
- [ ] Injeção de comando no campo `payment_id` é rejeitada.
- [ ] JSON com tamanho excessivo (> 10 KB) é rejeitado.

### 8.2 Configuração

- [ ] Senha do Redis não aparece em log.
- [ ] Endpoint de conexões não aparece em log de erro.
- [ ] Variáveis de ambiente são lidas de forma segura.
- [ ] Secrets não estão hardcoded.

---

## 9. Regressão

- [ ] Todos os testes unitários passam.
- [ ] Todos os testes de integração passam.
- [ ] Nenhum teste quebrado após refatoração.
- [ ] Cobertura de código > 80%.

---

## Resumo para Aprovação

| Área               | Status | Observações |
|--------------------|--------|-------------|
| Unit Tests         | [ ]    |             |
| Integration Tests  | [ ]    |             |
| Concorrência       | [ ]    |             |
| Graceful Shutdown  | [ ]    |             |
| Falhas             | [ ]    |             |
| Performance        | [ ]    |             |
| Observabilidade    | [ ]    |             |
| Segurança          | [ ]    |             |
| Regressão          | [ ]    |             |

**Assinatura do Hardening Engineer:** ____________________

**Data da validação:** ____________________
