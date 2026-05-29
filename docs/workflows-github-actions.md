# Workflows GitHub Actions — gopayground

Este documento descreve os três workflows de GitHub Actions configurados no
repositório `github.com/Daniel-Dos/gopayground`. Cada workflow atende a um
propósito específico dentro do pipeline de desenvolvimento: integração contínua,
execução de comandos OpenCode via comentários, revisão automática de pull
requests e triagem de issues. O objetivo é centralizar a documentação operacional
para que qualquer desenvolvedor entenda o comportamento, os requisitos de
configuração e as boas práticas de manutenção.

---

## 1. Workflow CI — `ci.yml`

O workflow de CI é o guardião da qualidade do código. Ele executa três etapas
em sequência — lint, testes e build — em todo push para a branch `master` e em
toda abertura ou atualização de pull request contra `master`. Se qualquer etapa
falhar, o workflow falha por completo, impedindo o merge.

### 1.1 Trigger e Concorrencia

O workflow e acionado pelos eventos `push` e `pull_request`, ambos filtrados
para a branch `master`:

```yaml
on:
  push:
    branches: [master]
  pull_request:
    branches: [master]
```

A chave `concurrency` agrupa execucoes pelo nome do workflow combinado com a
referencia da branch ou PR. Quando um novo push chega enquanto uma execucao
anterior ainda esta rodando para a mesma branch, a execucao anterior e
cancelada automaticamente (`cancel-in-progress: true`). Isso evita desperdicio
de minutos de CI em commits intermediarios.

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
```

### 1.2 Permissoes

O workflow declara `contents: read` como permission unica. O GitHub Actions
atribui essa permissao por padrao desde 2023, mas a declaracao explicita
torna a intencao visivel e impede que uma futura configuracao global mais
permissiva afete este workflow sem revisao.

### 1.3 Jobs

Os tres jobs formam uma pipeline sequencial: `lint` → `test` → `build`. Cada
job depende do anterior (`needs`), o que significa que `test` so executa se
`lint` passar, e `build` so executa se `test` passar.

#### Job lint

Executa golangci-lint na versao `v1.64.0` sobre todo o codigo do repositorio.
O timeout de 5 minutos e suficiente para projetos Go de medio porte; se o
lint ultrapassar esse limite, o job falha.

```yaml
lint:
  runs-on: ubuntu-latest
  timeout-minutes: 5
  steps:
    - uses: actions/checkout@v6
      with:
        persist-credentials: false
    - uses: actions/setup-go@v5
      with:
        go-version-file: go.mod
        cache: true
    - name: golangci-lint
      uses: golangci/golangci-lint-action@v6
      with:
        version: v1.64.0
```

Dois detalhes importantes:

- `go-version-file: go.mod` faz o setup-go ler a versao do Go diretamente do
  arquivo `go.mod`. Isso elimina a duplicacao de versao entre o workflow e o
  modulo Go. Quando o `go.mod` atualizar para `1.27.0` ou outra versao, o CI
  automaticamente usara a nova versao.
- `cache: true` no setup-go habilita o cache nativo de modulos Go, reduzindo
  o tempo de setup de cerca de 2 minutos para poucos segundos em execucoes
  subsequentes.

A versao do golangci-lint esta fixada em `v1.64.0` (nao `latest`). Isso evita
que uma atualizacao da ferramenta introduza novos linters ou regras que
quebrem o CI inesperadamente. A versao pode ser atualizada manualmente quando
desejado.

#### Job test

Executa a suite de testes com o comando `make test`, que por sua vez roda
`go test ./... -race -count=1 -timeout=120s`. O timeout do job e de 10
minutos, que comporta folgadamente os 120 segundos definidos no Makefile
mesmo para suites que crescam no futuro.

```yaml
test:
  runs-on: ubuntu-latest
  needs: lint
  timeout-minutes: 10
  steps:
    - uses: actions/checkout@v6
      with:
        persist-credentials: false
    - uses: actions/setup-go@v5
      with:
        go-version-file: go.mod
        cache: true
    - name: Test
      run: make test
