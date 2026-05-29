# SDD — workflows-001: GitHub Actions Workflows para gopayground

| Campo | Valor |
|---|---|
| ID | workflows-001 |
| Status | Draft |
| Data | 2026-05-28 |
| Autor | Architect |
| Repositório | `github.com/Daniel-Dos/gopayground` |

---

## 1. Propósito

### 1.1 Problema

O projeto **gopayground** é um sistema distribuído de processamento de pagamentos
que utiliza Kafka, Redis, DynamoDB e OpenTelemetry, com componentes em Go
(consumer, producer, UI web). Atualmente:

- **Não existe pipeline de CI/CD** — toda alteração é testada manualmente
- **Não há integração com OpenCode no GitHub** — impossível usar comentários
  `/oc` ou `/opencode` em issues e PRs
- **Não há revisão automática de PRs** — toda revisão é manual
- **Não há diretório `.github/`** — a infraestrutura de GitHub Actions está
  totalmente por criar

Isso gera riscos de qualidade, retrabalho e falta de rastreabilidade nas
alterações.

### 1.2 Motivação de Negócio

| Motivação | Detalhamento |
|---|---|
| **Qualidade** | Garantir que lint, testes e build passem antes de qualquer merge em `master` |
| **Produtividade** | Usar OpenCode para executar tarefas automaticamente via comentários (`/oc`) |
| **Confiabilidade** | Revisão automatizada de PRs para detectar problemas cedo |
| **Manutenibilidade** | Workflows padronizados, com cache e paralelismo, fáceis de estender |
| **Segurança** | Permissões mínimas em cada workflow, sem exposição de secrets |

### 1.3 Sistemas Envolvidos

| Sistema | Relação com os Workflows |
|---|---|
| **GitHub Actions** | Runner onde os workflows executam |
| **gopayground (código)** | Código-fonte que será buildado, testado e lintado |
| **OpenCode App** | Já instalado no repositório; será chamado pelos workflows |
| **golangci-lint** | Ferramenta de lint para Go |
| **Go toolchain** | Versão 1.26.0 — compilação e testes |
| **Anthropic API** | Modelo de IA usado pelo OpenCode (Claude) |

### 1.4 Restrições Conhecidas

- O repositório já possui o **OpenCode App** instalado (configurado via
  `opencode github install`)
- O arquivo `opencode.json` já existe com `default_agent: "planner"`
- O projeto usa **Go 1.26.0** — os workflows devem usar essa versão
- `Makefile` com targets: `test`, `lint`, `build`, `build-ui`, `build-producer`
- Docker Compose com Kafka, Redis, Floci (DynamoDB local), OpenTelemetry — **não
  será usado no CI** (os testes unitários não dependem de infra externa)
- **Não existe diretório `.github/` ainda** — será criado
- Os workflows **não devem conter secrets hardcoded** — usar GitHub Secrets

---

## 2. Trabalho a Ser Feito / Definition of Done (DoD)

### 2.1 O Que Será Entregue

Quatro arquivos de workflow em `.github/workflows/`:

| # | Arquivo | Descrição |
|---|---|---|
| 1 | `ci.yml` | CI padrão: lint → test → build em pushes e PRs para master |
| 2 | `opencode.yml` | Resposta a comentários `/oc` e `/opencode` em issues e PRs |
| 3 | `pr-review.yml` | Revisão automática de PRs com OpenCode ao abrir/sincronizar |
| 4 | `opencode-triage.yml` | (Opcional) Triagem automática de novas issues |

Além disso, os **secrets** necessários devem estar documentados para configuração
no repositório GitHub.

### 2.2 Critérios de Aceitação (DoD)

- [ ] **WF-01**: `ci.yml` executa lint, test e build em todo push para `master`
  e todo PR aberto contra `master`
- [ ] **WF-02**: `ci.yml` usa cache de Go modules para acelerar execuções
- [ ] **WF-03**: `ci.yml` falha se qualquer job falhar (bloqueia merge)
- [ ] **WF-04**: `opencode.yml` é acionado quando um comentário contém `/oc`
  ou `/opencode`
- [ ] **WF-05**: `opencode.yml` tem as permissões mínimas para operar
  (`id-token: write`)
- [ ] **WF-06**: `pr-review.yml` executa automaticamente ao abrir, sincronizar
  ou reabrir um PR
