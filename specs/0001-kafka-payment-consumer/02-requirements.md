# 02 — Requisitos

## Requisitos Funcionais

| ID     | Descrição                                                                                     | Prioridade |
|--------|-----------------------------------------------------------------------------------------------|------------|
| RF-001 | Consumir eventos do tópico Kafka `payment.events`                                             | Alta       |
| RF-002 | Validar o schema do payload: `payment_id` (string), `status` (string), `amount` (float64),    | Alta       |
|        | `currency` (string, ISO 4217), `timestamp` (RFC3339)                                          |            |
| RF-003 | Persistir o status atual do pagamento no Redis com chave `payment:<payment_id>` e TTL         | Alta       |
| RF-004 | Persistir o histórico completo da transação no DynamoDB (tabela `payment_history`)             | Alta       |
| RF-005 | Garantir idempotência via Redis com chave `idempotency:<payment_id>` e TTL de 24h             | Alta       |
| RF-006 | Suportar retry com backoff exponencial até 3 tentativas                                       | Alta       |
| RF-007 | Publicar em DLQ (`payment.events.dlq`) após falha permanente após exaustão de retries         | Alta       |
| RF-008 | Realizar logging estruturado em todos os estágios (slog)                                      | Média      |
| RF-009 | Suportar graceful shutdown com drain completo das mensagens em andamento                     | Alta       |
| RF-010 | Expor métricas de processamento via OpenTelemetry                                             | Média      |
| RF-011 | Propagar tracing distribuído com correlation ID                                               | Média      |

### Schema do Payload (RF-002)

```json
{
  "payment_id":   "string",     // UUID v4 — obrigatório
  "status":       "string",     // pending | confirmed | failed | refunded — obrigatório
  "amount":       "number",     // float64, > 0 — obrigatório
  "currency":     "string",     // ISO 4217 (BRL, USD, EUR) — obrigatório
  "description":  "string",     // opcional, max 255 chars
  "timestamp":    "string"      // RFC3339 — obrigatório
}
```

### Regras de Validação (RF-002)

- `payment_id`: deve ser UUID v4 válido.
- `status`: deve ser um dos valores enumerados: `pending`, `confirmed`, `failed`, `refunded`.
- `amount`: deve ser número finito positivo (> 0).
- `currency`: deve seguir ISO 4217 (3 letras maiúsculas).
- `timestamp`: deve ser string RFC3339 válida, não pode ser data futura (> 5 min de skew).
- `description`: string opcional, máx 255 caracteres, sem caracteres de controle.

---

## Requisitos Não Funcionais

| ID      | Descrição                                                                               | Métrica        | Alvo              |
|---------|-----------------------------------------------------------------------------------------|----------------|-------------------|
| RNF-001 | Throughput mínimo sustentado de 1.000 mensagens por segundo em condições normais        | msg/s          | >= 1.000 msg/s    |
| RNF-002 | Latência p99 de processamento (Kafka → DynamoDB) menor que 100ms                        | ms             | p99 < 100ms       |
| RNF-003 | Idempotência com TTL de 24h no Redis                                                    | TTL            | 24h               |
| RNF-004 | Consumer group (`payment-consumer-group`) para escalabilidade horizontal                | N consumers    | >= 2              |
| RNF-005 | Graceful shutdown com timeout máximo de 30 segundos para drain                          | segundos       | 30s               |
| RNF-006 | Disponibilidade do componente: >= 99.9%                                                  | uptime         | >= 99.9%          |
| RNF-007 | Tolerância a falhas de Redis (circuit breaker + fallback para idempotência otimista)    |                |                   |
| RNF-008 | Tolerância a falhas de DynamoDB (retry + fallback para status apenas em Redis)          |                |                   |
| RNF-009 | Utilização máxima de CPU < 70% em pico de 1.000 msg/s                                   | % CPU          | < 70%             |
| RNF-010 | Utilização máxima de memória < 512 MB por instância                                     | MB             | < 512 MB          |

---

## Restrições Técnicas

| ID  | Restrição                                              | Justificativa                                    |
|-----|--------------------------------------------------------|--------------------------------------------------|
| R01 | Linguagem: Go 1.22+                                    | Stack definida no projeto                        |
| R02 | Kafka client: `github.com/IBM/sarama`                  | Biblioteca madura, suporte a consumer groups     |
| R03 | Redis client: `github.com/redis/go-redis/v9`            | Padrão da indústria para Go                      |
| R04 | DynamoDB client: `github.com/aws/aws-sdk-go-v2`        | SDK oficial AWS                                  |
| R05 | Validação: `github.com/go-playground/validator/v10`     | Validação declarativa de structs                 |
| R06 | Tracing: `go.opentelemetry.io/otel`                     | Padrão para observabilidade distribuída          |
| R07 | Logging: `log/slog` (stdlib)                            | Log estruturado nativo do Go                     |
| R08 | Testes: `testing` padrão + `github.com/stretchr/testify`| Asserts expressivos                              |
| R09 | Kafka ambiente: tópico `payment.events` com 3 partições | Escalabilidade e paralelismo                     |
| R10 | Tópico DLQ: `payment.events.dlq` com 1 partição         | Simplicidade, sem exigência de ordenação         |

---

## Histórico de Revisão

| Versão | Data       | Autor     | Descrição           |
|--------|------------|-----------|---------------------|
| 1.0    | 2026-05-24 | Architect | Versão inicial      |
