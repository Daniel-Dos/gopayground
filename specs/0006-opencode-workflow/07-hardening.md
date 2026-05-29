# 07 — Hardening (Pipeline CI/CD)

Embora este seja um workflow de CI/CD (não um serviço em produção), aplicamos princípios de resiliência, segurança e observabilidade para garantir execução robusta e previsível.

## Estratégia de Retry e Backoff

### Health Check de Serviços

```yaml
# Kafka: até 30 tentativas com intervalo de 2s (total ~60s)
for i in $(seq 1 30); do
  docker inspect kafka-server --format='{{.State.Status}}' 2>/dev/null | grep -q running && break
  sleep 2
done

# Redis: até 15 tentativas com intervalo de 2s (total ~30s)
for i in $(seq 1 15); do
  docker inspect redis-server --format='{{.State.Status}}' 2>/dev/null | grep -q running && break
  sleep 2
done
```

**Justificativa**: Retry linear com 2s de intervalo. Backoff exponencial não é necessário porque estamos apenas aguardando o container Docker ficar `running` (não estamos fazendo polling contra um serviço remoto com rate limiting).

### Melhoria futura (opcional)
Adicionar verificação real de prontidão do Kafka via `nc -z localhost 9092` ou `kafka-topics.sh` após o container ficar running, para garantir que o broker está aceitando conexões.

## Timeout em Cada Operação

| Step | Timeout | Mecanismo |
|------|---------|-----------|
| Job completo | 30 min | `timeout-minutes: 30` no YAML |
| Checkout | 5 min | `actions/checkout@v6` timeout implícito |
| Setup Go | 3 min | `actions/setup-go@v6` timeout implícito |
| Setup Docker | 2 min | `docker/setup-buildx-action@v3` timeout implícito |
| Start Kafka | 2 min | Docker pull + run, sem timeout adicional |
| Start Redis | 1 min | Docker pull + run, sem timeout adicional |
| Health check Kafka | 60s | Loop com 30 iterações × 2s |
| Health check Redis | 30s | Loop com 15 iterações × 2s |
| Build multi-alvo | 10 min | `make` sem timeout adicional |
| Run opencode | 15 min | Agente OpenCode |

## Proteção contra Falha Parcial

### Estratégia
O workflow NÃO usa `continue-on-error` nos steps críticos:

- Se **Kafka** falhar ao iniciar → workflow falha (build e testes dependem dele)
- Se **Redis** falhar ao iniciar → workflow falha (serviço obrigatório para UI/consumer)
- Se **Build** falhar → workflow falha (sem binários, agente não pode testar)
- Se **OpenCode** falhar → workflow falha (propósito principal do job)

### Exceção
O step de **remoção de containers** (`docker rm -f`) usa `|| true` para ignorar erro quando o container não existe (primeira execução).

## Observabilidade

### Logs Estruturados nos Steps

Cada step imprime mensagens informativas para debug:

```yaml
- name: Wait for services
  run: |
    echo "Waiting for Kafka..."
    for i in $(seq 1 30); do
      STATUS=$(docker inspect kafka-server --format='{{.State.Status}}' 2>/dev/null || echo "not_found")
      echo "  attempt $i/30 - Kafka status: $STATUS"
      [ "$STATUS" = "running" ] && echo "Kafka is ready" && break
      sleep 2
    done
```

### Métricas Nativas do GitHub Actions

- **Duração de cada step**: visível na UI do GitHub Actions
- **Taxa de sucesso/falha**: dashboard de Actions do repositório
- **Logs de execução**: disponíveis na aba "Checks" de cada PR

### Tracing Distribuído

Não se aplica — GitHub Actions não suporta OpenTelemetry nativamente. O log de cada step serve como fonte primária de observabilidade.

## Tratamento de Concorrência

### Concorrência entre workflows

Não há conflito de concorrência entre este workflow e o `ci.yml`:
- `opencode.yml`: acionado por PR/comentários
- `ci.yml`: acionado por push

Porém, se dois comentários `/run` forem feitos simultaneamente, duas execuções podem rodar em paralelo. Isso é aceitável pois cada execução:
- Usa `docker rm -f` antes de `docker run` (idempotente)
- Cria containers com nomes fixos (o segundo `docker run` falha se o primeiro ainda estiver rodando com o mesmo nome)

### Melhoria futura
Adicionar `concurrency` ao workflow para cancelar execuções anteriores do mesmo grupo:

```yaml
concurrency:
  group: opencode-${{ github.event.issue.number || github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
```

## Segurança Operacional

### Secrets
- `OPENCODE_API_KEY` é injetada via `env:` no step "Run opencode"
- GitHub Actions mascara automaticamente o valor nos logs
- **Nunca** expor a chave em `echo` ou scripts customizados

### Checkout
- `persist-credentials: false` impede que o token do GITHUB_TOKEN seja propagado para steps subsequentes (segurança contra vazamento via cache/container)

### Permissões
- `contents: read` — apenas leitura do código (não pode fazer push)
- `pull-requests: write` — necessário para o OpenCode comentar em PRs
- `issues: write` — necessário para o OpenCode comentar em Issues
- `checks: write` — necessário para o OpenCode atualizar status checks
- `id-token: write` — necessário para autenticação OIDC (se aplicável no futuro)

### Container Security
- Imagens oficiais: `apache/kafka:3.9.0` e `redis:7-alpine` — verificadas e mantidas
- `--rm` flag: containers são removidos automaticamente ao finalizar
- Portas expostas apenas no `localhost` do runner (não acessível externamente)

## Checklist de Hardening

- [ ] Health check com retry para Kafka e Redis
- [ ] Timeout global de 30 minutos no job
- [ ] Remoção de containers antes de iniciar (idempotência)
- [ ] `persist-credentials: false` no checkout
- [ ] `OPENCODE_API_KEY` via secrets (nunca hardcoded)
- [ ] Logs informativos em cada step para debug
- [ ] Containers oficiais e verificados
- [ ] Sem `continue-on-error` nos steps críticos
- [ ] `docker rm -f || true` para ignorar erro de container inexistente
