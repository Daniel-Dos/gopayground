# 01 — Contexto

## Contexto do Negócio

Este spec define um **consumer Kafka** responsável por processar eventos de pagamento
em um sistema distribuído de pagamentos. Cada evento representa uma transação
financeira que precisa ser validada, processada e persistida.

### Fluxo de Negócio

1. Uma **origem produtora** (ex: API de pagamentos, gateway externo) publica um
   evento no tópico Kafka `payment.events`.
2. O consumer Kafka lê a mensagem do tópico.
3. O payload é **validado** contra um schema esperado (payment_id, status, amount,
   currency, timestamp).
4. O **Idempotency Checker** verifica no Redis se o `payment_id` já foi processado.
5. Se já processado → acknowledge da mensagem (skip silencioso).
6. Se não processado → marca o `payment_id` no Redis com TTL de 24h.
7. O **status atual do pagamento** é atualizado no Redis (chave: `payment:<payment_id>`,
   TTL configurável).
8. O **histórico completo** da transação é registrado no DynamoDB (tabela
   `payment_history`).
9. Em caso de sucesso → commit da mensagem (mark).
10. Em caso de falha recuperável → **retry** com backoff exponencial (até 3
    tentativas: 100ms, 300ms, 900ms).
11. Em caso de falha definitiva após exaustão de retries → publicação em **DLQ**
    (tópico `payment.events.dlq`).

## Sistemas Envolvidos

| Sistema     | Função                                              |
|-------------|-----------------------------------------------------|
| **Kafka**   | Fonte de eventos (tópico `payment.events`)          |
| **Redis**   | Cache de idempotência + status atual do pagamento   |
| **DynamoDB**| Armazenamento permanente do histórico de transações |
| **DLQ**     | Tópico Kafka separado para mensagens com falha      |
| **OTel**    | Observabilidade (tracing distribuído + métricas)    |

## Estratégia de Idempotência

- `payment_id` é usado como **chave de idempotência**.
- Antes de processar, verifica-se no Redis a existência da chave
  `idempotency:<payment_id>`.
- Se a chave existe → mensagem já foi processada → acknowledge sem ação.
- Se a chave não existe → atomically cria a chave com SET NX + TTL de 24h.
- A chave expira automaticamente após 24h para evitar acúmulo.
- **Caso de borda**: se o Redis falhar, o consumer pode reprocessar a mesma
  mensagem → o DynamoDB deve aceitar registros duplicados (consistência eventual)
  ou rejeitar via chave composta.

## Estratégia de Retry

- Até **3 tentativas** por mensagem.
- Backoff exponencial: 1ª tentativa → 100ms, 2ª → 300ms, 3ª → 900ms.
- O retry é **síncrono** (bloqueia o worker até sucesso ou exaustão).
- Após 3 tentativas falhas → a mensagem é enviada para DLQ.

## Estratégia de DLQ

- Tópico dedicado: `payment.events.dlq`.
- A mensagem original é publicada **com headers** contendo:
  - `original_topic`: `payment.events`
  - `error_count`: número de tentativas realizadas
  - `last_error`: mensagem de erro da última tentativa
  - `timestamp`: timestamp do envio para DLQ
- Um worker separado (fora do escopo desta spec) deve processar a DLQ.

## Observabilidade

- **Logs estruturados** com `slog` em cada etapa: recebimento, validação,
  idempotência, persistência, erro.
- **Tracing distribuído** com OpenTelemetry: cada mensagem gera um span pai,
  com spans filhos para cada operação (Redis, DynamoDB).
- **Métricas** exportadas via OTel:
  - `payment.consumer.messages_received` (contador)
  - `payment.consumer.messages_processed` (contador, status=success|error)
  - `payment.consumer.processing_duration` (histograma)
  - `payment.consumer.retry_attempts` (contador)
  - `payment.consumer.dlq_published` (contador)
  - `payment.consumer.idempotency_hits` (contador)

## Considerações Importantes

- O consumer opera em **consumer group** (`payment-consumer-group`) para
  escalabilidade horizontal.
- O particionamento do Kafka é baseado em `payment_id` para garantir ordenação
  por pagamento.
- O número de workers (goroutines) é configurável via variável de ambiente.
- **Graceful shutdown** obrigatório: ao receber SIGTERM/SIGINT, o consumer deve
  finalizar o processamento das mensagens em andamento antes de sair.

---

## Não Escopo

- Processamento da DLQ (será coberto em spec futura).
- Interface administrativa para re-processamento manual.
- Notificações ao usuário final.
- Integração com gateway de pagamento externo.
