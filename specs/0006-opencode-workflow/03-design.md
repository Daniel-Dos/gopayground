# 03 — Design

## Diagrama de Arquitetura (Workflow)

```mermaid
flowchart TD
    subgraph "Triggers"
        PR[PR aberto/sync/reaberto<br/>branch: master]
        IC[Comentário em Issue]
        RC[Comentário em Review de PR]
    end

    subgraph "Condição de Execução"
        C{Evento é PR<br/>OU comentário contém<br/>/oc, /opencode ou /run?}
    end

    subgraph "Steps do Workflow"
        S1[Checkout<br/>actions/checkout@v6]
        S2[Setup Docker BuildKit<br/>docker/setup-buildx-action@v3]
        S3[Setup Go 1.26.0<br/>actions/setup-go@v6]
        S4[Start Kafka 3.9.0<br/>docker run apache/kafka]
        S5[Start Redis 7-alpine<br/>docker run redis]
        S6[Wait for Services<br/>health check script]
        S7[Build multi-alvo<br/>make build build-ui build-producer]
        S8[Executar OpenCode<br/>anomalyco/opencode/github@latest]
    end

    PR --> C
    IC --> C
    RC --> C
    C -->|sim| S1
    S1 --> S2 --> S3 --> S4 --> S5 --> S6 --> S7 --> S8

    C -->|não| END([Fim - job ignorado])
```

## Mapeamento Detalhado Rust → Go

| Step (Original - Rust) | Equivalente (Go - Proposto) | Justificativa |
|------------------------|----------------------------|---------------|
| `dtolnay/rust-toolchain@stable` | `actions/setup-go@v6` com `go-version-file: go.mod` e `cache: true` | Setup da toolchain da linguagem; Go usa `setup-go` com cache de módulos |
| `docker run -d -p 4222:4222 --name nats-server nats` (NATS) | `docker run -d -p 9092:9092 --name kafka-server ... apache/kafka:3.9.0` + `docker run -d -p 6379:6379 --name redis-server redis:7-alpine` | NATS (mensageria simples) → Kafka (broker do projeto) + Redis (cache/estado do projeto) |
| `cargo build` | `make build build-ui build-producer` | Compilação dos binários Go: consumer, ui, producer |
| Mesmo | `docker rm -f kafka-server redis-server` antes de `docker run` (adicional) | Garante idempotência em re-runs |
| Mesmo | Script de health check após start dos containers (adicional) | Garante que serviços estão prontos antes do build |

## Configuração dos Containers de Infraestrutura

### Kafka (modo KRaft — sem ZooKeeper)

```bash
docker run --rm -d \
  -p 9092:9092 \
  --name kafka-server \
  -e KAFKA_PROCESS_ROLES=broker,controller \
  -e KAFKA_NODE_ID=1 \
  -e KAFKA_CONTROLLER_QUORUM_VOTERS=1@localhost:9093 \
  -e CLUSTER_ID=opencode-cluster \
  -e KAFKA_CONTROLLER_LISTENER_NAMES=CONTROLLER \
  -e KAFKA_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093 \
  -e KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://localhost:9092 \
  -e KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT \
  -e KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1 \
  -e KAFKA_AUTO_CREATE_TOPICS_ENABLE=true \
  apache/kafka:3.9.0
```

### Redis

```bash
docker run --rm -d \
  -p 6379:6379 \
  --name redis-server \
  redis:7-alpine
```

### Health Check

```bash
# Aguarda Kafka ficar pronto (até 60s)
for i in $(seq 1 30); do
  docker inspect kafka-server --format='{{.State.Status}}' 2>/dev/null | grep -q running && break
  sleep 2
done

# Aguarda Redis ficar pronto (até 30s)
for i in $(seq 1 15); do
  docker inspect redis-server --format='{{.State.Status}}' 2>/dev/null | grep -q running && break
  sleep 2
done
```

## Prompt Condicional (OpenCode)

A lógica do prompt reproduz exatamente o comportamento do workflow original:

```yaml
prompt: |
  ${{ github.event_name == 'pull_request' && 'Execute o /run' || github.event.comment.body }}
  ${{ github.event_name != 'pull_request' && contains(github.event.comment.body, '/run') && 'Execute o /run' || '' }}
```

**Comportamento esperado:**

| Cenário | Evento | Conteúdo do prompt |
|---------|--------|-------------------|
| PR aberto/sync/reaberto | `pull_request` | `"Execute o /run\n"` |
| Comentário com `/run` | `issue_comment` | corpo do comentário + `"\nExecute o /run"` |
| Comentário com `/oc` | `issue_comment` | corpo do comentário |
| Comentário sem comando | `issue_comment` | corpo do comentário (linha extra vazia) |
| Comentário em review com `/run` | `pull_request_review_comment` | corpo do comentário + `"\nExecute o /run"` |

## Decisões Arquiteturais

| Decisão | Alternativa | Justificativa |
|---------|-------------|---------------|
| Container único para Kafka (KRaft) | Usar image bitnami/kafka com ZooKeeper | Projeto já usa `apache/kafka:3.9.0` em KRaft no docker-compose; manter consistência |
| Health check via loop `docker inspect` | Usar `docker compose up -d` | Workflow de CI não tem docker-compose.yml disponível (pode não estar no runner); container individual é mais portátil e rápido |
| Dois containers separados (Kafka + Redis) | Container único com docker-compose | Simplicidade e alinhamento com o original (que só subia NATS); dois comandos `docker run` separados |
| `make build build-ui build-producer` | `go build ./...` | Makefile é a interface padronizada do projeto; `build-ui` e `build-producer` são builds específicos |
| Remover containers antes de iniciar | Confiar que não há conflito | Idempotência — em caso de re-run, containers podem já existir do run anterior |
| Mesmas permissões do original | Permissões mínimas | Manter compatibilidade com o que o OpenCode precisa para criar/atualizar PRs, checks, issues |
| Modelo `opencode/big-pickle` | Modelo `opencode/micro` | Manter o mesmo modelo já configurado no workflow atual do projeto |
