# Guia de Setup

## Requisitos

- **Go** 1.26+
- **Docker** e **Docker Compose** (para serviços locais)
- **Make** (opcional, para usar o Makefile)

## Stack de Serviços (Docker Compose)

O arquivo `docker-compose.yml` sobe todos os serviços necessários:

| Serviço          | Imagem                                   | Portas             | Descrição                            |
|------------------|------------------------------------------|--------------------|--------------------------------------|
| **kafka**        | `apache/kafka:latest` (KRaft mode)       | `9092`             | Message broker (3 partições padrão)  |
| **redis**        | `redis:7-alpine`                         | `6379`             | Cache de status e idempotência       |
| **floci**        | `floci/floci:latest`                     | `4566`             | DynamoDB Local (compatível AWS)      |
| **payment-ui**   | Build local (`Dockerfile.ui`)            | `8081`             | Interface web de monitoramento       |
| **otel-collector**| `otel/opentelemetry-collector-contrib`   | `4317`, `4318`, `8888`, `8889` | Coleta e exporta telemetria |

### Iniciar serviços

```bash
# Iniciar todos os serviços
docker-compose up -d

# Verificar status
docker-compose ps

# Acompanhar logs
docker-compose logs -f
```

### Parar serviços

```bash
docker-compose down
```

### Rede

Todos os serviços compartilham a rede `payment-network` (bridge).

## Compilação

### Consumer

```bash
make build
# ou
go build -o bin/consumer ./cmd/consumer
```

### UI

```bash
make build-ui
# ou
go build -o bin/ui ./cmd/ui
```

### CLI Producer

```bash
make build-producer
# ou
go build -o bin/producer ./cmd/producer
```

## Execução

### 1. Iniciar dependências

```bash
docker-compose up -d kafka redis floci otel-collector
```

### 2. Executar o Consumer

```bash
make run
# ou
go run ./cmd/consumer
```

O consumer inicia na porta `8080` (health checks) e começa a consumir do tópico `payment.events`.

### 3. Executar a UI (separadamente)

```bash
make run-ui
# ou
go run ./cmd/ui
```

A UI fica disponível em `http://localhost:8081`.

### 4. Publicar eventos de teste

```bash
# Evento único com valores padrão
go run ./cmd/producer

# Evento customizado
go run ./cmd/producer --status failed --amount 50.00

# Bulk de 10 eventos
go run ./cmd/producer --count 10

# Modo dry-run (apenas valida)
go run ./cmd/producer --dry-run
```

### Usando Docker Compose completo (incluindo UI)

```bash
# Sobe tudo inclusive a UI
docker-compose up -d

# Compila e executa o consumer localmente
make run

# OU executa tudo isolado (apenas dependências)
docker-compose up -d kafka redis floci otel-collector
```

## Variáveis de Ambiente

### Consumer

| Variável                       | Default                     | Descrição                                |
|--------------------------------|-----------------------------|------------------------------------------|
| `KAFKA_BROKERS`                | `localhost:9092`            | Lista de brokers Kafka                   |
| `KAFKA_TOPIC`                  | `payment.events`            | Tópico de consumo                        |
| `KAFKA_DLQ_TOPIC`              | `payment.events.dlq`        | Tópico DLQ                               |
| `KAFKA_CONSUMER_GROUP`         | `payment-consumer-group`    | Consumer group ID                        |
| `REDIS_ADDR`                   | `localhost:6379`            | Endereço Redis                           |
| `REDIS_PASSWORD`               | (vazio)                     | Senha Redis                              |
| `DYNAMODB_ENDPOINT`            | `http://localhost:4566`     | Endpoint DynamoDB (local)                |
| `DYNAMODB_TABLE`               | `payment_history`           | Tabela DynamoDB                          |
| `WORKER_COUNT`                 | `10`                        | Número de workers concorrentes           |
| `IDEMPOTENCY_TTL_HOURS`        | `24`                        | TTL da chave de idempotência (horas)     |
| `STATUS_TTL_HOURS`             | `168`                       | TTL do status no Redis (7 dias)          |
| `RETRY_MAX_ATTEMPTS`           | `3`                         | Máximo de tentativas de retry            |
| `RETRY_BASE_DELAY_MS`          | `100`                       | Delay base do backoff (ms)               |
| `OTEL_EXPORTER_OTLP_ENDPOINT`  | `localhost:4317`            | Endpoint OTel collector                  |
| `OTEL_SERVICE_NAME`            | `payment-consumer`          | Nome do serviço para tracing             |
| `GRACEFUL_SHUTDOWN_TIMEOUT`    | `30s`                       | Timeout para graceful shutdown           |
| `SERVER_PORT`                  | `8080`                      | Porta do servidor HTTP (health check)    |

### UI

| Variável              | Default                     | Descrição                             |
|-----------------------|-----------------------------|---------------------------------------|
| `UI_PORT`             | `8081`                      | Porta do servidor HTTP                |
| `UI_EVENT_BUS_BUFFER` | `256`                       | Tamanho do buffer do Event Bus        |
| `UI_READ_TIMEOUT`     | `10s`                       | Timeout de leitura HTTP               |
| `UI_WRITE_TIMEOUT`    | `30s`                       | Timeout de escrita HTTP (SSE)         |
| `REDIS_ADDR`          | `localhost:6379`            | Endereço Redis (compartilhado)        |
| `REDIS_PASSWORD`      | (vazio)                     | Senha Redis (compartilhado)           |
| `DYNAMODB_ENDPOINT`   | `http://localhost:4566`     | Endpoint DynamoDB (compartilhado)     |
| `DYNAMODB_TABLE`      | `payment_history`           | Tabela DynamoDB (compartilhado)       |

## Configuração via Arquivo YAML

O projeto também suporta configuração via `config.yaml`:

```yaml
server:
  port: 8080
```

As variáveis de ambiente sobrescrevem as configurações do arquivo. O arquivo é opcional — se não existir, os defaults são aplicados.

## Testes

```bash
# Executar todos os testes com race detector
make test

# Ou diretamente
go test ./... -race -count=1 -timeout=120s

# Executar lint
make lint
```

## Dockerfiles

### Consumer (Dockerfile)

```bash
docker build -t payment-consumer -f Dockerfile .
docker run --network=host payment-consumer
```

### UI (Dockerfile.ui)

```bash
docker build -t payment-ui -f Dockerfile.ui .
docker run --network=host payment-ui
```

## Troubleshooting

### Kafka não conecta

Verifique se o Kafka está saudável:
```bash
docker-compose logs kafka
```

### Redis não responde

```bash
docker-compose exec redis redis-cli PING
# Deve retornar PONG
```

### DynamoDB não disponível

```bash
# Listar tabelas
docker-compose exec floci awslocal dynamodb list-tables
# Deve retornar "payment_history"
```

### Erro de configuração

Certifique-se de que as variáveis de ambiente estão corretas, especialmente ao rodar fora do Docker Compose (endereços `localhost` vs nomes de serviço Docker).
