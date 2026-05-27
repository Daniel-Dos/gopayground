# Architecture Decision Record (ADR)

**ADR ID**: ADR-007  
**Title**: CLI Producer como Binário Separado do Consumer  
**Status**: Accepted  
**Date**: 2026-05-25  
**Owner**: Payment Platform Team  
**Deciders**: Architect, Senior Engineer  
**Scope**: Ferramenta de linha de comando para publicação de eventos de pagamento

---

## Core

### 1. Context

**Problem statement**: A equipe precisa de uma ferramenta de linha de comando para publicar eventos de pagamento no Kafka para testes manuais, demonstrações, e integração. Esta ferramenta não deve estar acoplada ao serviço consumer, permitindo evolução independente, builds separados, e sem dependências desnecessárias no consumer.

**Goals**:
- Publicar eventos de pagamento no tópico `payment.events`
- Suportar entrada via flags, stdin pipe, arquivo JSON, e geração em massa
- Validar payloads antes de publicar (reusa o mesmo validador)
- Suportar rate limiting para simulação de carga
- Binário independente, sem dependências do consumer

**Non-goals**:
- Consumir mensagens do Kafka (apenas produzir)
- Gerenciar estado ou status de pagamentos
- Interface gráfica
- Operações de admin no Kafka (criar tópicos)

**Constraints** (REQUIRED):
- Platform/runtime: Go 1.26, CLI puro (sem HTTP server)
- Team/operational maturity: Uso interno pela equipe de desenvolvimento
- Distribuição: Binário único, sem dependências externas

**Assumptions**:
- Ferramenta usada principalmente em desenvolvimento e testes
- Não haverá produção usando CLI como produtor principal
- Kafka acessível via rede local (localhost:9092 padrão)

### 2. Decision Drivers (What Matters Most)

| Priority | Driver | Why it matters | How we measure |
|---:|---|---|---|
| 1 | Independência | Não acoplar consumer à CLI | Build separado, zero imports do consumer |
| 2 | Facilidade de uso | Flags intuitivas, pipe, stdin | --help, exemplos, JSON output |
| 3 | Reuso de validação | Mesmas regras do consumer | Reusa `internal/validator` |
| 4 | Modo dry-run | Validar sem publicar | --dry-run flag |

### 3. Options Considered

| Option | Summary | Pros | Cons | Reversibility |
|---|---|---|---|---|
| A - Binário separado (cmd/producer) | Independente, main.go separado | Zero acoplamento, build independente, sem dependências extras no consumer | Duas compilações, Makefile mais complexo | Easy |
| B - Modo CLI dentro do consumer | Consumer aceita flag --producer | Um único binário, reuso máximo | Consumer ganha dependência de CLI, complexidade desnecessária, risco de segurança | Medium |
| C - Script shell com curl/kcat | Script bash usando kcat (kafkacat) | Zero build, independente | Sem validação de payload, dependência de kcat instalado, sem confiabilidade | Easy |
| D - HTTP API no consumer | Endpoint POST /publish no consumer | Acesso via qualquer HTTP client (curl) | Consumer vira servidor HTTP, risco de segurança, acoplamento | Medium |

### 4. Decision

**We choose**: Option A - Binário separado (cmd/producer)

**Why**:
- `cmd/producer/main.go` é completamente independente de `cmd/consumer` — zero imports compartilhados além de `internal/validator`
- `Makefile` tem targets separados: `build-producer` e `build` (consumer) — linhas 9-16
- CLI tem seu próprio `sarama.NewConfig` com timeouts específicos (`Producer.Timeout=10s`, `Net.DialTimeout=5s`, `MaxMessageBytes=100KB`) — `main.go:269-273`
- Suporta múltiplas fontes de eventos: flags individuais, stdin pipe, arquivo JSON, e geração em massa (`getEvents()` em `main.go:135-158`)
- `dry-run` valida sem publicar (`--dry-run` em `main.go:57`)
- `json-output` permite scripting com output JSON (`--json-output` em `main.go:58`)
- Rate limiting integrado (`--rate` em `main.go:55`) — implementado via `time.Ticker` em `internal/producer/producer.go:48-52`

### 5. Architecture Impact (Implementation-Ready)

**Boundaries and contracts**
- Public APIs/contracts affected:
  - `producer.Service.Publish(ctx, events, rate)` — interface em `internal/producer/producer.go:20-22`
  - `producer.GenerateEvent(...)` — gera evento único com defaults — `producer.go:117-139`
  - `producer.GenerateBulkEvents(count)` — gera N eventos — `producer.go:141-153`
  - CLI flags:
    - `--payment-id`, `--status`, `--amount`, `--currency`, `--description`
    - `--topic`, `--brokers`, `--payload`, `--file`, `--count`, `--rate`
    - `--dry-run`, `--json-output`