- [ ] **WF-07**: `pr-review.yml` usa um prompt personalizado de revisão
  específico para o projeto
- [ ] **WF-08**: `opencode-triage.yml` (se incluso) tria issues novas com
  filtro de conta >= 30 dias
- [ ] **WF-09**: Todos os workflows usam `actions/checkout@v6`
- [ ] **WF-10**: Nenhum secret hardcoded — todos via `secrets.ANTHROPIC_API_KEY`
- [ ] **WF-11**: Workflow de CI não depende de infra externa (Kafka, Redis, etc.)
- [ ] **WF-12**: Diretório `.github/workflows/` criado com os 4 arquivos

### 2.3 Fora de Escopo

- **Deploy automático** (CD) — não será implementado nesta spec
- **Integração com Docker Hub ou容器 registry**
- **Testes de integração com Kafka/Redis no CI** (exigiria serviços containers)
- **Análise de segurança (SAST/DAST)** — pode ser adicionada em spec futura
- **Notificações em Slack/Discord** — pode ser adicionada em spec futura
- **Code coverage reports** — pode ser adicionado em spec futura

---

## 3. Especificação

### 3.1 Visão Geral da Arquitetura de Workflows

```
┌─────────────────────────────────────────────────────────────────────────┐
│                            GitHub Events                                │
│                                                                         │
│  push master    PR → master    issue_comment    PR opened    issue opened│
│       │              │              │               │              │    │
│       ▼              ▼              ▼               ▼              ▼    │
│  ┌────────┐   ┌────────┐   ┌────────────┐   ┌──────────┐   ┌─────────┐ │
│  │ CI     │   │ CI     │   │ OpenCode    │   │ PR Review│   │ Triage  │ │
│  │ (lint, │   │ (lint, │   │ (/oc,       │   │ (auto    │   │ (auto   │ │
│  │  test, │   │  test, │   │  /opencode) │   │  review) │   │  triage)│ │
│  │  build)│   │  build)│   │             │   │          │   │         │ │
│  └────────┘   └────────┘   └──────┬──────┘   └─────┬────┘   └────┬────┘ │
│                                    │                │             │      │
│                                    ▼                ▼             ▼      │
│                           ┌──────────────────────────────────────┐       │
│                           │         OpenCode Action               │       │
│                           │  (anomalyco/opencode/github@latest)   │       │
│                           │                                       │       │
│                           │  Model: Claude Sonnet 4               │       │
│                           │  Agent: planner (default)             │       │
│                           └──────────────────────────────────────┘       │
│                                                                          │
│  Secrets:                                                                │
│  ┌──────────────────────────────┐                                        │
│  │ ANTHROPIC_API_KEY            │                                        │
│  │ GITHUB_TOKEN (automático)    │                                        │
│  └──────────────────────────────┘                                        │
└─────────────────────────────────────────────────────────────────────────┘
```

### 3.2 Workflow 1: CI — `.github/workflows/ci.yml`

#### 3.2.1 Trigger

```yaml
on:
  push:
    branches: [master]
  pull_request:
    branches: [master]
```

#### 3.2.2 Jobs

**Job 1: lint**

| Campo | Valor |
|---|---|
| `runs-on` | `ubuntu-latest` |
| Steps | `actions/checkout@v6` → `actions/setup-go@v5` → Cache Go modules → `golangci-lint run ./...` |
| Cache | `actions/cache@v4` para `~/go/pkg/mod` e `~/.cache/golangci-lint` |
| Go version | `1.26.x` (usando `go-version-file: go.mod` para ler a versão do módulo) |

**Job 2: test**

| Campo | Valor |
|---|---|
| `runs-on` | `ubuntu-latest` |
| `needs` | `lint` (executa em paralelo, mas lint conclui primeiro visualmente) |
| Steps | `actions/checkout@v6` → `actions/setup-go@v5` → Cache → `make test` |
| Go version | `1.26.x` |
| Nota | Usa `-race -count=1 -timeout=120s` conforme Makefile. Sem dependência externa. |

**Job 3: build**

| Campo | Valor |
|---|---|
| `runs-on` | `ubuntu-latest` |
| `needs` | `test` |
| Steps | `actions/checkout@v6` → `actions/setup-go@v5` → Cache → `make build build-ui build-producer` |
| Go version | `1.26.x` |
| Nota | Compila consumer, ui e producer para verificar se tudo compila |

