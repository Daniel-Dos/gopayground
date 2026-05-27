# Architecture Decision Record (ADR)

**ADR ID**: ADR-003  
**Title**: DynamoDB para Armazenamento de Histórico de Pagamentos  
**Status**: Accepted  
**Date**: 2026-05-25  
**Owner**: Payment Platform Team  
**Deciders**: Architect, Senior Engineer  
**Scope**: Armazenamento permanente de histórico de pagamentos

---

## Core

### 1. Context

**Problem statement**: O sistema precisa de um armazenamento permanente e escalável para histórico completo de pagamentos processados. Cada evento de pagamento deve ser registrado com todos os seus metadados, incluindo trace_id para rastreabilidade distribuída. O histórico precisa ser consultável por payment_id e ordenado por timestamp.

**Goals**:
- Armazenar cada evento de pagamento sem perda (append-only)
- Consultar histórico completo de um payment_id com latência < 100ms
- Garantir que um mesmo evento não seja registrado duplicado (idempotência na escrita)
- Suportar escalabilidade horizontal sem intervenção manual
- Funcionar localmente em desenvolvimento sem AWS real

**Non-goals**:
- Consultas agregadas (count, sum) — cobertas pelo Redis (ADR-002)
- Atualização de registros existentes (apenas insert)
- Transações com Redis ou Kafka
- Backup/restore gerenciado

**Constraints** (REQUIRED):
- Latency/SLO: Consulta de histórico < 100ms, escrita < 200ms
- Platform/runtime: DynamoDB local via Floci (emulador), DynamoDB gerenciado em produção
- Data residency: N/A
- Cost: DynamoDB on-demand em produção, Floci gratuito em dev

**Assumptions**:
- Floci (emulador DynamoDB) é suficiente para desenvolvimento e testes
- Tabela `payment_history` criada manualmente (não via aplicação)
- Sem necessidade de índices secundários (apenas PK + SK)

### 2. Decision Drivers (What Matters Most)

| Priority | Driver | Why it matters | How we measure |
|---:|---|---|---|
| 1 | Durabilidade | Histórico não pode ser perdido | DynamoDB gerenciado = 99.999% durabilidade |
| 2 | Latência de consulta | UI precisa carregar histórico rápido | P99 < 100ms para Query por PK |
| 3 | Simplicidade operacional | Sem gerenciamento de servidores | DynamoDB serverless, Floci em dev |
| 4 | Custo desenvolvimento | Sem custo AWS em dev | Floci em Docker, sem conta AWS |

### 3. Options Considered

| Option | Summary | Pros | Cons | Reversibility |
|---|---|---|---|---|
| A - DynamoDB + Floci | Tabela NoSQL, SDK AWS v2, emulador local | Serverless, escalável, `ConditionExpression` nativa, Floci mock | Sem joins, sem queries complexas, aprendizado SDK | Medium |
| B - PostgreSQL | Relacional com índice | Consultas complexas, transações, familiaridade | Precisa de servidor dedicado, schema migrations, mais caro em produção | Hard |
| C - MongoDB | Document store, schema flexível | Schema dinâmico, queries ricas, escalável | Dependência extra não gerenciada, replica set complexo | Medium |
| D - Redis (persistente) | Redis com AOF/RDB | Já temos Redis, baixa latência | Persistência limitada, sem query, sem índices secundários | Medium |

### 4. Decision

**We choose**: Option A - DynamoDB + Floci

**Why**:
- `ConditionExpression: "attribute_not_exists(payment_id) AND attribute_not_exists(#ts)"` em `internal/history/dynamodb.go:53` garante idempotência na escrita — mesmo evento não é registrado duas vezes
- Modelo `PaymentHistory` com `payment_id` (PK) + `timestamp` (SK) em `internal/models/payment.go:22-32` permite query eficiente por payment_id
- Floci (`floci/floci:latest`) emula DynamoDB localmente sem conta AWS, com `FLOCI_STORAGE_MODE=memory`
- SDK AWS v2 (`github.com/aws/aws-sdk-go-v2`) é maduro, bem tipado, com suporte a contexto e tracing
- Tabela `payment_history` tem schema definido via struct `PaymentHistory` com tags `dynamodbav`

### 5. Architecture Impact (Implementation-Ready)

**Boundaries and contracts**
- Public APIs/contracts affected:
  - `internal/history.Recorder` interface: `RecordHistory(ctx, event) error`
  - `DynamoDBPutItemAPI` interface: `PutItem(ctx, params)` — definida em `dynamodb.go:18-20`
  - Tabela `payment_history` com `payment_id` (String, PK) e `timestamp` (String, SK)
  - Rota `GET /api/payments/{id}/history` na UI — `internal/ui/handlers.go:194`