```

Os testes nao dependem de infraestrutura externa (Kafka, Redis, DynamoDB).
Isso e uma decisao arquitetural do projeto: os testes unitarios usam
interfaces e mocks que isolam o codigo das dependencias. Workflows de CI que
exigissem containers subiriam o custo e a complexidade sem necessidade para
este estagio do projeto.

#### Job build

Compila os tres binarios do projeto: consumer (`go build -o bin/consumer
./cmd/consumer`), UI (`go build -o bin/ui ./cmd/ui`) e producer (`go build -o
bin/producer ./cmd/producer`). O comando `make build build-ui build-producer`
executa os tres builds. O timeout de 5 minutos e confortavel para projetos Go.

```yaml
build:
  runs-on: ubuntu-latest
  needs: test
  timeout-minutes: 5
  steps:
    - uses: actions/checkout@v6
      with:
        persist-credentials: false
    - uses: actions/setup-go@v5
      with:
        go-version-file: go.mod
        cache: true
    - name: Build
      run: make build build-ui build-producer
```

### 1.4 Decisoes de Design do CI

O workflow usa uma versao unica de Go em vez de uma matrix de versoes. O
projeto e fixo em Go 1.26.0 (definido no `go.mod`), e uma matrix com multiplas
versoes adicionaria tempo de execucao e complexidade sem retorno. Se no futuro
o projeto precisar suportar multiplas versoes, adicionar a matrix e trivial.

Os jobs sao sequenciais (lint → test → build) por uma razao pratica: nao faz
sentido executar o build se os testes falharam, e o lint, por ser rapido,
funciona como uma barreira inicial de qualidade. Um pipeline paralelo completo
executaria todos os jobs simultaneamente, o que poderia desperdicar recursos
se o codigo ja tiver erros de compilacao ou testes vermelhos.

`persist-credentials: false` esta presente em todos os steps de checkout para
evitar que o token padrao do GitHub Actions vaze para contextos onde nao e
necessario. O CI so precisa ler o codigo, nunca escrever de volta.

---

## 2. `opencode.yml` — Assistente OpenCode para PRs

### Trigger

- `pull_request`: executado ao abrir, sincronizar ou reabrir PRs para `master`
- `issue_comment`: executado quando um comentário contém `/oc`, `/opencode` ou `/run`
- `pull_request_review_comment`: executado quando um comentário em code review contém `/oc`, `/opencode` ou `/run`

### Fluxo de Execução

1. **Checkout** do repositório com `fetch-depth: 0` e `persist-credentials: false`
2. **Get changed files**: usa `tj-actions/changed-files@v46` para detectar arquivos alterados no PR
3. **Prepare prompt**: monta prompt dinâmico — lista de arquivos alterados para PRs, corpo do comentário para triggers manuais
4. **Run opencode**: executa o agente OpenCode com modelo `opencode/big-pickle` e prompt otimizado (sem `use_github_token`, o contexto vem do tj-actions)

### Permissões
```yaml
permissions:
  id-token: write
  contents: read
  pull-requests: write
  issues: write
  checks: write
```

### Segurança
- `OPENCODE_API_KEY` armazenada em `secrets.OPENCODE_API_KEY`
- `persist-credentials: false` no checkout
- `continue-on-error: true` no step de extração de arquivos (falha suave)
- `timeout-minutes: 10` para evitar execuções infinitas
- `concurrency` group com `cancel-in-progress: true`

---

## 3. Workflow Issue Triage — `opencode-triage.yml`

O workflow de triagem de issues automatiza a resposta inicial a novas issues
abertas no repositorio. Ele usa um filtro anti-spam baseado na idade da conta
do GitHub e, para issues legitimas, invoca o OpenCode para analisar o conteudo
e fornecer uma resposta orientativa.

### 3.1 Trigger

```yaml
on:
  issues:
    types: [opened]
