# Architecture Decision Record (ADR)

**ADR ID**: ADR-013  
**Title**: UI passa a publicar eventos via Producer HTTP  
**Status**: Accepted  
**Date**: 2026-05-28  
**Owner**: Payment Platform Team  
**Deciders**: Architect, Senior Engineer  
**Scope**: UI e Producer — remoção do produtor Kafka embutido na UI

---

## Core

### 1. Context

**Problem statement**: A Payment UI (`cmd/ui`) mantinha um produtor Kafka embutido (`producer.Service`) que duplicava a lógica de publicação já existente no producer standalone (`cmd/producer`). Eventos publicados pela UI não transitavam pelo serviço producer, o que significava que logs, métricas e tracing do producer não capturavam essas publicações — criando uma lacuna de observabilidade. Além disso, a UI precisava manter dependências diretas do Kafka (`sarama.SyncProducer`, configuração de timeouts, etc.) mesmo sendo primariamente uma interface de consulta e monitoramento.

**Goals**:
- Centralizar toda publicação Kafka no serviço producer standalone
- Garantir que eventos publicados pela UI apareçam nos logs e métricas do producer
- Remover dependências Kafka diretas da UI
- Reduzir a superfície de configuração necessária na UI
- Manter o SSE em tempo real funcional para o browser

**Non-goals**:
- Alterar a API pública da UI (`POST /api/publish`, `POST /api/publish/bulk`) — os endpoints permanecem com a mesma assinatura e resposta
- Modificar o serviço producer standalone — apenas garante-se que ele já expõe os endpoints HTTP necessários
- Remover o EventBus / Redis Pub/Sub — SSE continua dependendo dele
- Alterar o fluxo do consumer ou de outros componentes

**Constraints** (REQUIRED):
- Platform/runtime: Go 1.26, HTTP/1.1 entre serviços
- SLO: Resposta de publicação em < 10s (timeout do HTTP client)
- Team/operational maturity: Serviço producer já opera em produção com modo `serve`

**Assumptions**:
- O producer standalone está sempre disponível quando a UI está no ar (mesmo docker-compose)
- A latência adicional de ~10-50ms do round-trip HTTP é aceitável para publicações manuais via UI
- O endpoint `/publish` do producer já existe e retorna `{payment_id, partition, offset}`
- O endpoint `/publish/bulk` do producer já existe e retorna uma lista de resultados

### 2. Decision Drivers (What Matters Most)

| Priority | Driver | Why it matters | How we measure |
|---:|---|---|---|
| 1 | Centralização de logs | Todo evento publicado passa pelo mesmo serviço | Logs do producer contêm todas as publicações |
| 2 | Simplificação da UI | UI não precisa de dependências Kafka | `go.mod` da UI sem `sarama`, `internal/producer` |
| 3 | Consistência arquitetural | Um único serviço responsável por publicar no Kafka | Producer é a única entrada para o Kafka |
| 4 | Baixa latência | Round-trip HTTP não pode degradar UX | Tempo de resposta < 200ms no pior caso |

### 3. Options Considered

| Option | Summary | Pros | Cons | Reversibility |
|---|---|---|---|---|
| A - HTTP Producer (escolhido) | UI chama `producer:8082/publish` via HTTP | Centraliza publicação, UI mais leve, observabilidade completa | Latência adicional de round-trip HTTP, dependência de rede entre serviços | Easy |
| B - Manter embedded producer | UI continua com `sarama.SyncProducer` próprio | Zero latência adicional, sem dependência externa | Logs duplicados, manutenção duplicada, UI com dependência Kafka desnecessária | Easy |
| C - SyncProducer direto (spec original) | UI chama Sarama SyncProducer diretamente sem camada `producer.Service` | Máxima performance | Perde centralização, sem logs no producer, duplicação de configuração Kafka | Medium |

### 4. Decision

**We choose**: Option A - HTTP Producer

**Why**:
- **UI** (`cmd/ui/main.go`):
  - Removeu `createKafkaProducer()`, `sarama.SyncProducer`, `producer.Service` — as linhas que criavam e mantinham o produtor Kafka embutido foram eliminadas
  - `NewServer` agora recebe apenas as dependências necessárias (Redis, DynamoDB, logger, meter) — sem `producer.Service`
- **Handlers** (`internal/ui/handlers.go`):
  - `HandlePublish` e `HandlePublishBulk` fazem HTTP POST para `producerURL/publish` e `producerURL/publish/bulk` respectivamente — linhas 397 e 492
  - `Handlers` agora contém `producerURL string` e `httpClient *http.Client` — linhas 54-55
  - Após a resposta HTTP do producer, `HandlePublish` ainda publica o evento no EventBus para o SSE — linha 436-447
  - Timeout configurado em 10s para publicação única, 30s para bulk — linhas 374, 483
- **Config** (`internal/config/config.go`):
  - `UIConfig.ProducerURL string` (tag `mapstructure:"producer_url"`) — linha 52
- **Config YAML** (`config.yaml`):
  - `ui.producer_url: "http://localhost:8082"` — linha 36
- **Docker Compose** (`docker-compose.yml`):
  - `UI_PRODUCER_URL=http://producer:8082` — linha 168
- **SSE** continua funcionando: após receber a resposta HTTP do producer, o handler publica no EventBus (Redis Pub/Sub) que alimenta o SSE — `handlers.go:436-447`

### 5. Architecture Impact (Implementation-Ready)

**Boundaries and contracts**
- Public APIs/contracts affected:
  - Nenhuma — `POST /api/publish` e `POST /api/publish/bulk` mantêm contratos inalterados
  - A UI agora depende do serviço producer (porta 8082) para publicar — se o producer estiver off, os endpoints retornam 502