- Backward compatibility plan: N/A (primeira implementação)
- Schema evolution strategy: Adicionar campos opcionais no struct; DynamoDB é schemaless

**Data and consistency**
- Source of truth: DynamoDB (histórico permanente)
- Consistency model: Eventual (escrita via PutItem com ConditionExpression)
- Migration strategy: N/A (primeira implementação)

**Failure modes and resilience**
- Known failure modes:
  - DynamoDB (Floci) indisponível → consumer loga erro e retenta (via retry handler)
  - `ConditionalCheckFailedException` → evento já existe, retorna nil (não é erro) — `dynamodb.go:59-62`
  - Floci com storage mode `memory` → dados perdidos em restart
- Timeouts/retries/backoff policy: Timeout de 10s para operações de history (`handler.go:275`)
- Idempotency strategy: `ConditionExpression` evita duplicatas mesmo se retry reexecutar `RecordHistory`
- Degradation plan: Se DynamoDB falha, consumer retenta até exaustão → DLQ

**Security**
- Threat model summary: Floci sem autenticação, DynamoDB produção usa IAM roles
- AuthN/AuthZ model: AWS SDK carrega credenciais via `awsconfig.LoadDefaultConfig`
- Secret and key management: N/A em dev (Floci), IAM roles em produção
- Audit logging requirements: Cada registro inclui `trace_id` do OTel para rastreabilidade

**Observability**
- SLIs/SLOs: DynamoDB PutItem latency < 200ms, Query latency < 100ms
- Metrics/traces/logs to add:
  - Logs de erro em `handler.go:278` para falha de `RecordHistory`
  - Trace spans com atributo `payment_id` propagado até a escrita
- Dashboards and alerts: N/A

**Cost and capacity**
- Expected traffic/load: ~1-10k registros/dia (cada evento = 1 registro)
- Cost model: Floci gratuito (dev), DynamoDB on-demand ~$0.25/mês para 10k registros
- Capacity plan: DynamoDB auto-scaling em produção

### 6. Rollout, Validation, and Rollback

**Rollout plan**
- Feature flag / staged rollout: N/A
- Data migration steps: Tabela `payment_history` deve existir (criada via AWS CLI ou Floci init)
- Runbook updates: Verificar `dynamodb_endpoint` (Floci em dev, DynamoDB URL em produção)

**Validation plan**
- Tests to add (unit/integration/contract):
  - `internal/history/dynamodb_test.go` — testes com mock DynamoDB
  - `internal/ui/handlers_test.go` — testes HTTP para `GET /api/payments/{id}/history`
- Load/perf tests: N/A

**Rollback plan**
- How to revert code: `git revert`
- How to revert data (or forward-fix): Remover registros duplicados manualmente via `DeleteItem`
- Timebox for rollback decision: 30 min

### 7. Consequences

**Positive**
- `ConditionExpression` em `dynamodb.go:53` previne duplicatas no histórico mesmo com reprocessamento
- `trace_id` armazenado em cada registro (`dynamodb.go:41`) permite rastreabilidade ponta-a-ponta
- UI consulta histórico com `KeyConditionExpression: "payment_id = :pid"` em `handlers.go:213` — latência < 50ms
- Floci permite desenvolver sem AWS real — `docker-compose.yml:30-41`

**Negative / tradeoffs**
- Escrever em DynamoDB é I/O-bound e pode ser o gargalo em alta vazão
- `FLOCI_STORAGE_MODE=memory` perde dados em restart (aceitável em dev)
- Sem query por status ou data no DynamoDB (apenas por payment_id)
- `attribute_not_exists` protege contra duplicatas mas impede atualizações no mesmo registro

**Follow-ups**
- Provisionar tabela DynamoDB em produção com auto-scaling (Owner: DevOps, Q3 2026)
- Considerar TTL no DynamoDB para expurgo automático de registros antigos (Owner: Senior Engineer, Q4 2026)

### 8. Links

- Código: `internal/history/dynamodb.go` — implementação completa do Recorder
- Código: `internal/models/payment.go` — struct `PaymentHistory` com tags `dynamodbav`
- Código: `internal/ui/handlers.go` — linhas 194-241 (consulta de histórico)
- Código: `docker-compose.yml` — linhas 30-41 (serviço Floci)
- Config: `dynamodb_endpoint=http://localhost:4566`, `dynamodb_table=payment_history`
- Testes: `internal/history/dynamodb_test.go`
- Relacionados: ADR-002 (Redis), ADR-008 (consistência eventual)

---

## References

- ADR concept and template rationale: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
- AWS SDK Go v2 DynamoDB: https://aws.github.io/aws-sdk-go-v2/docs/
