# Workflows GitHub Actions — gopayground

Este documento descreve os quatro workflows de GitHub Actions configurados no
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

## 2. Workflow OpenCode Integration — `opencode.yml`

Este workflow permite que qualquer pessoa com acesso ao repositorio invoque o
OpenCode Agent diretamente de comentarios em issues e pull requests. Basta
escrever `/oc` ou `/opencode` no inicio ou no meio do comentario para que o
workflow dispare.

### 2.1 Trigger

O workflow escuta dois eventos:

```yaml
on:
  issue_comment:
    types: [created]
  pull_request_review_comment:
    types: [created]
```

O evento `issue_comment` captura comentarios em issues e comentarios gerais em
PRs. O evento `pull_request_review_comment` captura comentarios feitos durante
uma revisao de codigo (review comments). Ambos usam `types: [created]` para
executar apenas quando o comentario e criado, ignorando edicoes ou delecoes.

### 2.2 Filtro de Conteudo

O job so executa se o corpo do comentario conter `/oc` ou `/opencode`:

```yaml
if: |
  contains(github.event.comment.body, '/oc') ||
  contains(github.event.comment.body, '/opencode')
```

Isso usa `contains()` do GitHub Expression Language, que e case-sensitive e
busca a substring em qualquer posicao do texto. Comentarios que nao contenham
nenhum dos marcadores sao ignorados sem custo de execucao.

### 2.3 Job OpenCode

```yaml
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

A permissao `id-token: write` e o minimo necessario para o OpenCode obter o
token de instalacao do OpenCode App. O workflow nao declara permissoes de
escrita em `contents` ou `pull-requests` porque o OpenCode App ja esta
instalado no repositorio e gerencia suas proprias permissoes de forma
independente.

O model `anthropic/claude-sonnet-4-20250514` e o Claude Sonnet 4, que oferece
um equilibrio entre qualidade de resposta e custo. O agente padrao e o
`planner`, conforme definido no `opencode.json` do projeto.

### 2.4 Como Usar

Em qualquer issue ou pull request, escreva um comentario contendo `/oc` ou
`/opencode` seguido da instrucao desejada. Por exemplo:

- `/oc analise o arquivo main.go e sugira melhorias`
- `/opencode crie uma spec para adicionar suporte a PostgreSQL`
- `/oc explique o fluxo de processamento de pagamentos`

O OpenCode respondera no mesmo thread do comentario. O tempo de resposta
depende da complexidade da tarefa e da latencia da API Anthropic, mas
tipicamente leva entre 30 segundos e 2 minutos.

---

## 3. Workflow PR Review — `pr-review.yml`

O workflow de revisao automatica de pull requests usa o OpenCode para analisar
cada PR aberto contra `master` e postar um review com apontamentos
classificados por gravidade. O objetivo e detectar problemas cedo e reduzir a
carga de revisao manual.

### 3.1 Trigger

```yaml
on:
  pull_request:
    types: [opened, synchronize, reopened]
```

Os tres tipos de evento cobrem o ciclo de vida completo de um PR:

- `opened`: quando o PR e criado pela primeira vez.
- `synchronize`: quando novos commits sao adicionados ao branch do PR.
- `reopened`: quando um PR fechado e reaberto.

Note que o evento `pull_request` com `types: [opened]` ja cobre o cenario
inicial; `synchronize` garante que revisoes sejam atualizadas a cada novo
commit, e `reopened` cobre PRs que foram fechados e reabertos.

### 3.2 Concorrencia

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
```

Assim como no CI, execucoes concorrentes para o mesmo PR (por exemplo, varios
commits rapidos) sao canceladas automaticamente. Apenas a execucao mais recente
prossegue.

### 3.3 Job Review