```

Executa apenas quando uma nova issue e criada. Issues reabertas ou editadas nao
disparam o workflow.

### 3.2 Filtro Anti-Spam: Job `check-account-age`

O primeiro job consulta a API do GitHub para obter a data de criacao da conta
do usuario que abriu a issue. Se a conta tiver menos de 30 dias, a issue e
ignorada e o workflow termina sem acao.

```yaml
check-account-age:
  runs-on: ubuntu-latest
  permissions:
    contents: read
    issues: read
  outputs:
    result: ${{ steps.check.outputs.result }}
  steps:
    - uses: actions/github-script@v7
      id: check
      with:
        script: |
          try {
            const user = await github.rest.users.getByUsername({
              username: context.payload.issue.user.login
            });
            const created = new Date(user.data.created_at);
            const days = (Date.now() - created) / (1000 * 60 * 60 * 24);
            core.setOutput('result', days >= 30 ? 'true' : 'false');
          } catch (error) {
            console.log('Erro ao verificar usuario, permitindo triagem:', error.message);
            core.setOutput('result', 'true');
          }
        result-encoding: string
```

O script usa `actions/github-script@v7`, que fornece um ambiente Node.js com
o cliente octokit pre-configurado e autenticado. A funcao
`github.rest.users.getByUsername` faz uma chamada a API REST do GitHub para
obter os dados publicos do usuario.

Se a consulta falhar (por exemplo, se o usuario foi deletado ou a API retornar
erro), o bloco `catch` define `result` como `'true'`, permitindo que a triagem
prosseca. Essa e uma decisao de design conservadora: e melhor triar uma issue
duvidosa do que silenciosamente ignora-la.

O job `check-account-age` usa permissoes minimas (`contents: read` e
`issues: read`) porque so precisa consultar dados publicos e definir um output.

### 3.3 Job `triage`

O segundo job so executa se o output de `check-account-age` for `'true'`:

```yaml
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
        OPENCODE_API_KEY: ${{ secrets.OPENCODE_API_KEY }}
      with:
        model: opencode/big-pickle
        prompt: |
          Revise esta issue para o projeto gopayground.
          - Se houver documentacao relevante, mencione links.
          - Se for um bug claro, sugira possiveis causas e solucoes.
          - Se for um pedido de feature, avalie a complexidade.
          - Caso contrario, nao comente.
```

Este job requer `issues: write` porque o OpenCode precisa postar um comentario
na issue com a analise. O prompt e direto e objetivo: o OpenCode deve avaliar
se a issue e um bug, um pedido de feature ou algo que nao requer resposta.

O comportamento esperado:

- **Bug claro**: o OpenCode sugere possiveis causas e solucoes, alem de
  mencionar documentacao relevante se existir.
- **Pedido de feature**: o OpenCode avalia a complexidade e pode sugerir
  passos iniciais para implementacao.
- **Outros casos**: o OpenCode nao comenta, evitando poluir a issue com
  respostas genericas ou desnecessarias.

---

## 4. Configuracao Inicial

Para que os workflows funcionem corretamente, tres configuracoes sao
necessarias no repositorio GitHub.

### 4.1 Secrets

Apenas um secret precisa ser criado manualmente:

| Secret | Workflows que usam | Descricao |
|---|---|---|
| `OPENCODE_API_KEY` | opencode.yml, opencode-triage.yml | Chave de API para o modelo opencode/big-pickle |

O `GITHUB_TOKEN` e automaticamente fornecido pelo GitHub Actions em cada
execucao de workflow e nao requer configuracao manual.

Para adicionar o `OPENCODE_API_KEY`:

1. Acesse `https://github.com/Daniel-Dos/gopayground/settings/secrets/actions`
2. Clique em "New repository secret"
3. Nome: `OPENCODE_API_KEY`
4. Valor: a chave de API fornecida pelo OpenCode
5. Clique em "Add secret"

### 4.2 Permissoes do GITHUB_TOKEN

O `GITHUB_TOKEN` padrao do GitHub Actions precisa de permissao para criar e
aprovar pull requests, pois o OpenCode pode precisar criar branches e PRs
durante a execucao.

