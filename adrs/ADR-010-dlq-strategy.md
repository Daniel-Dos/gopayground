# Architecture Decision Record (ADR)

**ADR ID**: ADR-010  
**Title**: Dead Letter Queue (DLQ) para Mensagens com Falha  
**Status**: Accepted  
**Date**: 2026-05-25  
**Owner**: Payment Platform Team  
**Deciders**: Architect, Senior Engineer  
**Scope**: Tratamento de mensagens que não podem ser processadas

---

## Core

### 1. Context

**Problem statement**: Mensagens no Kafka podem falhar permanentemente (payload inválido) ou transitoriamente (timeout Redis, DynamoDB down). Após exaustão do retry (ADR-004), mensagens com falha permanente precisam ser isoladas para análise sem bloquear o processamento de mensagens saudáveis.

**Goals**:
- Isolar mensagens com falha permanente em um tópico separado
- Preservar metadados da mensagem original (tópico, partição, offset, erro)
- Permitir reprocessamento manual das mensagens DLQ
- Diferenciar erros de validação (falha imediata) de erros de processamento (após retry)
- Incluir trace_id para rastreabilidade

**Non-goals**:
- Reprocessamento automático da DLQ
- Limpeza automática da DLQ (retenção configurável do Kafka)
- Dead Letter Queue para Redis ou DynamoDB
- Fila de retry separada (tentativas são síncronas)

**Constraints** (REQUIRED):
- Platform/runtime: Kafka via sarama, mesmo cluster do tópico principal
- Durabilidade: Mensagens DLQ persistem conforme retenção do Kafka
- Team/operational maturity: Inspeção manual via consumer CLI ou UI

**Assumptions**:
- DLQ é inspecionada manualmente (não automática)
- Tamanho da DLQ é pequeno (< 0.1% das mensagens)
- Tópico DLQ com 1 partição é suficiente

### 2. Decision Drivers (What Matters Most)

| Priority | Driver | Why it matters | How we measure |
|---:|---|---|---|
| 1 | Preservação de contexto | Saber por que falhou e de onde veio | Headers: original_topic, original_offset, last_error |
| 2 | Isolamento | Mensagens ruins não bloqueiam as boas | Tópico separado `payment.events.dlq` |
| 3 | Rastreabilidade | Correlacionar com tracing | Header trace_id |
| 4 | Simplicidade | Reusa SyncProducer da aplicação | ~85 linhas de código |

### 3. Options Considered

| Option | Summary | Pros | Cons | Reversibility |
|---|---|---|---|---|
| A - Kafka DLQ (escolhido) | Tópico `payment.events.dlq` | Mesmo broker, headers preservados, reuso de producer | Requer Kafka operacional, custo de armazenamento | Easy |
| B - Redis DLQ | Lista Redis com TTL | Redis já disponível, sem dependência extra | Perde-se restart (sem persistência), sem estrutura de headers | Easy |
| C - DynamoDB DLQ | Tabela separada para falhas | Persistente, consultável | Custo extra de escrita, sem replay automático | Medium |
| D - Log file DLQ | Arquivo JSON no disco | Zero dependências, simplicidade | Sem estrutura de re-processamento, sem indexação | Hard (perde contexto) |

### 4. Decision

**We choose**: Option A - Kafka DLQ

**Why**:
- Tópico `payment.events.dlq` (configurável via `kafka_dlq_topic`) definido em `config.go:19`
- Implementação `internal/dlq/producer.go` com ~85 linhas, reusa `sarama.SyncProducer`
- Headers preservam metadados completos da mensagem original:
  - `original_topic`, `original_partition`, `original_offset` — `dlq/producer.go:41-43`
  - `error_count` (3), `last_error`, `dlq_timestamp` — `dlq/producer.go:44-46`
  - `trace_id` do span OTel atual — `dlq/producer.go:48-52`
- Dois caminhos para DLQ:
  1. Validação falha → DLQ imediata com `reason: "validation"` — `handler.go:207-214`
  2. Retry exaustado → DLQ com `reason: "retry"` — `handler.go:287-299`
- Métrica `payment.consumer.dlq_published` com atributo `reason` (validation/retry) — `handler.go:105-111`

### 5. Architecture Impact (Implementation-Ready)

**Boundaries and contracts**
- Public APIs/contracts affected:
  - `dlq.Producer.Publish(ctx, msg, err)` — interface em `dlq/producer.go:19-21`
  - Tópico `payment.events.dlq` (1 partição, configurável)
  - Headers DLQ: `original_topic`, `original_partition`, `original_offset`, `error_count`, `last_error`, `dlq_timestamp`, `trace_id`
- Backward compatibility plan: N/A
- Schema evolution strategy: Headers adicionais podem ser incluídos sem quebrar consumidores existentes