```yaml
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
        with:
          model: anthropic/claude-sonnet-4-20250514
          use_github_token: true
          prompt: |
            Revise este pull request para o projeto gopayground (Go 1.26).

            Criterios de revisao:
            1. **Qualidade do codigo Go**: idiomatico, interfaces pequenas, composicao.
               - `context.Context` deve ser passado em operacoes bloqueantes.
               - Erros devem usar `fmt.Errorf("%w", err)` para wrapping.
               - Preferir simplicidade sobre abstracao prematura.
            2. **Testes**: o PR inclui testes table-driven? Usa `-race`? Subtests?
               Cobertura minima para novas funcionalidades?
            3. **Sistemas distribuidos**: tratamento de timeout, retry, backoff,
               idempotencia para operacoes com Kafka/Redis/DynamoDB.
            4. **Logs estruturados**: pontos criticos com `slog.Logger`.
            5. **Seguranca**: injecao, validacao de entrada, permissoes minimas.
            6. **Breaking changes**: mudancas em interfaces publicas (`pkg/` ou
               `internal/`) que possam afetar outros componentes.
            7. **Documentacao**: README, ADRs ou comentarios atualizados?

            Para cada problema encontrado, classifique como:
            - **Blocker**: impede o merge (bug, seguranca, breaking change)
            - **Warning**: deve ser corrigido, mas nao bloqueia
            - **Suggestion**: melhoria opcional

            Inclua o trecho de codigo relevante em cada apontamento.
```

### 3.4 O Parametro `use_github_token`

O parametro `use_github_token: true` instrui o OpenCode a usar o
`GITHUB_TOKEN` padrao do GitHub Actions para postar o review como um
comentario no PR. Sem essa flag, o OpenCode nao teria permissao para escrever
no pull request, e o review ficaria invisivel dentro do workflow log.

O `GITHUB_TOKEN` e injetado automaticamente pelo GitHub Actions; nao e
necessario configura-lo manualmente. O token tem escopo limitado ao
repositorio onde o workflow executa e e valido apenas durante a execucao do
workflow.

### 3.5 Criterios de Revisao

O prompt personalizado define sete criterios de avaliacao, cada um mapeado
diretamente para as convencoes tecnicas do projeto:

1. **Qualidade do codigo Go**: reflete as convencoes estabelecidas no
   `AGENTS.md` — interfaces pequenas, composicao, `context.Context` em
   operacoes bloqueantes, erros com `fmt.Errorf("%w", err)`.

2. **Testes**: verifica se o PR segue o padrao de testes table-driven com
   subtests, pratica definida como obrigatoria para o projeto.

3. **Sistemas distribuidos**: critico para um sistema que usa Kafka, Redis e
   DynamoDB. O revisor busca tratamento de timeout, retry com backoff e
   idempotencia.

4. **Logs estruturados**: o projeto usa `slog.Logger` para logs estruturados
   nos pontos criticos. O revisor verifica se novos codigos seguem essa
   convencao.

5. **Seguranca**: validacao de entrada, protecao contra injecao e principio
   do menor privilegio.

6. **Breaking changes**: mudancas em interfaces publicas de `pkg/` ou
   `internal/` que possam afetar outros componentes do sistema.

7. **Documentacao**: verifica se o PR atualiza README, ADRs ou comentarios
   quando necessario.

### 3.6 Classificacao dos Apontamentos

Cada problema encontrado e classificado em uma de tres categorias:

- **Blocker**: problemas que impedem o merge. Incluem bugs de seguranca,
  vulnerabilidades, breaking changes sem discussao previa e violacoes graves
  das convencoes do projeto.
- **Warning**: problemas que devem ser corrigidos antes do merge, mas nao
  sao impeditivos absolutos. Exemplos: log ausente em ponto critico, falta de
  tratamento de erro em operacao de IO, teste sem subtest.
- **Suggestion**: melhorias opcionais que nao impactam a correcao ou
  seguranca. Exemplos: renomear variavel para maior clareza, extrair funcao
  muito longa, adicionar comentario de documentacao.