- Backward compatibility plan: Total — respostas HTTP são idênticas às anteriores
- Schema evolution strategy: N/A

**Data and consistency**
- Source of truth: Kafka (via producer HTTP)
- Consistency model: A mesma — o producer usa SyncProducer do Sarama com `RequiredAcks`
- Migration strategy: N/A (não há migração de dados)

**Failure modes and resilience**
- Known failure modes:
  - Producer HTTP off → UI retorna 502 `"producer unavailable"` — `handlers.go:408`
  - Timeout de conexão HTTP → `http.Client.Do` retorna erro, UI retorna 502 — `handlers.go:406-408`
  - Producer retorna erro no body → UI retorna o status code e mensagem de erro do producer — `handlers.go:418-426`
  - Producer retorna JSON inválido → UI retorna 502 `"invalid producer response"` — `handlers.go:429-432`
  - Producer URL não configurada → UI retorna 502 `"producer not configured"` — `handlers.go:363-365`
- Timeouts/retries/backoff policy:
  - `http.Client.Timeout = 10s` para requisições únicas — `handlers.go:67`
  - Context timeout de 10s para `HandlePublish` — `handlers.go:374`
  - Context timeout de 30s para `HandlePublishBulk` — `handlers.go:483`
  - `MaxBytesReader` de 64KB para publish, 4KB para bulk — `handlers.go:378, 472`
  - Sem retry (falha HTTP → erro retornado ao usuário)
- Idempotency strategy: Producer já lida com idempotência via Sarama
- Degradation plan: UI continua funcionando para consultas (SSE, payments, metrics) mesmo com producer off

**Security**
- Threat model summary: Conexão HTTP entre serviços na mesma rede Docker (sem TLS)
- AuthN/AuthZ model: N/A (serviços internos, sem autenticação entre eles)
- Secret and key management: N/A
- Audit logging requirements: Logs de publicação centralizados no producer

**Observability**
- SLIs/SLOs: Publicação via HTTP < 10s (timeout)
- Metrics/traces/logs to add:
  - Log na UI: `"payment published"` com `payment_id`, `partition`, `offset` — `handlers.go:449-453`
  - Log na UI: `"producer HTTP call failed"` em caso de erro — `handlers.go:406`
  - Log na UI: `"eventbus publish error"` se EventBus falhar — `handlers.go:445`
  - Métricas OTel de requisição HTTP já capturam `/api/publish` e `/api/publish/bulk`
- Dashboards and alerts: N/A

**Cost and capacity**
- Expected traffic/load: Publicações manuais, tipicamente < 10 req/min
- Cost model: N/A
- Capacity plan: N/A

### 6. Rollout, Validation, and Rollback

**Rollout plan**
- Feature flag / staged rollout: N/A (mudança transparente para o usuário)
- Data migration steps: N/A
- Runbook updates: Garantir que `producer:8082` esteja listado como dependência da UI

**Validation plan**
- Tests to add (unit/integration/contract):
  - Testes de integração da UI com mock HTTP server para o producer
  - Testes de timeout e erro HTTP nos handlers de publish
  - Testes de contrato: resposta da UI é idêntica antes/depois da mudança
- Load/perf tests: Verificar latência adicional do round-trip HTTP

**Rollback plan**
- How to revert code: `git revert`
- How to revert data (or forward-fix): N/A
- Timebox for rollback decision: 15 min

### 7. Consequences

**Positive**
- Logs de publicação centralizados — toda publicação passa pelo mesmo serviço producer
- UI mais leve — removeu dependência de `sarama.SyncProducer` e `internal/producer`
- Configuração Kafka eliminada da UI — não precisa mais de `kafka.brokers`, `kafka.topic`, timeouts, etc.
- Observabilidade completa — tracing e métricas do producer capturam todas as publicações
- SSE mantido funcional — o EventBus é alimentado após a resposta HTTP do producer (`handlers.go:436-447`)
- Arquitetura mais limpa — UI é interface de consulta, producer é responsável por escrever no Kafka
- Docker Compose atualizado com `UI_PRODUCER_URL` apontando para o serviço producer

**Negative / tradeoffs**
- Latência adicional de ~10-50ms por publicação devido ao round-trip HTTP entre UI e producer
- UI agora depende do serviço producer para publicar — se o producer estiver off, `POST /api/publish` retorna 502
- Necessidade de configuração `producer_url` na UI (adicionado ao `UIConfig`)
- A UI precisa de um `http.Client` com timeouts configurados adequadamente
- Se o producer estiver sobrecarregado, a UI pode ter timeouts nas publicações

**Follow-ups**
- Adicionar circuit breaker na UI para o HTTP client do producer (Owner: Senior Engineer, Q3 2026)
- Adicionar health check do producer na página de status da UI (Owner: Senior Engineer, Q3 2026)

### 8. Links

- Código: `internal/ui/handlers.go` — `HandlePublish` (linhas 361-456), `HandlePublishBulk` (linhas 458-530)
- Código: `internal/ui/server.go` — `NewServer` sem `producer.Service` (linhas 37-89)
- Código: `cmd/ui/main.go` — removido `createKafkaProducer()` e dependências Sarama
- Código: `internal/config/config.go` — `UIConfig.ProducerURL` (linha 52)
- Config: `config.yaml` — `ui.producer_url: "http://localhost:8082"` (linha 36)
- Config: `docker-compose.yml` — `UI_PRODUCER_URL=http://producer:8082` (linha 168)
- Relacionados: ADR-007 (CLI Producer como binário separado)

---

## References

- ADR concept and template rationale: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
- Go net/http: https://pkg.go.dev/net/http