**Data and consistency**
- Source of truth: DLQ é cópia da mensagem original + headers de erro
- Consistency model: At-most-once (se DLQ falhar, mensagem é perdida — log de erro)
- Migration strategy: N/A

**Failure modes and resilience**
- Known failure modes:
  - DLQ publish falha → log de erro em `handler.go:294-299`, mensagem original PERDIDA (sem fallback)
  - Contexto cancelado durante DLQ → retorna erro (`dlq/producer.go:77-78`)
  - sarama.SendMessage é blocking sem context → goroutine separada com canal (`dlq/producer.go:70-74`)
- Timeouts/retries/backoff policy: Sem retry para DLQ (se não conseguir publicar, loga e segue)
- Idempotency strategy: N/A
- Degradation plan: Se DLQ falha, mensagem é perdida (sistema continua processando)

**Security**
- Threat model summary: N/A
- AuthN/AuthZ model: N/A
- Secret and key management: N/A
- Audit logging requirements: DLQ inclui `last_error` e `dlq_timestamp` em headers

**Observability**
- SLIs/SLOs: DLQ rate < 0.1% de mensagens totais
- Metrics/traces/logs to add:
  - `payment.consumer.dlq_published` (counter com `reason` attribute) — `handler.go:105`
  - Logs: `"payload validation failed"` com offset e erro — `handler.go:203`
  - Logs: `"retry exhausted, publishing to DLQ"` com payment_id — `handler.go:290`
  - Logs: `"failed to publish to DLQ"` em caso de erro — `handler.go:210`
- Dashboards and alerts: Se dlq_published > 1% de messages_received, alertar

**Cost and capacity**
- Expected traffic/load: < 0.1% das mensagens, tipicamente 0-5 msg/dia
- Cost model: Armazenamento mínimo no Kafka
- Capacity plan: Retenção padrão do Kafka para tópico DLQ

### 6. Rollout, Validation, and Rollback

**Rollout plan**
- Feature flag / staged rollout: N/A
- Data migration steps: Tópico `payment.events.dlq` pode ser auto-criado (`KAFKA_AUTO_CREATE_TOPICS_ENABLE=true`)
- Runbook updates: Comando para consumir DLQ: `kafka-console-consumer --topic payment.events.dlq --from-beginning`

**Validation plan**
- Tests to add (unit/integration/contract):
  - `internal/dlq/producer_test.go` — testes do producer com mock
  - `internal/consumer/handler_test.go` — validação → DLQ, retry exaustão → DLQ
- Load/perf tests: N/A

**Rollback plan**
- How to revert code: `git revert`
- How to revert data (or forward-fix): Reprocessar mensagens da DLQ com script externo
- Timebox for rollback decision: 30 min

### 7. Consequences

**Positive**
- Mensagens com payload inválido vão imediatamente para DLQ (sem retry desnecessário) — `handler.go:201-216`
- Headers preservam contexto completo para debugging: offset original, erro, trace_id
- `trace_id` no header DLQ (`dlq/producer.go:48-52`) permite correlacionar com tracing OTel
- Goroutine separada com canal (`dlq/producer.go:70-74`) permite cancelamento via context (sarama.SendMessage é blocking)
- Métrica `dlq_published` com `reason` permite monitorar tipos de falha

**Negative / tradeoffs**
- Se DLQ publish falhar, mensagem é perdida (sem fallback) — log de erro apenas
- `error_count` é hardcoded como "3" (`dlq/producer.go:44`) — não reflete tentativas reais
- Goroutine no DLQ publish cria overhead para cada mensagem com falha
- DLQ é Kafka → se cluster Kafka estiver com problemas, DLQ também falha

**Follow-ups**
- Tornar `error_count` dinâmico baseado no número real de tentativas (Owner: Senior Engineer, Q3 2026)
- Implementar fallback para DLQ em Redis ou arquivo se Kafka estiver indisponível (Owner: Senior Engineer, Q4 2026)
- Criar script de reprocessamento da DLQ (Owner: DevOps, Q3 2026)

### 8. Links

- Código: `internal/dlq/producer.go` — implementação completa (85 linhas)
- Código: `internal/consumer/handler.go` — linhas 201-216 (validação → DLQ), 285-300 (retry → DLQ)
- Config: `kafka_dlq_topic: payment.events.dlq` (em `config.go:19`)
- Testes: `internal/dlq/producer_test.go`
- Relacionados: ADR-001 (Kafka), ADR-004 (retry), ADR-011 (OTel tracing)

---

## References

- ADR concept and template rationale: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
- Kafka Dead Letter Queue pattern: https://docs.confluent.io/platform/current/connect/userguide.html#dead-letter-queue