Cada apontamento inclui o trecho de codigo relevante, o que permite ao autor
do PR identificar exatamente o local da ocorrencia sem precisar ler o diff
inteiro.

---

## 4. Workflow Issue Triage — `opencode-triage.yml`

O workflow de triagem de issues automatiza a resposta inicial a novas issues
abertas no repositorio. Ele usa um filtro anti-spam baseado na idade da conta
do GitHub e, para issues legitimas, invoca o OpenCode para analisar o conteudo
e fornecer uma resposta orientativa.

### 4.1 Trigger

```yaml
on:
  issues:
    types: [opened]
```

Executa apenas quando uma nova issue e criada. Issues reabertas ou editadas nao
disparam o workflow.

### 4.2 Filtro Anti-Spam: Job `check-account-age`

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

### 4.3 Job `triage`

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
        ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
      with:
        model: anthropic/claude-sonnet-4-20250514
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

## 5. Configuracao Inicial

Para que os workflows funcionem corretamente, tres configuracoes sao
necessarias no repositorio GitHub.

### 5.1 Secrets

Apenas um secret precisa ser criado manualmente:

| Secret | Workflows que usam | Descricao |
|---|---|---|
| `ANTHROPIC_API_KEY` | opencode.yml, pr-review.yml, opencode-triage.yml | Chave de API para o modelo Claude Sonnet 4 da Anthropic |

O `GITHUB_TOKEN` e automaticamente fornecido pelo GitHub Actions em cada
execucao de workflow e nao requer configuracao manual.

Para adicionar o `ANTHROPIC_API_KEY`:

1. Acesse `https://github.com/Daniel-Dos/gopayground/settings/secrets/actions`
2. Clique em "New repository secret"
3. Nome: `ANTHROPIC_API_KEY`
4. Valor: a chave de API obtida no console da Anthropic
5. Clique em "Add secret"

### 5.2 Permissoes do GITHUB_TOKEN

O `GITHUB_TOKEN` padrao do GitHub Actions precisa de permissao para criar e
aprovar pull requests, pois o OpenCode pode precisar criar branches e PRs
durante a execucao.

1. Acesse `https://github.com/Daniel-Dos/gopayground/settings/actions`
2. Em "Workflow permissions", selecione "Allow GitHub Actions to create and
   approve pull requests"
3. Salve as alteracoes

### 5.3 OpenCode App

O OpenCode App deve estar instalado no repositorio. Para verificar:

1. Acesse `https://github.com/apps/opencode-agent`
2. Confirme que o app esta instalado na organizacao ou no usuario
   `Daniel-Dos` e que tem acesso ao repositorio `gopayground`

O arquivo `opencode.json` na raiz do projeto ja define `default_agent:
"planner"`, que e o agente utilizado por todos os workflows que invocam o
OpenCode.

### 5.4 Sumario de Permissoes por Workflow

| Workflow | Permissoes | Justificativa |
|---|---|---|
| `ci.yml` | `contents: read` | So precisa ler o codigo e publicar status checks |
| `opencode.yml` | `id-token: write` | OpenCode precisa do token de instalacao do App |
| `pr-review.yml` | `id-token: write` | OpenCode precisa postar review no PR (usa `GITHUB_TOKEN`) |
| `opencode-triage.yml` | `id-token: write`, `issues: write` | OpenCode precisa comentar na issue |

---

## 6. Manutencao

### 6.1 Versionamento de Actions

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

### 6.2 Como Testar Localmente com `act`

A ferramenta `act` (https://github.com/nektos/act) permite executar workflows
do GitHub Actions localmente, o que agiliza o desenvolvimento e调试 de novos
workflows sem precisar fazer push no repositorio.

Para testar o workflow de CI localmente:

```bash
act -W .github/workflows/ci.yml --pull=false
```

Para testar o workflow de PR review:

```bash
act pull_request -W .github/workflows/pr-review.yml
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
  a chave `ANTHROPIC_API_KEY` disponivel localmente.

### 6.3 Boas Praticas

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