#### 3.2.3 Decisões de Design

| Decisão | Justificativa |
|---|---|
| **Single Go version (não matrix)** | O projeto é fixo em Go 1.26.0 (definido no `go.mod`). Matrix adicionaria complexidade sem benefício. Fácil de adicionar depois se necessário. |
| **Jobs sequenciais (lint → test → build)** | Build só faz sentido se os testes passarem. Lint roda primeiro por ser rápido. |
| **Cache de Go modules** | Reduz tempo de setup de ~2min para ~10s. |
| **Sem matrix de OS** | O projeto não tem código dependente de SO específico. Ubuntu é suficiente. |
| **`go-version-file: go.mod`** | Evita duplicar a versão do Go no workflow. Sempre sincronizada com o `go.mod`. |

### 3.3 Workflow 2: OpenCode Integration — `.github/workflows/opencode.yml`

#### 3.3.1 Trigger

```yaml
on:
  issue_comment:
    types: [created]
  pull_request_review_comment:
    types: [created]
```

#### 3.3.2 Condição de Execução

O job só executa se o comentário contiver `/oc` ou `/opencode`:

```yaml
if: |
  contains(github.event.comment.body, '/oc') ||
  contains(github.event.comment.body, '/opencode')
```

#### 3.3.3 Job: opencode

| Campo | Valor |
|---|---|
| `runs-on` | `ubuntu-latest` |
| `permissions` | `id-token: write` (mínimo para OpenCode) |
| Steps | `actions/checkout@v6` → `anomalyco/opencode/github@latest` |
| Model | `anthropic/claude-sonnet-4-20250514` |
| Agent | `planner` (conforme `opencode.json`) |
| Env | `ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}` |

#### 3.3.4 Decisões de Design

| Decisão | Justificativa |
|---|---|
| **Permissão mínima `id-token: write`** | Segurança: o OpenCode não precisa de `contents: write` ou `pull-requests: write` para responder em comentários. Requests de escrita (criar branch/PR) usam o token do App. |
| **Sem `share: true` explícito** | O padrão do OpenCode é `true` para repositórios públicos — suficiente. |
| **Sem `github_token` personalizado** | Usa o token de instalação do OpenCode App automaticamente (já instalado). |
| **Agent = planner** | Coerente com a configuração do projeto (`opencode.json`). |

### 3.4 Workflow 3: PR Review — `.github/workflows/pr-review.yml`

#### 3.4.1 Trigger

```yaml
on:
  pull_request:
    types: [opened, synchronize, reopened]
```

#### 3.4.2 Job: review

| Campo | Valor |
|---|---|
| `runs-on` | `ubuntu-latest` |
| `permissions` | `id-token: write` |
| Steps | `actions/checkout@v6` → `anomalyco/opencode/github@latest` |
| Model | `anthropic/claude-sonnet-4-20250514` |
| `use_github_token` | `true` |
| Env | `ANTHROPIC_API_KEY`, `GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}` |

#### 3.4.3 Prompt Personalizado

O workflow deve usar um prompt específico para o contexto do projeto:

```yaml
prompt: |
  Revise este pull request para o projeto gopayground (Go 1.26).

  Critérios de revisão:
  1. **Qualidade do código Go**: idiomático, interfaces pequenas, composição.
     - `context.Context` deve ser passado em operações bloqueantes.
     - Erros devem usar `fmt.Errorf("%w", err)` para wrapping.
     - Preferir simplicidade sobre abstração prematura.
  2. **Testes**: o PR inclui testes table-driven? Usa `-race`? Subtests?
     Cobertura mínima para novas funcionalidades?
  3. **Sistemas distribuídos**: tratamento de timeout, retry, backoff,
     idempotência para operações com Kafka/Redis/DynamoDB.
  4. **Logs estruturados**: pontos críticos com `slog.Logger`.
  5. **Segurança**: injeção, validação de entrada, permissões mínimas.
  6. **Breaking changes**: mudanças em interfaces públicas (`pkg/` ou
     `internal/`) que possam afetar outros componentes.
  7. **Documentação**: README, ADRs ou comentários atualizados?

  Para cada problema encontrado, classifique como:
  - ❌ **Blocker**: impede o merge (bug, segurança, breaking change)
  - ⚠️ **Warning**: deve ser corrigido, mas não bloqueia
  - 💡 **Suggestion**: melhoria opcional

  Inclua o trecho de código relevante em cada apontamento.
```

