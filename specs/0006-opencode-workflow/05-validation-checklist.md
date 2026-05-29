# 05 — Validação e Checklist de Aceitação

## Checklist de Aceitação

### Estrutura e Equivalência com o Original

- [ ] **A1 — Triggers**: workflow é acionado por PR para master (opened/synchronize/reopened), issue_comment (created) e pull_request_review_comment (created) — igual ao original
- [ ] **A2 — Condição**: expressão `if` no job é semanticamente idêntica ao original (PR ou comandos /oc, /opencode, /run)
- [ ] **A3 — Permissões**: `id-token: write`, `contents: read`, `pull-requests: write`, `issues: write`, `checks: write` — todas presentes
- [ ] **A4 — Steps**: sequência correta: Checkout → Setup Docker → Setup Go → Start Kafka → Start Redis → Wait → Build → Run opencode
- [ ] **A5 — Prompt condicional**: lógica identica ao original com duas linhas condicionais no `prompt`
- [ ] **A6 — Modelo**: `opencode/big-pickle` (mesmo do workflow atual)
- [ ] **A7 — OPENCODE_API_KEY**: obtida via `secrets.OPENCODE_API_KEY` (não hardcoded)

### Infraestrutura

- [ ] **I1 — Go toolchain**: `actions/setup-go@v6` com `go-version-file: go.mod` e `cache: true`
- [ ] **I2 — Docker BuildKit**: `docker/setup-buildx-action@v3` presente
- [ ] **I3 — Kafka**: container `apache/kafka:3.9.0` em modo KRaft, porta 9092, nome `kafka-server`
- [ ] **I4 — Redis**: container `redis:7-alpine`, porta 6379, nome `redis-server`
- [ ] **I5 — Idempotência**: `docker rm -f` executado antes de cada `docker run` para limpar execuções anteriores
- [ ] **I6 — Health check**: loop de espera com `docker inspect` para Kafka (até 60s) e Redis (até 30s)
- [ ] **I7 — Build**: `make build build-ui build-producer` executa sem erros
- [ ] **I8 — Timeout**: `timeout-minutes: 30` definido no job

### Gitflow

- [ ] **G1 — Branch**: `feature/opencode-workflow-go` criada a partir de `develop`
- [ ] **G2 — Commit**: mensagem descritiva seguindo conventional commits (`feat(ci): ...`)
- [ ] **G3 — PR**: criado com base `develop` e head `feature/opencode-workflow-go`

### Segurança

- [ ] **S1 — Sem secrets expostos**: nenhuma chave hardcoded no YAML
- [ ] **S2 — persist-credentials**: `false` no checkout (segurança contra vazamento de token)
- [ ] **S3 — Permissões mínimas**: apenas as necessárias para o OpenCode operar

## Cenários de Teste Obrigatórios

### CT-01: Trigger por PR
1. Criar PR de `feature/opencode-workflow-go` para `master`
2. **Esperado**: workflow é acionado automaticamente
3. **Esperado**: prompt enviado ao OpenCode é `"Execute o /run"`

### CT-02: Trigger por comentário `/oc run` em PR
1. Em PR existente, comentar `/oc run`
2. **Esperado**: workflow é acionado
3. **Esperado**: prompt contém o corpo do comentário + linha extra

### CT-03: Trigger por comentário `/run` em Issue
1. Em issue existente, comentar `/run test feature`
2. **Esperado**: workflow é acionado
3. **Esperado**: prompt contém `"/run test feature\nExecute o /run"`

### CT-04: Comentário sem comando não aciona
1. Em PR existente, comentar "Apenas um comentário comum"
2. **Esperado**: workflow NÃO é acionado

### CT-05: Kafka e Redis sobem corretamente
1. Executar workflow
2. **Esperado**: logs mostram "Kafka is ready" e "Redis is ready"
3. **Esperado**: containers `kafka-server` e `redis-server` estão rodando nos steps seguintes

### CT-06: Build multi-alvo
1. Executar workflow
2. **Esperado**: `make build` gera `bin/consumer`
3. **Esperado**: `make build-ui` gera `bin/ui`
4. **Esperado**: `make build-producer` gera `bin/producer`

### CT-07: Re-run do workflow (idempotência)
1. Executar workflow uma vez
2. Re-executar o mesmo workflow (re-run)
3. **Esperado**: containers antigos são removidos (`docker rm -f`) antes de subir novos
4. **Esperado**: workflow completa com sucesso

## Critérios de Hardening (Pipeline)

- [ ] **H1 — Timeout global**: job não executa por mais de 30 minutos
- [ ] **H2 — Retry de serviços**: health check tem loop com retry (não falha na primeira tentativa)
- [ ] **H3 — Graceful cleanup**: containers são removidos (se existentes) antes de iniciar
- [ ] **H4 — Logs informativos**: cada step imprime status para debug
- [ ] **H5 — Cache**: módulos Go são cacheados entre execuções
