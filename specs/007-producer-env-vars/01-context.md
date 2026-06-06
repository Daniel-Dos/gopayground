# 01 — Contexto

## Problema

O `docker-compose.yml` define variáveis de ambiente para o serviço `producer`:
`KAFKA_BROKERS`, `KAFKA_TOPIC`, `OTEL_ENDPOINT`, `OTEL_EXPORTER_OTLP_ENDPOINT`
e `OTEL_SERVICE_NAME`. Porém, o `cmd/producer/main.go` **nunca lê** essas
variáveis — ele só usa flags de linha de comando com valores padrão hardcoded.

Além disso, o producer **não inicializa o OpenTelemetry**. As variáveis
`OTEL_*` definidas no `docker-compose.yml` permanecem mortas: não há tracer
provider nem meter provider sendo configurados, o que significa que traces
e métricas do producer nunca são exportados para o collector.

O `command` do serviço producer no `docker-compose.yml` atualmente faz:
```yaml
command: ["serve", "--port", "8082", "--brokers", "kafka:9092"]
```

Isso ignora completamente as env vars definidas abaixo, criando uma
inconsistência: o arquivo de composição declara variáveis que o código
não consome.

## Motivação de Negócio

1. **12-Factor App / Cloud-Native** — A aplicação deve obter configuração
   do ambiente (env vars) e não de código ou flags fixas. Isso permite
   que o mesmo binário seja usado em diferentes ambientes (dev, staging,
   produção) sem recompilação.

2. **Consistência com o consumer** — O consumer (`cmd/consumer/main.go`)
   já usa `internal/config` com Viper, que lê `config.yaml` + env vars,
   e inicializa OpenTelemetry via `pkg/telemetry`. O producer deve seguir
   o mesmo padrão.

3. **Observabilidade** — Sem OpenTelemetry, o producer não gera traces
   nem métricas. Isso impede diagnóstico de latência, rastreamento de
   requisições entre serviços e monitoramento de taxa de publicação.

4. **Operacionalidade** — Operadores esperam que variáveis de ambiente
   funcionem. A discrepância entre o que o `docker-compose.yml` declara
   e o que o código efetivamente lê gera confusão e erros em deploy.

## Sistemas Envolvidos

| Sistema            | Função                                                |
|--------------------|-------------------------------------------------------|
| **Producer**       | Serviço HTTP que publica eventos de pagamento no Kafka|
| **Kafka**          | Tópico `payment.events` onde os eventos são publicados|
| **OpenTelemetry**  | Coleta de traces e métricas do producer               |
| **otel-collector** | Coletor OTLP para onde traces/métricas são exportados |
| **config.yaml**    | Arquivo de configuração com defaults da aplicação     |

## Restrições Conhecidas

1. O modo "publish" (CLI) **não deve ser alterado** — continua usando
   apenas flags de linha de comando, sem dependência de config ou telemetry.
2. O `Dockerfile.producer` já copia `config.yaml` para `/app/config.yaml`
   no container — nenhuma alteração em Docker é necessária.
3. O `docker-compose.yml` já define as env vars necessárias — nenhuma
   alteração no compose é necessária.
4. Flags de CLI no modo "serve" devem continuar funcionando e ter
   **maior precedência** que env vars e config.yaml.
5. Nenhuma dependência Go nova deve ser adicionada — `internal/config`
   e `pkg/telemetry` já existem e usam Viper e OTel SDK respectivamente.

## Fluxo Alvo

```
1. docker-compose.yml define env vars (já existentes)
2. Producer inicia no modo "serve"
3. internal/config carrega: config.yaml → env vars (sobrescreve) → flags (sobrescrevem)
4. OpenTelemetry é inicializado com cfg.OTel.Endpoint e cfg.OTel.ServiceName
5. Kafka producer é configurado com cfg.Kafka.Brokers e cfg.Kafka.Topic
6. Servidor HTTP inicia na porta cfg.Server.Port
7. Ao receber SIGTERM/SIGINT, telemetry é desligado graciosamente antes de sair
```

---

## Decisões de Design (preliminares)

- **Reuso do `internal/config`** — evitar duplicação de lógica de
  configuração. O pacote já lida com Viper, env vars e decode.
- **Ordem de precedência**: flags > env vars > config.yaml > defaults
  do Viper. Isso mantém a flexibilidade da CLI sem quebrar a configuração
  via ambiente.
- **Telemetry inicializado apenas no modo "serve"** — o modo "publish"
  é uma CLI transient e não precisa de OTel.
- **Config port para o producer** — o producer usa porta 8082 (diferente
  do consumer que usa 8080). O `config.yaml` tem `server.port: 8080`,
  então o producer deve sobrescrever via env var `SERVER_PORT=8082`
  ou via flag `--port 8082`.