#### 3.4.4 Decisões de Design

| Decisão | Justificativa |
|---|---|
| **`use_github_token: true`** | Necessário para que o OpenCode possa postar o review como comentário no PR usando o `GITHUB_TOKEN`. |
| **Prompt personalizado** | Adaptado ao stack Go + Kafka + Redis + DynamoDB do projeto. Inclui critérios de sistemas distribuídos. |
| **Trigger em opened/synchronize/reopened** | Cobre criação, atualização e reabertura de PRs. |

### 3.5 Workflow 4 (Opcional): Issue Triage — `.github/workflows/opencode-triage.yml`

#### 3.5.1 Trigger

```yaml
on:
  issues:
    types: [opened]
```

#### 3.5.2 Jobs

**Job 1: check-account-age**

Filtra contas com menos de 30 dias (reduz spam):

```yaml
- uses: actions/github-script@v7
  id: check
  with:
    script: |
      const user = await github.rest.users.getByUsername({
        username: context.payload.issue.user.login
      });
      const created = new Date(user.data.created_at);
      const days = (Date.now() - created) / (1000 * 60 * 60 * 24);
      return days >= 30;
    result-encoding: string
```

**Job 2: triage**

Só executa se `check.outputs.result == 'true'`. Usa OpenCode para analisar a
issue e fornecer orientação.

#### 3.5.3 Prompt

```yaml
prompt: |
  Revise esta issue para o projeto gopayground.
  - Se houver documentação relevante, mencione links.
  - Se for um bug claro, sugira possíveis causas e soluções.
  - Se for um pedido de feature, avalie a complexidade.
  - Caso contrário, não comente.
```

### 3.6 Secrets Necessários

| Secret | Workflows que Usam | Descrição |
|---|---|---|
| `ANTHROPIC_API_KEY` | opencode.yml, pr-review.yml, opencode-triage.yml | Chave de API para o modelo Claude Sonnet 4 |
| `GITHUB_TOKEN` | pr-review.yml | Token automático do GitHub Actions (não precisa criar) |

#### Como Configurar

1. Acesse **Settings → Secrets and variables → Actions** no repositório
   `github.com/Daniel-Dos/gopayground`
2. Adicione `ANTHROPIC_API_KEY` com o valor da chave da Anthropic
3. O `GITHUB_TOKEN` é automático — não precisa configurar

### 3.7 Permissões Mínimas

| Workflow | `permissions` | Justificativa |
|---|---|---|
| `ci.yml` | `contents: read` (padrão) | Só precisa ler o código e publicar status checks |
| `opencode.yml` | `id-token: write` | OpenCode precisa do token de instalação do App |
| `pr-review.yml` | `id-token: write` + `GITHUB_TOKEN` | OpenCode precisa postar review no PR |
| `opencode-triage.yml` | `id-token: write`, `issues: write` | OpenCode precisa comentar na issue |

> **Nota**: Workflows sem `permissions` explícitas herdam `contents: read` e
> `issues: read` por padrão no GitHub (desde 2023). Para `pr-review.yml`,
> `use_github_token: true` faz o OpenCode usar o `GITHUB_TOKEN` que já tem
> as permissões adequadas.

### 3.8 Estrutura de Arquivos Esperada

```
.github/
  workflows/
    ci.yml
    opencode.yml
    pr-review.yml
    opencode-triage.yml       # (opcional)
```

---

## 4. Riscos e Mitigações