1. Acesse `https://github.com/Daniel-Dos/gopayground/settings/actions`
2. Em "Workflow permissions", selecione "Allow GitHub Actions to create and
   approve pull requests"
3. Salve as alteracoes

### 4.3 OpenCode App

O OpenCode App deve estar instalado no repositorio. Para verificar:

1. Acesse `https://github.com/apps/opencode-agent`
2. Confirme que o app esta instalado na organizacao ou no usuario
   `Daniel-Dos` e que tem acesso ao repositorio `gopayground`

O arquivo `opencode.json` na raiz do projeto ja define `default_agent:
"planner"`, que e o agente utilizado por todos os workflows que invocam o
OpenCode.

### 4.4 Sumario de Permissoes por Workflow

| Workflow | Permissoes | Justificativa |
|---|---|---|
| `ci.yml` | `contents: read` | So precisa ler o codigo e publicar status checks |
| `opencode.yml` | `id-token: write` | OpenCode precisa do token de instalacao do App |
| `opencode-triage.yml` | `id-token: write`, `issues: write` | OpenCode precisa comentar na issue |

---

## 5. Manutencao

### 5.1 Versionamento de Actions

Todas as actions de terceiros utilizam major version pin (`@v6`, `@v5`, `@v7`),
o que oferece um equilibrio entre estabilidade e recebimento de atualizacoes
nao-breakantes dentro da mesma major version:

| Action | Versao | Notas |
|---|---|---|
| `actions/checkout` | `@v6` | Major version pin — seguro |
| `actions/setup-go` | `@v5` | Major version pin — seguro |
| `golangci/golangci-lint-action` | `@v6` | Major version pin — seguro |
| `anomalyco/opencode/github` | `@latest` | Usar `@latest` conforme documentacao oficial; considerar pin SHA quando a action estabilizar |
| `actions/github-script` | `@v7` | Major version pin — seguro |

A action `anomalyco/opencode/github` usa `@latest` porque a documentacao
oficial recomenda essa abordagem e a action ainda esta em evolucao ativa.
Quando a action atingir uma versao estavel (`@v1` ou similar), recomenda-se
migrar para major version pin ou SHA fixo para maior reprodutibilidade.

### 5.2 Como Testar Localmente com `act`

A ferramenta `act` (https://github.com/nektos/act) permite executar workflows
do GitHub Actions localmente, o que agiliza o desenvolvimento e调试 de novos
workflows sem precisar fazer push no repositorio.

Para testar o workflow de CI localmente:

```bash
act -W .github/workflows/ci.yml --pull=false
```

Para testar o workflow de issue triage:

```bash
act issues -W .github/workflows/opencode-triage.yml
```

Notas sobre o uso do `act`:

- O `act` requer Docker instalado, pois executa os steps dentro de containers.
- Secrets locais podem ser definidos em um arquivo `.secrets` (nao versionado)
  ou passados via variavel de ambiente `ACTIONS_SECRETS`.
- O `act` nao suporta todos os eventos e recursos do GitHub Actions. Eventos
  como `issue_comment` podem ter suporte limitado dependendo da versao.
- Para testar workflows que usam `anomalyco/opencode/github`, e necessario ter
  a chave `OPENCODE_API_KEY` disponivel localmente.

### 5.3 Boas Praticas

- Toda alteracao em arquivos de workflow deve passar por revisao em pull
  request, assim como qualquer outra alteracao no codigo.
- Evite adicionar complexidade desnecessaria aos workflows. Workflows pequenos
  e focados sao mais faceis de debugar e manter.
- Monitore a aba Actions do GitHub periodicamente para identificar falhas
  recorrentes. Configure notificacoes de falha nas configuracoes do
  repositorio (Settings → Notifications).
- Revise periodicamente se ha secrets expostos nos logs do GitHub Actions.
  O GitHub automaticamente mascara secrets em logs, mas erros de
  implementacao podem vazar informacoes antes do mascaramento.
- Quando adicionar um novo workflow, mantenha este documento atualizado com a
  descricao, trigger, permissoes e secrets utilizados.