- Backward compatibility plan: N/A (primeira implementação)
- Schema evolution strategy: N/A

**Data and consistency**
- Source of truth: N/A
- Consistency model: N/A
- Migration strategy: N/A

**Failure modes and resilience**
- Known failure modes:
  - Kafka indisponível → `sarama.NewSyncProducer` retorna erro, exit code 1 (`main.go:275-278`)
  - Payload inválido → erro de validação, continua para próximo evento (`producer.go:84-88`)
  - Contexto cancelado (Ctrl+C) → `Publish` retorna resultados parciais com erro de contexto (`producer.go:55-62`)
  - Arquivo muito grande (> 10MB) → `"file too large"` — `main.go:95-97`
  - Stdin vazio → `"stdin is empty"` — `main.go:84-85`
- Timeouts/retries/backoff policy:
  - `Producer.Timeout = 10s` (main.go:271)
  - `Net.DialTimeout = 5s` (main.go:272)
  - Sem retry (falha → erro retornado no Result)
- Idempotency strategy: N/A (CLI pode gerar mesmo payment_id)
- Degradation plan: CLI sempre retorna exit code 0 se tudo OK, 1 se algum evento falhar

**Security**
- Threat model summary: Conexão Kafka sem autenticação (PLAINTEXT)
- AuthN/AuthZ model: N/A
- Secret and key management: N/A
- Audit logging requirements: Eventos publicados incluem header `source: cli-producer`

**Observability**
- SLIs/SLOs: N/A
- Metrics/traces/logs to add: Output JSON/Text de resultados (partition, offset, error)
- Dashboards and alerts: N/A

**Cost and capacity**
- Expected traffic/load: Uso manual, tipicamente 1-100 eventos por execução
- Cost model: N/A (ferramenta de desenvolvimento)
- Capacity plan: N/A

### 6. Rollout, Validation, and Rollback

**Rollout plan**
- Feature flag / staged rollout: N/A
- Data migration steps: N/A
- Runbook updates: Adicionar `producer help` aos comandos comuns

**Validation plan**
- Tests to add (unit/integration/contract):
  - `cmd/producer/main_test.go` — testes de integração do CLI
  - `internal/producer/producer_test.go` — testes do service Publish
- Load/perf tests: N/A

**Rollback plan**
- How to revert code: `git revert`
- How to revert data (or forward-fix): N/A
- Timebox for rollback decision: 15 min

### 7. Consequences

**Positive**
- Consumer não tem dependência de CLI — `cmd/consumer/main.go` não importa `internal/producer`
- Flags ricas e flexíveis: `echo '{"payment_id":"..."}' | ./producer publish`
- Output em dois formatos: texto legível (`main.go:199-205`) e JSON para scripting (`main.go:207-236`)
- Headers adicionados automaticamente: `source: cli-producer`, `timestamp` — linhas 96-98 do `producer.go`
- Rate limiting permite simular carga com `--rate 10` (10 msg/s) — `producer.go:48-52`
- Validator reusado garante que eventos CLI seguem mesmas regras do consumer — `producer.go:84`
- Tratamento de Ctrl+C com graceful cancellation — `main.go:248-254`

**Negative / tradeoffs**
- Dois binários para build/manter (Makefile com targets `build`, `build-ui`, `build-producer`)
- Lógica de parse de flags está em `cmd/producer/main.go` (~290 linhas), maior que o service em `internal/producer`
- Sem tracing OTel no CLI (diferente do consumer) — eventos não têm trace_id de origem
- `GenerateEvent` usa `math/rand` para UUID (espera `uuid.New()`) — mas `payment_id` vazio gera UUID v4 real em `producer.go:119`

**Follow-ups**
- Adicionar tracing OTel ao CLI producer para rastreabilidade de eventos (Owner: Senior Engineer, Q3 2026)
- Adicionar flag `--template` para gerar eventos a partir de templates Go (Owner: Senior Engineer, Q4 2026)

### 8. Links

- Código: `cmd/producer/main.go` — entrada da CLI (290 linhas)
- Código: `internal/producer/producer.go` — service de publicação (153 linhas)
- Makefile: linhas 14-16 (`build-producer`), 24-25 (`run-producer`)
- Testes: `cmd/producer/main_test.go`, `internal/producer/producer_test.go`
- Config: `kafka_brokers=localhost:9092`, `kafka_topic=payment.events`
- Relacionados: ADR-001 (Kafka), ADR-001 (validação)

---

## References

- ADR concept and template rationale: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
- Sarama SyncProducer: https://pkg.go.dev/github.com/IBM/sarama#SyncProducer