### 4.1 Tabela de Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|---|---|---|---|
| R-01 | **Chave ANTHROPIC_API_KEY exposta** | Baixa | Crítico | Usar GitHub Secrets; nunca logar secrets; rodar `trivy` ou `gitleaks` se disponível |
| R-02 | **Pipeline CI lento (>10min)** | Média | Médio | Cache de Go modules; usar `go-version-file`; paralelizar lint e test |
| R-03 | **Falso positivo no lint bloqueia PR** | Baixa | Baixo | Revisar config do golangci-lint; permitir override manual |
| R-04 | **OpenCode não responde (timeout da API)** | Média | Médio | OpenCode tem retry interno; monitorar logs do workflow |
| R-05 | **PR Review com prompt genérico** | Baixa | Médio | Usar prompt personalizado (já definido na spec) |
| R-06 | **Workflow de triagem marcando issues legítimas como spam** | Baixa | Médio | Filtro de conta >= 30 dias; reverter manualmente se necessário |
| R-07 | **Mudança na action `anomalyco/opencode/github`** | Baixa | Alto | Fixar versão com SHA256 ou `@v1`; testar após atualizações |
| R-08 | **Concorrência de múltiplos workflows no mesmo runner** | Média | Baixo | GitHub Actions gerencia fila; usar `concurrency` no CI se necessário |

### 4.2 Trade-offs

| Decisão | Prós | Contras |
|---|---|---|
| **Versão única de Go (não matrix)** | Workflow simples, rápido, sem falsos positivos | Não detecta problemas em versões futuras do Go |
| **Jobs sequenciais** | Clareza, pipeline lógico (lint → test → build) | Mais lento que fully parallel |
| **`actions/checkout@v6`** | Versão mais recente, suporte a longpaths | Pequena chance de breaking change (mitigada por SHA pinning) |
| **OpenCode com `id-token: write` mínimo** | Segurança: token sem escopo extra | Pode exigir permissão adicional se o App não estiver instalado |
| **Filtro de 30 dias no triage** | Reduz spam significativamente | Pode bloquear issues legítimas de contas novas |

### 4.3 Planos de Contingência

| Risco | Ação |
|---|---|
| CI falha por problema no runner | Re-executar workflow manualmente via GitHub UI |
| OpenCode action quebra por atualização | Pinar SHA da action; criar fallback com script manual |
| ANTHROPIC_API_KEY expirada ou rotacionada | Documentar procedimento de rotação; alerta no Discord/Slack |

---

## Anexo A: Conteúdo Referencial dos Workflows

### A.1 `ci.yml` (esboço)

```yaml
name: CI
on:
  push:
    branches: [master]
  pull_request:
    branches: [master]

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: latest

  test:
    runs-on: ubuntu-latest
    needs: lint
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - name: Test
        run: make test

  build:
    runs-on: ubuntu-latest
    needs: test
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - name: Build
        run: make build build-ui build-producer
```

### A.2 `opencode.yml` (esboço)

```yaml
name: opencode
on:
  issue_comment:
    types: [created]
  pull_request_review_comment:
    types: [created]

jobs:
  opencode:
    if: |
      contains(github.event.comment.body, '/oc') ||
      contains(github.event.comment.body, '/opencode')
    runs-on: ubuntu-latest
    permissions:
      id-token: write
    steps:
      - uses: actions/checkout@v6
        with:
          persist-credentials: false
      - uses: anomalyco/opencode/github@latest
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        with:
          model: anthropic/claude-sonnet-4-20250514
```

### A.3 `pr-review.yml` (esboço)

```yaml
name: opencode-review
on:
  pull_request:
    types: [opened, synchronize, reopened]

jobs:
  review:
    runs-on: ubuntu-latest
    permissions:
      id-token: write
    steps:
      - uses: actions/checkout@v6
        with:
          persist-credentials: false
      - uses: anomalyco/opencode/github@latest
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        with:
          model: anthropic/claude-sonnet-4-20250514
          use_github_token: true
          prompt: |
            Revise este pull request para o projeto gopayground (Go 1.26).

            Critérios de revisão:
            1. **Qualidade do código Go**: idiomático, interfaces pequenas, composição.
               - `context.Context` deve ser passado em operações bloqueantes.
               - Erros devem usar `fmt.Errorf("%w", err)` para wrapping.
               - Preferir simplicidade sobre abstração prematura.
            2. **Testes**: o PR inclui testes table-driven? Usa `-race`? Subtests?
               Cobertura mínima para novas funcionalidades?
            3. **Sistemas distribuídos**: tratamento de timeout, retry, backoff,
               idempotência para operações com Kafka/Redis/DynamoDB.
            4. **Logs estruturados**: pontos críticos com `slog.Logger`.
            5. **Segurança**: injeção, validação de entrada, permissões mínimas.
            6. **Breaking changes**: mudanças em interfaces públicas (`pkg/` ou
               `internal/`) que possam afetar outros componentes.
            7. **Documentação**: README, ADRs ou comentários atualizados?

            Para cada problema encontrado, classifique como:
            - ❌ **Blocker**: impede o merge (bug, segurança, breaking change)
            - ⚠️ **Warning**: deve ser corrigido, mas não bloqueia
            - 💡 **Suggestion**: melhoria opcional

            Inclua o trecho de código relevante em cada apontamento.
```

