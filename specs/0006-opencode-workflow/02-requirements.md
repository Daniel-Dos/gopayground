# 02 — Requisitos

## Requisitos Funcionais (RF)

### RF-001 — Triggers idênticos ao original
O workflow DEVE ser acionado nos mesmos eventos que o original do Rust:
- `pull_request` para branch `master` com types `[opened, synchronize, reopened]`
- `issue_comment` com type `[created]`
- `pull_request_review_comment` com type `[created]`

### RF-002 — Condição de execução idêntica ao original
O job DEVE executar apenas quando:
- O evento for `pull_request`, OU
- O corpo do comentário contiver ` /oc` ou começar com `/oc`, OU
- O corpo do comentário contiver ` /opencode` ou começar com `/opencode`, OU
- O corpo do comentário contiver ` /run` ou começar com `/run`

### RF-003 — Setup do ambiente Go
O workflow DEVE configurar o toolchain Go usando `actions/setup-go@v6` com:
- `go-version-file: go.mod` (lê versão 1.26.0 do go.mod)
- `cache: true` (habilita cache de módulos Go)

### RF-004 — Setup do Docker BuildKit
O workflow DEVE configurar Docker BuildKit via `docker/setup-buildx-action@v3`.

### RF-005 — Inicialização do Kafka
O workflow DEVE iniciar um container Kafka (imagem `apache/kafka:3.9.0`) em background na porta `9092`, em modo KRaft (sem ZooKeeper).

### RF-006 — Inicialização do Redis
O workflow DEVE iniciar um container Redis (imagem `redis:7-alpine`) em background na porta `6379`.

### RF-007 — Build multi-alvo
O workflow DEVE executar `make build build-ui build-producer` para compilar todos os binários do projeto (consumer, ui, producer).

### RF-008 — Execução do OpenCode
O workflow DEVE executar o agente OpenCode via `anomalyco/opencode/github@latest` com:
- Modelo: `opencode/big-pickle`
- Prompt condicional idêntico ao original
- Variável de ambiente `OPENCODE_API_KEY` extraída de secrets

### RF-009 — Prompt condicional
O prompt enviado ao OpenCode DEVE seguir a mesma lógica condicional do original:
- Se o evento for `pull_request`: o prompt deve ser `"Execute o /run"`
- Se o evento for comentário e contiver `/run`: o prompt deve ser o corpo do comentário + `"Execute o /run"`
- Caso contrário: o prompt deve ser o corpo do comentário

### RF-010 — Permissões do job
O job DEVE declarar as seguintes permissões:
- `id-token: write`
- `contents: read`
- `pull-requests: write`
- `issues: write`
- `checks: write`

## Requisitos Não-Funcionais (RNF)

### RNF-001 — Portabilidade
O workflow DEVE funcionar em `ubuntu-latest` (GitHub Actions runner padrão).

### RNF-002 — Idempotência de containers
O workflow DEVE usar `docker rm -f` antes de iniciar os containers Kafka e Redis para garantir que não haja conflito de nomes em casos de re-run do job.

### RNF-003 — Resiliência de serviços
O workflow DEVE aguardar a disponibilidade dos serviços Kafka e Redis antes de prosseguir para o build (health check via `docker inspect` ou `timeout` com retry).

### RNF-004 — Cache de dependências
O workflow DEVE utilizar o cache do `actions/setup-go@v6` para acelerar execuções subsequentes.

### RNF-005 — Segurança de credenciais
A chave `OPENCODE_API_KEY` DEVE ser obtida exclusivamente via `secrets.OPENCODE_API_KEY` — nunca hardcoded no workflow.

### RNF-006 — Manutenibilidade
O workflow DEVE ser semanticamente equivalente ao original do Rust, com comentários claros indicando o propósito de cada step e as adaptações realizadas.

### RNF-007 — Observabilidade
O workflow DEVE ter `timeout-minutes` definido no job para evitar execuções infinitas. Os steps de inicialização de containers DEVEM logar o status de cada serviço.

### RNF-008 — Compatibilidade com gitflow
O workflow DEVE residir em uma branch `feature/opencode-workflow-go` criada a partir de `develop`, seguindo o fluxo gitflow do projeto.

## Fora de Escopo

- **Não** modificar o workflow `ci.yml` existente
- **Não** adicionar Floci (DynamoDB local) ou OpenTelemetry Collector — são desnecessários para o agente OpenCode
- **Não** alterar o modelo OpenCode atual (`opencode/big-pickle`)
- **Não** criar novos secrets ou variáveis de ambiente além de `OPENCODE_API_KEY`
- **Não** modificar o Makefile ou docker-compose
- **Não** alterar a branch de destino (master) ou os types de trigger
