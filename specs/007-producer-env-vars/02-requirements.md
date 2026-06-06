# 02 — Requisitos

## Requisitos Funcionais

| ID     | Descrição                                                                                | Prioridade |
|--------|------------------------------------------------------------------------------------------|------------|
| RF-001 | Carregar configurações do producer (modo "serve") usando `internal/config.NewConfig()`.   | Alta       |
| RF-002 | Inicializar OpenTelemetry TracerProvider e MeterProvider no modo "serve".                | Alta       |
| RF-003 | Desligar TracerProvider e MeterProvider graciosamente no shutdown do modo "serve".        | Alta       |
| RF-004 | Respeitar ordem de precedência: **flags > env vars > config.yaml > defaults**.           | Alta       |
| RF-005 | Manter modo "publish" (CLI) inalterado — sem config, sem telemetry.                       | Alta       |
| RF-006 | Manter compatibilidade total de flags do modo "serve" (`--port`, `--brokers`, `--topic`). | Alta       |
| RF-007 | Servir na porta correta conforme configuração (default 8082 via env ou flag).             | Média      |
| RF-008 | Logs do producer devem usar `cfg.OTel.ServiceName` como identificador de serviço.         | Média      |

### Mapeamento de Configurações

O producer no modo "serve" precisa das seguintes configurações do `Config`
struct. O mapeamento entre env vars, config.yaml e flags deve ser:

| Config Path          | Env Var             | config.yaml        | Flag         | Default           |
|----------------------|---------------------|--------------------|--------------|-------------------|
| `kafka.brokers`      | `KAFKA_BROKERS`     | `kafka.brokers`    | `--brokers`  | `localhost:9092`  |
| `kafka.topic`        | `KAFKA_TOPIC`       | `kafka.topic`      | `--topic`    | `payment.events`  |
| `server.port`        | `SERVER_PORT`       | `server.port`      | `--port`     | `8082`            |
| `otel.endpoint`      | `OTEL_ENDPOINT`     | `otel.endpoint`    | _(nenhuma)_  | `localhost:4317`  |
| `otel.service_name`  | `OTEL_SERVICE_NAME` | `otel.service_name` | _(nenhuma)_  | `payment-producer`|

> **Nota:** As env vars `OTEL_EXPORTER_OTLP_ENDPOINT` definidas no
> `docker-compose.yml` seguem a convenção OpenTelemetry, mas o producer
> usa `OTEL_ENDPOINT` (configurada via Viper). O `docker-compose.yml`
> define ambas para compatibilidade. A env var efetivamente lida será
> `OTEL_ENDPOINT` (mapeada para `otel.endpoint` via Viper).

### Ordem de Precedência (modo "serve")

```
1. Flags de CLI (--port, --brokers, --topic) — maior prioridade
2. Variáveis de ambiente (KAFKA_BROKERS, KAFKA_TOPIC, SERVER_PORT, OTEL_*)
3. Arquivo config.yaml
4. Defaults do Viper / código
```

Regras:
- Se uma flag for fornecida, ela **sempre** vence — ignora env var e config.yaml
- Se uma env var for fornecida mas a flag não, a env var vence
- Se nem flag nem env var forem fornecidas, usa config.yaml
- Se config.yaml não tiver o campo, usa o default do Viper / código

---

## Requisitos Não Funcionais

| ID      | Descrição                                                        | Métrica         | Alvo             |
|---------|------------------------------------------------------------------|-----------------|------------------|
| RNF-001 | Nenhuma nova dependência Go além das já existentes.              | go.mod          | Sem novas deps   |
| RNF-002 | Producer com env vars deve iniciar e publicar eventos no Kafka.  | Teste funcional | OK               |
| RNF-003 | TracerProvider e MeterProvider devem ser desligados no shutdown. | Log / teste     | Shutdown chamado |
| RNF-004 | Flags de CLI devem continuar funcionando exatamente como antes.  | Teste regressão | Mesmo output    |
| RNF-005 | Modo "publish" não deve carregar config.yaml nem inicializar OTel.| Teste unitário  | Zero chamadas    |
| RNF-006 | Config port default deve ser 8082 para o producer (modo serve).   | Teste           | 8082             |

---

## Fora de Escopo

- Alterações no `docker-compose.yml` — já está correto
- Alterações no `Dockerfile.producer` — já copia `config.yaml`
- Adição de novas env vars além das já definidas no compose
- Configurações de Redis, DynamoDB, Worker, Retry, UI — não são usadas pelo producer
- OpenTelemetry no modo "publish" — CLI transient não precisa
- Criação de flags `--otel-endpoint` ou `--otel-service-name` — OTel configurado
  apenas via env vars e config.yaml

---

## Restrições Técnicas

| ID  | Restrição                                             | Justificativa                           |
|-----|-------------------------------------------------------|-----------------------------------------|
| R01 | Usar `internal/config` já existente.                  | Reuso, consistência com consumer         |
| R02 | Usar `pkg/telemetry` já existente.                    | Reuso, consistência com consumer         |
| R03 | Modo "publish" permanece 100% baseado em `flag` stdlib.| Spec 0003, não misturar responsabilidades|
| R04 | Kafka client: `github.com/IBM/sarama` (existente).     | Já em uso pelo consumer                  |
| R05 | Testes: `testing` + `github.com/stretchr/testify`.     | Padrão do projeto                        |

---

## Histórico de Revisão

| Versão | Data       | Autor     | Descrição           |
|--------|------------|-----------|---------------------|
| 1.0    | 2026-06-06 | Architect | Versão inicial      |