### A.4 `opencode-triage.yml` (esboço — opcional)

```yaml
name: Issue Triage
on:
  issues:
    types: [opened]

jobs:
  check-account-age:
    runs-on: ubuntu-latest
    outputs:
      result: ${{ steps.check.outputs.result }}
    steps:
      - uses: actions/github-script@v7
        id: check
        with:
          script: |
            const user = await github.rest.users.getByUsername({
              username: context.payload.issue.user.login
            });
            const created = new Date(user.data.created_at);
            const days = (Date.now() - created) / (1000 * 60 * 60 * 24);
            return days >= 30;
          result-encoding: string

  triage:
    needs: check-account-age
    if: needs.check-account-age.outputs.result == 'true'
    runs-on: ubuntu-latest
    permissions:
      id-token: write
      issues: write
    steps:
      - uses: actions/checkout@v6
        with:
          persist-credentials: false
      - uses: anomalyco/opencode/github@latest
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        with:
          model: anthropic/claude-sonnet-4-20250514
          prompt: |
            Revise esta issue para o projeto gopayground.
            - Se houver documentação relevante, mencione links.
            - Se for um bug claro, sugira possíveis causas e soluções.
            - Se for um pedido de feature, avalie a complexidade.
            - Caso contrário, não comente.
```

---

## Anexo B: Considerações de Manutenção

### B.1 Boas Práticas para Manutenção dos Workflows

| Prática | Descrição |
|---|---|
| **Manter versões fixas** | Usar `actions/checkout@v6` (major version) ou SHA específico, nunca `@main` |
| **Documentar mudanças** | Alterações em workflows devem passar por PR e serem revisadas |
| **Testar workflows localmente** | Usar `act` (nektos/act) para testar workflows antes de commitar |
| **Secrets review** | Revisar periodicamente se há secrets expostos nos logs do Actions |
| **Monitorar falhas** | Configurar notificações de falha do GitHub Actions (email ou Slack) |
| **Simplificar antes de otimizar** | Workflows pequenos são mais fáceis de debugar. Adicionar complexidade só quando necessário |

### B.2 Dependências Externas

| Recurso | Versão/Dependência | Atualização |
|---|---|---|
| `actions/checkout` | `@v6` | Major version pin — seguro |
| `actions/setup-go` | `@v5` | Major version pin — seguro |
| `golangci/golangci-lint-action` | `@v6` | Major version pin — seguro |
| `anomalyco/opencode/github` | `@latest` | **Risco**: considerar pinar SHA ou `@v1` quando estável |
| `actions/github-script` | `@v7` | Major version pin — seguro |

### B.3 Estratégia de Versionamento de Actions

Recomendação: nas actions do OpenCode (`anomalyco/opencode/github`), considerar
usar `@latest` inicialmente (como documentado oficialmente) e migrar para SHA
fixo quando a action atingir maturidade, para maior reprodutibilidade.

---

## Anexo C: Guia de Configuração Inicial

Passos necessários no repositório GitHub após a criação dos workflows:

1. **Verificar instalação do OpenCode App**
   - Acessar `github.com/apps/opencode-agent`
   - Confirmar que está instalado no repositório `Daniel-Dos/gopayground`

2. **Adicionar secret `ANTHROPIC_API_KEY`**
   - Settings → Secrets and variables → Actions
   - New repository secret → Nome: `ANTHROPIC_API_KEY`, Valor: (chave da Anthropic)

3. **Verificar permissões do `GITHUB_TOKEN`**
   - Settings → Actions → General → Workflow permissions
   - Marcar: "Allow GitHub Actions to create and approve pull requests"
   - (Necessário para o OpenCode criar branches e PRs)

4. **Push dos workflows**
   - Commit e push dos arquivos em `.github/workflows/`
   - Verificar execução inicial na aba Actions do GitHub
