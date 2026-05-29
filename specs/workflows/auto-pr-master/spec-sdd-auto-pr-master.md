# SDD — auto-pr-master-001: Auto-PR de Develop para Master (Release/Hotfix)

## Metadata

| Campo | Valor |
|---|---|
| **ID** | `auto-pr-master-001` |
| **Status** | Draft |
| **Data** | 2026-05-28 |
| **Autor** | Architect |
| **Repositório** | `github.com/Daniel-Dos/gopayground` |
| **SDD Relacionado** | [`specs/workflows/gitflow/spec-sdd-gitflow.md`](../gitflow/spec-sdd-gitflow.md) (gitflow-001) |
| **SDD Relacionado** | [`specs/workflows/auto-pr/spec-sdd-auto-pr.md`](../auto-pr/spec-sdd-auto-pr.md) (auto-pr-001) |
| **Stack** | GitHub Actions, Go 1.26, `gh` CLI |

---

## 1. Contexto e Motivação

### 1.1 Problema

O projeto **gopayground** já adotou o modelo GitFlow completo conforme spec
`gitflow-001`. O fluxo atual de PRs automatizados é:

| Origem | Destino | Gatilho | Tipo |
|---|---|---|---|
| `feature/*` | `develop` | Push (auto via `ci.yml`) | Automático |
| `release/*` | `develop` | Push (auto via `ci.yml`) | Automático |
| `hotfix/*` | `develop` | Push (auto via `ci.yml`) | Automático |
| `release/*` | `master` | Manual (desenvolvedor abre PR) | Manual |
| `hotfix/*` | `master` | Manual (desenvolvedor abre PR) | Manual |

**Problema:** O merge para `master` atualmente é manual. Quando um PR de
`release/*` ou `hotfix/*` é mergeado em `develop`, o desenvolvedor precisa
**lembrar** de abrir um PR manual da mesma branch para `master`. Esse passo
manual é:

1. **Esquecível** — PRs para `master` são omitidos em cenários de pressão
2. **Inconsistente** — Nem todo desenvolvedor segue o mesmo padrão
3. **Sem rastreabilidade** — Não há registro automático do caminho `release/hotfix → master`
4. **Retrabalho** — Se o PR para `master` não for aberto, a release/hotfix não chega em produção

### 1.2 Motivação de Negócio

| Motivação | Detalhamento |
|---|---|
| **Automação de releases** | O merge de uma `release/*` ou `hotfix/*` em `develop` dispara automaticamente a criação do PR correspondente para `master` |
| **Consistência** | Garantir que toda release/hotfix tenha um PR para `master` sem depender de ação manual |
| **Rastreabilidade** | O PR automático para `master` registra: branch de origem, commits mergeados e changelog |
| **Produtividade** | Elimina o passo manual de abrir PR para `master` — o desenvolvedor só precisa revisar e aprovar |
| **Resiliência** | Mesmo que o PR automático não seja mergeado imediatamente, ele existe como lembrete e trilha de auditoria |

### 1.3 Sistemas Envolvidos

| Sistema | Papel |
|---|---|
| **GitHub Actions** | Execução do workflow `auto-pr-master.yml` |
| **`gh` CLI** | Verificação de PRs existentes e criação de novos PRs |
| **GitHub API** | Consulta de PRs mergeados, branches e status |
| **Repositório gopayground** | Código-fonte Go 1.26 com Makefile |

### 1.4 Restrições Conhecidas

1. **Trigger**: O workflow deve disparar em `pull_request` `types: [closed]` em `develop`
2. **Merge check**: Deve verificar `github.event.pull_request.merged == true` — apenas PRs mergeados, não fechados sem merge
3. **Branch filtering**: Apenas branches `release/*` e `hotfix/*` disparam o auto-PR para `master`. Branches `feature/*` NÃO devem disparar
4. **Idempotência**: Se já existir um PR aberto da mesma branch para `master`, não criar duplicata
5. **GITHUB_TOKEN**: O token padrão do GitHub Actions NÃO dispara workflows downstream — PRs criados com `GITHUB_TOKEN` não acionam `ci.yml` ou `pr-review.yml` no PR de destino
6. **Falha parcial**: Se o auto-PR falhar (ex.: conflito de merge), o workflow não deve falhar o pipeline — o PR manual ainda é possível
7. **Compatibilidade reversa**: Não quebrar o fluxo existente de auto-PR para `develop`
8. **Branch protection**: `master` tem branch protection que exige CI verde e revisão — o auto-PR deve respeitar isso (o PR será criado, mas o merge requer aprovação manual)

---

## 2. Requisitos Funcionais e Não-funcionais

### 2.1 Requisitos Funcionais

| ID | Descrição | Prioridade |
|---|---|---|
| **RF-001** | Criar workflow GitHub Actions `auto-pr-master.yml` que dispara em `pull_request: types: [closed] branches: [develop]` | Alta |
| **RF-002** | Verificar `github.event.pull_request.merged == true` antes de prosseguir | Crítica |
| **RF-003** | Extrair branch de origem (`head.ref`) do PR mergeado e verificar se é `release/*` ou `hotfix/*` | Alta |
| **RF-004** | Se a branch de origem for `feature/*`, NÃO criar PR para `master` (apenas logar e sair) | Alta |
| **RF-005** | Extrair versão da branch `release/X.Y.Z` → `X.Y.Z` para uso no título e body do PR | Média |
| **RF-006** | Verificar se já existe PR aberto da branch de origem para `master` antes de criar (idempotência) | Alta |
| **RF-007** | Criar PR para `master` com `gh pr create` se não existir PR aberto | Alta |
| **RF-008** | Título do PR auto-criado deve seguir padrão: `[release] vX.Y.Z` para `release/*` e `[hotfix] descrição` para `hotfix/*` | Média |
| **RF-009** | Body do PR auto-criado deve incluir: branch de origem, commits mergeados (changelog), hash do merge commit, data do merge | Média |
| **RF-010** | Se o PR para `master` já existir, logar o número do PR existente e não criar duplicata | Alta |

### 2.2 Requisitos Não-funcionais

| ID | Descrição | Categoria |
|---|---|---|
| **RNF-001** | Workflow nunca deve criar PR para `master` a partir de branches `feature/*` ou branches sem prefixo conhecido | Segurança |
| **RNF-002** | Nenhum PR duplicado para `master` deve ser criado (idempotência garantida) | Confiabilidade |
| **RNF-003** | Falha na criação do PR para `master` não deve impedir que outros jobs do workflow completem | Resiliência |
| **RNF-004** | Timeout máximo do job: 5 minutos | Desempenho |
| **RNF-005** | Workflow deve usar `GITHUB_TOKEN` com escopo mínimo (`pull-requests: write`) | Segurança |
| **RNF-006** | Logs devem conter informações suficientes para debug (branch, PR number, decisões tomadas) | Observabilidade |
| **RNF-007** | O body do PR deve conter informação de changelog com commits desde o último merge em `develop` ou `master` | Rastreabilidade |

### 2.3 Fora de Escopo

| Item | Justificativa |
|---|---|
| **Auto-merge do PR para `master`** | `master` tem branch protection com revisão obrigatória — auto-merge quebraria essa proteção. Será avaliado em spec futura |
| **Auto-deploy a partir de `master`** | Sem ambiente de produção definido atualmente |
| **Notificações Slack/Discord sobre novos PRs para `master`** | Pode ser adicionado posteriormente |
| **Criação de tags Git no merge para `master`** | Já coberto como recomendação no spec `gitflow-001` (fora de escopo naquela spec) |
| **Validação de convenção de nomenclatura de branch `release/*` e `hotfix/*`** | Será avaliado em spec futura |
| **Bloqueio de merge em `develop` se não houver PR para `master`** | Mudança de comportamento muito drástica — requer discussão com equipe |
| **PR para `master` a partir de `feature/*` mesmo com label/criteria especial** | Fora do modelo GitFlow |

---

## 3. Decisões Arquiteturais

### ADR-001: Trigger em PR mergeado vs. push em develop

| Item | Decisão |
|---|---|
| **Contexto** | Precisamos detectar quando um PR de `release/*` ou `hotfix/*` é mergeado em `develop` para criar o PR correspondente para `master` |
| **Opção A (escolhida)** | Usar trigger `pull_request: types: [closed] branches: [develop]` + verificação `github.event.pull_request.merged == true` |
| **Opção B (rejeitada)** | Usar trigger `push: branches: [develop]` e analisar o diff para detectar merges |
| **Justificativa** | O evento `pull_request.closed` com `merged == true` é semanticamente exato: sabemos exatamente qual PR foi mergeado, de qual branch, com quais commits. O trigger `push` exigiria parsing de mensagens de commit e análise de merge commits, que é mais complexo e propenso a erro |
| **Consequências** | O workflow só executa quando um PR é fechado (mergeado ou não). Precisamos do filtro `merged == true` para ignorar PRs fechados sem merge |

### ADR-002: Filtragem por tipo de branch (release/* e hotfix/* apenas)

| Item | Decisão |
|---|---|
| **Contexto** | Nem toda branch mergeada em `develop` deve gerar PR para `master` |
| **Decisão** | Usar `startsWith` bash para verificar se `head.ref` começa com `release/` ou `hotfix/`. Branches `feature/*` e outras são ignoradas |
| **Justificativa** | O modelo GitFlow é claro: apenas `release/*` e `hotfix/*` devem ir para `master`. `feature/*` são trabalho em andamento e não devem pular para produção |
| **Consequências** | Se o time quiser adicionar outros prefixos no futuro, basta estender a condição. Se um prefixo for digitado errado (ex.: `releas/1.0`), o workflow loga e ignora |

### ADR-003: Uso de `GITHUB_TOKEN` vs. Personal Access Token (PAT)

| Item | Decisão |
|---|---|
| **Contexto** | Para criar PRs via `gh pr create`, precisamos de um token com permissão `pull-requests: write` |
| **Opção A (escolhida)** | Usar `${{ github.token }}` (GITHUB_TOKEN padrão) |
| **Opção B (rejeitada)** | Usar PAT configurado como secret (`secrets.GH_PAT`) |
| **Justificativa** | `GITHUB_TOKEN` já tem o escopo necessário com `pull-requests: write`. É automático, efêmero e não requer gerenciamento de secrets. A limitação conhecida é que PRs criados com `GITHUB_TOKEN` NÃO disparam workflows, mas isso é ACEITÁVEL aqui: o PR para `master` com `GITHUB_TOKEN` não executará CI automaticamente, o que é o comportamento desejado (o CI já rodou no PR original para `develop`, e o PR para `master` herda os mesmos commits) |
| **Consequências** | O PR para `master` ficará sem status checks executados automaticamente. O revisor pode ver os checks do PR original. Se necessário, um `re-trigger` manual dos checks pode ser feito |

### ADR-004: Extração de versão do nome da branch

| Item | Decisão |
|---|---|
| **Contexto** | O título do PR para `master` deve incluir a versão da release ou descrição do hotfix |
| **Decisão** | Para `release/X.Y.Z`, extrair `X.Y.Z` e usar como versão (`vX.Y.Z`). Para `hotfix/descricao`, usar a descrição como está |
| **Justificativa** | `release/*` sempre carrega um número de versão semântica (ex.: `release/1.2.3`). `hotfix/*` geralmente tem uma descrição (ex.: `hotfix/crash-fix-null-pointer`). O tratamento diferenciado produz títulos mais legíveis |
| **Consequências** | Branches `release/` sem versão semântica (ex.: `release/test-only`) geram título `[release] test-only` — que é aceitável para ambientes não-produção |

### ADR-005: Criação de workflow separado vs. job adicional no ci.yml

| Item | Decisão |
|---|---|
| **Contexto** | Onde colocar a lógica de auto-PR para `master`? |
| **Opção A (escolhida)** | Criar workflow separado `auto-pr-master.yml` |
| **Opção B (rejeitada)** | Adicionar job no `ci.yml` existente |
| **Justificativa** | O trigger é completamente diferente (`pull_request.closed` vs `push`). Misturar triggers no mesmo workflow cria complexidade desnecessária. Workflows separados são mais modulares, fáceis de debugar e testar |
| **Consequências** | Mais um arquivo de workflow para gerenciar. Mas cada workflow tem responsabilidade única e clara |

### ADR-006: Tratamento de falha do job

| Item | Decisão |
|---|---|
| **Contexto** | Se a criação do PR para `master` falhar (ex.: conflito, API error, timeout), o workflow deve falhar ou continuar? |
| **Decisão** | Usar `continue-on-error: true` no job de criação do PR |
| **Justificativa** | O PR para `master` é uma conveniência, não um requisito crítico. Se falhar, o desenvolvedor ainda pode criar o PR manualmente. Falhar o workflow criaria alarme falso |
| **Consequências** | Falhas no auto-PR podem passar despercebidas se não houver monitoramento adequado. Mitigação: logging detalhado e notificação opcional |

---

## 4. Especificação Detalhada

### 4.1 Workflow `auto-pr-master.yml` — Especificação Completa

**Arquivo:** `.github/workflows/auto-pr-master.yml`

```yaml
# Auto-PR para Master
# Quando um PR de release/* ou hotfix/* é mergeado em develop,
# cria automaticamente um PR correspondente para master.
name: Auto-PR to Master

on:
  pull_request:
    types: [closed]
    branches: [develop]

permissions:
  contents: read
  pull-requests: write

concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.head.ref }}
  cancel-in-progress: false

jobs:
  create-master-pr:
    if: github.event.pull_request.merged == true
    runs-on: ubuntu-latest
    timeout-minutes: 5
    continue-on-error: true

    steps:
      - name: Checkout repository
        uses: actions/checkout@v6
        with:
          persist-credentials: false
          fetch-depth: 0

      - name: Extract source branch info
        id: info
        env:
          HEAD_REF: ${{ github.event.pull_request.head.ref }}
          BASE_REF: ${{ github.event.pull_request.base.ref }}
          MERGE_COMMIT: ${{ github.event.pull_request.merge_commit_sha }}
          PR_NUMBER: ${{ github.event.pull_request.number }}
        run: |
          echo "head_ref=$HEAD_REF" >> "$GITHUB_OUTPUT"
          echo "base_ref=$BASE_REF" >> "$GITHUB_OUTPUT"
          echo "merge_commit=$MERGE_COMMIT" >> "$GITHUB_OUTPUT"
          echo "pr_number=$PR_NUMBER" >> "$GITHUB_OUTPUT"

      - name: Check if source branch should trigger PR to master
        id: check
        env:
          HEAD_REF: ${{ github.event.pull_request.head.ref }}
        run: |
          if [[ "$HEAD_REF" == release/* ]]; then
            echo "trigger=true" >> "$GITHUB_OUTPUT"
            echo "prefix=release" >> "$GITHUB_OUTPUT"
            VERSION="${HEAD_REF#release/}"
            echo "version=$VERSION" >> "$GITHUB_OUTPUT"
            echo "title=[release] v$VERSION" >> "$GITHUB_OUTPUT"
            echo "Branch de origem: $HEAD_REF (release $VERSION)"
          elif [[ "$HEAD_REF" == hotfix/* ]]; then
            echo "trigger=true" >> "$GITHUB_OUTPUT"
            echo "prefix=hotfix" >> "$GITHUB_OUTPUT"
            DESCRIPTION="${HEAD_REF#hotfix/}"
            echo "version=$DESCRIPTION" >> "$GITHUB_OUTPUT"
            echo "title=[hotfix] $DESCRIPTION" >> "$GITHUB_OUTPUT"
            echo "Branch de origem: $HEAD_REF (hotfix $DESCRIPTION)"
          else
            echo "trigger=false" >> "$GITHUB_OUTPUT"
            echo "prefix=skip" >> "$GITHUB_OUTPUT"
            echo "Branch '$HEAD_REF' nao e release/* nem hotfix/* — nenhum PR para master sera criado."
          fi

      - name: Log skipped branches
        if: steps.check.outputs.trigger == 'false'
        run: |
          echo "##[info] Branch '${{ steps.info.outputs.head_ref }}' ignorada — apenas release/* e hotfix/* geram PR para master."

      - name: Check for existing PR to master
        id: existing
        if: steps.check.outputs.trigger == 'true'
        env:
          GH_TOKEN: ${{ github.token }}
          HEAD_REF: ${{ github.event.pull_request.head.ref }}
        run: |
          echo "Verificando se ja existe PR aberto de '$HEAD_REF' para 'master'..."
          EXISTING=$(gh pr list \
            --head "$HEAD_REF" \
            --base master \
            --state open \
            --json number,title,headRefName,baseRefName \
            --jq 'length' 2>/dev/null || echo "0")

          if [ "$EXISTING" -gt 0 ]; then
            PR_INFO=$(gh pr list \
              --head "$HEAD_REF" \
              --base master \
              --state open \
              --json number,title,url \
              --jq '.[0]' 2>/dev/null)
            PR_NUM=$(echo "$PR_INFO" | jq -r '.number')
            PR_URL=$(echo "$PR_INFO" | jq -r '.url')
            echo "PR ja existe para '$HEAD_REF' -> master: #$PR_NUM ($PR_URL)"
            echo "exists=true" >> "$GITHUB_OUTPUT"
            echo "pr_number=$PR_NUM" >> "$GITHUB_OUTPUT"
          else
            echo "Nenhum PR aberto encontrado para '$HEAD_REF' -> master."
            echo "exists=false" >> "$GITHUB_OUTPUT"
            echo "pr_number=" >> "$GITHUB_OUTPUT"
          fi

      - name: Generate PR body
        id: body
        if: steps.check.outputs.trigger == 'true' && steps.existing.outputs.exists == 'false'
        env:
          HEAD_REF: ${{ github.event.pull_request.head.ref }}
          PR_NUMBER_SRC: ${{ github.event.pull_request.number }}
          PR_TITLE_SRC: ${{ github.event.pull_request.title }}
          MERGE_COMMIT: ${{ github.event.pull_request.merge_commit_sha }}
          MERGED_AT: ${{ github.event.pull_request.merged_at }}
          PREFIX: ${{ steps.check.outputs.prefix }}
          VERSION: ${{ steps.check.outputs.version }}
          GH_TOKEN: ${{ github.token }}
        run: |
          REPO="${{ github.repository }}"

          # Coleta os commits do PR original (usando a API)
          COMMITS_JSON=$(gh pr view "$PR_NUMBER_SRC" \
            --repo "$REPO" \
            --json commits 2>/dev/null || echo '{"commits":[]}')

          CHANGELOG=$(echo "$COMMITS_JSON" | jq -r '.commits[] | "- \(.messageHeadline) (\(.oid[0:7]))"' 2>/dev/null || echo "N/A")

          # FILES CHANGED no PR original
          FILES_JSON=$(gh pr view "$PR_NUMBER_SRC" \
            --repo "$REPO" \
            --json files 2>/dev/null || echo '{"files":[]}')

          FILES_CHANGED=$(echo "$FILES_JSON" | jq -r '.files[] | "- \(.path) (\(.additions) adicoes, \(.deletions) remocoes)"' 2>/dev/null || echo "N/A")

          cat > /tmp/pr_body_master.md << BODYEOF
## 🤖 PR gerado automaticamente para master

Este PR foi criado automaticamente apos o merge do PR #${PR_NUMBER_SRC} em \`develop\`.

### Origem

| Item | Detalhe |
|------|---------|
| **Tipo** | $PREFIX |
| **Branch de origem** | $HEAD_REF |
| **Merge commit** | $MERGE_COMMIT |
| **Merge realizado em** | $MERGED_AT |
| **PR original** | #${PR_NUMBER_SRC} — $PR_TITLE_SRC |

### Changelog (commits do PR original)

${CHANGELOG}

### Arquivos alterados

${FILES_CHANGED}

### Revisão necessária

⚠️ Este PR precisa ser revisado e aprovado antes do merge em \`master\`.
O CI ja foi executado na branch de origem (PR #${PR_NUMBER_SRC}).

---

*PR gerado automaticamente pelo workflow \`Auto-PR to Master\`.*
BODYEOF

          echo "body_file=/tmp/pr_body_master.md" >> "$GITHUB_OUTPUT"

      - name: Create PR to master
        id: create
        if: steps.check.outputs.trigger == 'true' && steps.existing.outputs.exists == 'false'
        env:
          GH_TOKEN: ${{ github.token }}
          HEAD_REF: ${{ github.event.pull_request.head.ref }}
          TITLE: ${{ steps.check.outputs.title }}
        run: |
          echo "Criando PR para master..."
          echo "Titulo: $TITLE"
          echo "Branch: $HEAD_REF -> master"

          PR_URL=$(gh pr create \
            --base master \
            --head "$HEAD_REF" \
            --title "$TITLE" \
            --body-file "${{ steps.body.outputs.body_file }}" \
            2>&1)

          EXIT_CODE=$?
          if [ $EXIT_CODE -ne 0 ]; then
            echo "Falha ao criar PR para master."
            echo "Erro: $PR_URL"
            echo "created=false" >> "$GITHUB_OUTPUT"
            exit 0  # Não falha o job por causa do continue-on-error
          fi

          PR_NUM=$(echo "$PR_URL" | grep -oP '\d+$' || echo "unknown")
          echo "PR criado: $PR_URL"
          echo "created=true" >> "$GITHUB_OUTPUT"
          echo "pr_url=$PR_URL" >> "$GITHUB_OUTPUT"
          echo "pr_number=$PR_NUM" >> "$GITHUB_OUTPUT"

      - name: Log summary
        if: always()
        env:
          TRIGGER: ${{ steps.check.outputs.trigger }}
          HEAD_REF: ${{ github.event.pull_request.head.ref }}
          EXISTING_PR: ${{ steps.existing.outputs.exists }}
          NEW_PR: ${{ steps.create.outputs.created }}
          NEW_PR_URL: ${{ steps.create.outputs.pr_url }}
          EXISTING_PR_NUM: ${{ steps.existing.outputs.pr_number }}
        run: |
          echo "=========================================="
          echo "Resumo do Auto-PR para Master"
          echo "=========================================="
          echo "Branch de origem: $HEAD_REF"
          echo "Disparou PR? $TRIGGER"

          if [ "$TRIGGER" == "true" ]; then
            if [ "$EXISTING_PR" == "true" ]; then
              echo "Resultado: PR ja existia (#$EXISTING_PR_NUM) — nenhuma acao necessaria."
            elif [ "$NEW_PR" == "true" ]; then
              echo "Resultado: PR criado com sucesso: $NEW_PR_URL"
            else
              echo "Resultado: Falha ao criar PR (ver logs acima)."
            fi
          else
            echo "Resultado: Branch ignorada (nao e release/* ou hotfix/*)."
          fi
          echo "=========================================="
```

### 4.2 Variáveis de Contexto do GitHub Actions

O workflow utiliza as seguintes variáveis de contexto:

| Variável | Origem | Descrição |
|---|---|---|
| `github.event.pull_request.merged` | Evento | `true` se o PR foi mergeado, `false` se fechado sem merge |
| `github.event.pull_request.head.ref` | Evento | Nome da branch de origem do PR mergeado |
| `github.event.pull_request.base.ref` | Evento | Nome da branch de destino (sempre `develop`) |
| `github.event.pull_request.number` | Evento | Número do PR mergeado |
| `github.event.pull_request.merge_commit_sha` | Evento | SHA do commit de merge |
| `github.event.pull_request.merged_at` | Evento | Timestamp do merge |
| `github.event.pull_request.title` | Evento | Título do PR mergeado |
| `github.repository` | Contexto | Nome do repositório (`owner/repo`) |
| `github.token` | Contexto | `GITHUB_TOKEN` automático |

### 4.3 Fluxo de Decisão

```
Evento: pull_request [closed] em develop
         │
         ▼
    PR mergeado? ───NÃO──→ [FIM] Log "PR fechado sem merge, ignorando"
         │
        SIM
         │
         ▼
    head.ref começa com?
         │
         ├── release/* ──→ trigger = true, prefix = release
         │                   versão = extrair X.Y.Z
         │
         ├── hotfix/*  ──→ trigger = true, prefix = hotfix
         │                   descrição = extrair após hotfix/
         │
         └── outro ──────→ [FIM] Log "branch feature/* ignorada"
         │
        trigger=true
         │
         ▼
    gh pr list --head $BRANCH --base master --state open
         │
         ├── PR existe? ──SIM──→ [FIM] Log "PR #N já existe"
         │
         └── Não existe
                 │
                 ▼
         Coletar commits do PR original (gh pr view --json commits)
         Gerar body do PR com changelog e arquivos alterados
                 │
                 ▼
         gh pr create --base master --head $BRANCH --title "..." --body-file ...
                 │
                 ├── Sucesso → Log "PR #N criado: URL"
                 └── Falha   → Log "Falha ao criar PR" (continue-on-error)
```

### 4.4 Formato do Título do PR

| Prefixo da Branch | Título do PR | Exemplo |
|---|---|---|
| `release/1.2.0` | `[release] v1.2.0` | `[release] v1.2.0` |
| `release/2.0.0-beta` | `[release] v2.0.0-beta` | `[release] v2.0.0-beta` |
| `hotfix/crash-fix` | `[hotfix] crash-fix` | `[hotfix] crash-fix` |
| `hotfix/security-CVE-123` | `[hotfix] security-CVE-123` | `[hotfix] security-CVE-123` |

### 4.5 Formato do Body do PR

O body do PR gerado automaticamente segue este template:

```markdown
## 🤖 PR gerado automaticamente para master

Este PR foi criado automaticamente apos o merge do PR #123 em `develop`.

### Origem

| Item | Detalhe |
|------|---------|
| **Tipo** | release |
| **Branch de origem** | release/1.2.0 |
| **Merge commit** | abcdef1234567890 |
| **Merge realizado em** | 2026-05-28T22:00:00Z |
| **PR original** | #123 — [release] 1.2.0 (abc1234) |

### Changelog (commits do PR original)

- feat: add payment gateway integration (abc1234)
- fix: correct invoice calculation (def5678)
- refactor: extract payment validation (ghi9012)

### Arquivos alterados

- internal/payment/gateway.go (+120, -30)
- internal/invoice/calculator.go (+15, -8)
- internal/payment/validation.go (+45, -0)

### Revisão necessária

⚠️ Este PR precisa ser revisado e aprovado antes do merge em `master`.
O CI ja foi executado na branch de origem (PR #123).

---

*PR gerado automaticamente pelo workflow `Auto-PR to Master`.*
```

### 4.6 Concorrência

O workflow usa um grupo de concorrência baseado na branch de origem:

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.head.ref }}
  cancel-in-progress: false
```

**Justificativa**: Se dois PRs diferentes forem mergeados em `develop` simultaneamente, cada um processa independentemente (grupos diferentes). Se o mesmo PR for mergeado duas vezes (ex.: reopen + merge), `cancel-in-progress: false` permite que ambos executem — mas a verificação de idempotência (`gh pr list`) previne duplicatas.

### 4.7 Considerações sobre GITHUB_TOKEN e Workflows Downstream

**Problema conhecido**: PRs criados com `GITHUB_TOKEN` não disparam workflows de `pull_request` no repositório de destino. Isso significa que:

1. `ci.yml` NÃO executará automaticamente no PR recém-criado para `master`
2. `pr-review.yml` NÃO executará automaticamente no PR recém-criado para `master`

**Impacto**: O PR para `master` é criado sem status checks automáticos. Isso é **aceitável** porque:

- Os mesmos commits já passaram pelo CI quando a branch foi pushada originalmente
- Os status checks podem ser executados manualmente pelo revisor (repo owner pode acionar)
- A branch protection de `master` exige CI verde + review, então o PR precisa ser revisado de qualquer forma
- Solução futura: usar um PAT (Personal Access Token) se o comportamento de disparar workflows for necessário

**Recomendação**: Documentar para o revisor que, após abrir o PR, ele pode:
1. Verificar os status checks do PR original (#123) que já passaram
2. Se desejar CI explícito no PR de `master`, fechar/reabrir o PR ou fazer um push vazio

---

## 5. Checklist de Verificação

### 5.1 Pré-implantação

- [ ] Revisar YAML do workflow com `actionlint` ou validador YAML
- [ ] Verificar que `permissions:` inclui `pull-requests: write` sem permissões extras
- [ ] Garantir que `GITHUB_TOKEN` tem acesso ao repositório
- [ ] Comunicar à equipe sobre o novo workflow automático
- [ ] Atualizar documentação (`CONTRIBUTING.md`) se necessário

### 5.2 Validação do Workflow

- [ ] Trigger configurado como `pull_request: types: [closed] branches: [develop]`
- [ ] Filtro `github.event.pull_request.merged == true` presente no `if` do job
- [ ] Extração de prefixo `release/*` e `hotfix/*` correta (case-sensitive)
- [ ] Branches `feature/*` são ignoradas (trigger=false)
- [ ] Título do PR gerado corretamente: `[release] vX.Y.Z` e `[hotfix] descricao`
- [ ] Idempotência: `gh pr list` verifica PR existente antes de criar
- [ ] Body do PR inclui changelog com commits e arquivos alterados
- [ ] `continue-on-error: true` configurado no job
- [ ] `timeout-minutes: 5` configurado no job
- [ ] Concorrência configurada por branch de origem (`head.ref`)

### 5.3 Cenários de Teste Obrigatórios

| # | Cenário | Passos | Resultado Esperado |
|---|---|---|---|
| **C1** | Release mergeada em develop | 1. Criar `release/1.2.0` a partir de `develop` 2. Fazer push 3. Criar PR para `develop` 4. Aprovar e mergear | Workflow dispara → verifica `merged=true` → extrai `release/` → cria PR `[release] v1.2.0` para `master` com body contendo changelog |
| **C2** | Hotfix mergeado em develop | 1. Criar `hotfix/crash-fix` a partir de `master` 2. Fazer push 3. Criar PR para `develop` 4. Aprovar e mergear | Workflow dispara → verifica `merged=true` → extrai `hotfix/` → cria PR `[hotfix] crash-fix` para `master` |
| **C3** | Feature mergeada em develop (NÃO disparar) | 1. Criar `feature/nova-tela` a partir de `develop` 2. Fazer push 3. Criar PR para `develop` 4. Aprovar e mergear | Workflow dispara → verifica `merged=true` → extrai `feature/` → **NÃO** cria PR para `master` → log "Branch ignorada" |
| **C4** | PR fechado sem merge | 1. Criar PR de `release/1.0.0` para `develop` 2. Fechar PR sem merge | Workflow dispara → `merged=false` → job **não executa** (guard condition) |
| **C5** | Idempotência — merge duplicado | 1. Mergear `release/1.0.0` em `develop` (PR #10 criado para `master`) 2. Mergear novamente `release/1.0.0` em `develop` | Na segunda execução: workflow detecta PR já existente → log "PR #N já existe" → **não cria duplicata** |
| **C6** | Branch sem prefixo conhecido | 1. Criar branch `experimental/foo` 2. Mergear em `develop` | Workflow dispara → branch não corresponde a `release/*` nem `hotfix/*` → **NÃO** cria PR para `master` → log "ignorada" |
| **C7** | Concorrência — dois releases simultâneos | 1. Mergear `release/1.0.0` em `develop` 2. Simultaneamente mergear `release/2.0.0` em `develop` | Dois workflows executam em paralelo (grupos de concorrência diferentes) → ambos criam PRs para `master` sem colisão |
| **C8** | Falha na criação do PR | 1. Simular falha (ex.: branch protection impede) | Workflow falha no step de criação → `continue-on-error: true` → workflow não falha → log de erro registrado |

### 5.4 Pós-implantação

- [ ] **C1 executado**: release → auto-PR para master criado com sucesso
- [ ] **C2 executado**: hotfix → auto-PR para master criado com sucesso
- [ ] **C3 executado**: feature → NENHUM auto-PR para master
- [ ] **C4 executado**: PR fechado sem merge → workflow não executa
- [ ] **C5 executado**: idempotência funciona (não cria duplicata)
- [ ] **Logs revisados**: mensagens claras e informativas em cada execução
- [ ] **Body do PR revisado**: changelog correto, formatação markdown adequada

### 5.5 Rollback

Em caso de problemas, os passos para reverter são:

1. **Desabilitar workflow**: GitHub UI → Actions → `Auto-PR to Master` → `Disable workflow`
2. **Fechar PRs indesejados**: PRs criados incorretamente para `master`
3. **Remover arquivo**: Deletar `.github/workflows/auto-pr-master.yml`
4. **Comunicar rollback**: Informar a equipe sobre a reversão ao fluxo manual

---

## 6. Riscos e Trade-offs

### 6.1 Tabela de Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|---|---|---|---|
| R-01 | **PR automático para master criado com conflito de merge** | Média | Médio | O PR é criado mesmo com conflitos — o desenvolvedor precisa resolver. Mitigação: adicionar label `has-conflicts` se detectado |
| R-02 | **Changelog incompleto ou incorreto** | Baixa | Baixo | O changelog é gerado a partir dos commits do PR original (fonte autoritativa). Se o PR teve squash merge, será apenas 1 commit |
| R-03 | **GITHUB_TOKEN não dispara workflows no PR criado** | Certa | Médio | Comportamento conhecido e documentado. O CI já rodou nos commits originais. Se necessário, revisor pode fechar/reabrir o PR |
| R-04 | **Race condition: mesmo PR mergeado duas vezes rapidamente** | Muito Baixa | Baixo | `concurrency` group serializa por `head.ref`. Mesmo assim, idempotência via `gh pr list` é a segunda barreira |
| R-05 | **PR mergeado em develop mas branch head já foi deletada** | Média | Médio | `gh pr create` falha se a branch head não existir. O desenvolvedor precisará criar o PR manualmente. `continue-on-error` impede blocking |
| R-06 | **Release branch com versão mal formatada (ex.: `release/foo`)** | Baixa | Baixo | O título será `[release] foo` — aceitável. Não há validação de semver |
| R-07 | **Workflow executando para PRs de forks externos** | Muito Baixa | Baixo | `GITHUB_TOKEN` de forks não tem permissão para criar PRs no repo base. O job falharia silenciosamente. Mitigação: adicionar `if: github.event.pull_request.head.repo.full_name == github.repository` |

### 6.2 Trade-offs

| Decisão | Prós | Contras |
|---|---|---|
| **Workflow separado vs. job no ci.yml** | Responsabilidade única, triggers diferentes, mais modular | Mais um arquivo de workflow para gerenciar |
| **GITHUB_TOKEN vs. PAT** | Simples, automático, sem gerenciamento de secrets | PR criado não dispara workflows downstream |
| **Body com changelog vs. body simples** | Mais informativo, facilita revisão | Mais steps no workflow, dependência de `gh pr view --json commits` |
| **continue-on-error: true** | Falha não bloqueia, PR manual ainda é possível | Falha pode passar despercebida |
| **Fetch-depth: 0 (full history)** | Permite análise de diff entre branches | Checkout mais lento para repositórios grandes |
| **Cancel-in-progress: false** | Permite múltiplas execuções em paralelo | Duplicatas teóricas possíveis (mitigadas por idempotência) |

### 6.3 Planos de Contingência

| Risco | Ação |
|---|---|
| PR para master criado com conflito | Desenvolvedor resolve conflitos localmente e faz push na branch. PR existente é atualizado automaticamente |
| GITHUB_TOKEN sem permissão | Migrar para PAT (`secrets.GH_PAT`) com `pull-requests: write` |
| Branch head deletada antes do auto-PR | Desenvolvedor recria branch a partir do merge commit e abre PR manual |
| Changelog muito grande ou vazio | Adicionar limite de 50 commits no body; se vazio, usar fallback "N/A" |
| Workflow falha consistentemente | Desabilitar workflow, reverter ao fluxo manual, investigar causa |

---

## 7. Hardening

### 7.1 Segurança

| Item | Especificação |
|---|---|
| **Token mínimo** | `pull-requests: write` (criar PRs) + `contents: read` (checkout). Sem `contents: write` ou `issues: write` |
| **Proteção de execução** | `if: github.event.pull_request.merged == true` — apenas PRs mergeados disparam |
| **Filtro de branch** | Condição explícita `startsWith(head_ref, 'release/')` e `startsWith(head_ref, 'hotfix/')` — branches arbitrárias não disparam |
| **GITHUB_TOKEN** | Token efêmero e scoped ao workflow. Máscara automática nos logs |
| **Proteção contra forks** | Opcional: adicionar `if: github.event.pull_request.head.repo.full_name == github.repository` para ignorar PRs de forks |
| **Sem secrets adicionais** | Nenhum PAT ou secret extra é necessário para o funcionamento básico |

### 7.2 Resiliência e Tolerância a Falhas

| Item | Especificação |
|---|---|
| **`continue-on-error: true`** | Falha na criação do PR não propaga para o status do workflow |
| **Timeout** | `timeout-minutes: 5` para o job — evita execuções pendentes |
| **Tratamento de erro no bash** | `gh pr list` com `2>/dev/null \|\| echo "0"` — fallback para 0 em caso de erro |
| **Tratamento de erro no gh pr create** | Captura exit code com `$?` e trata sem falhar o job |
| **Idempotência** | Verificação `gh pr list --base master --state open` antes de criar |
| **Concorrência** | `concurrency` group por `head.ref` — serializa execuções para a mesma branch de origem |
| **Fallback manual** | Se o auto-PR falhar, desenvolvedor ainda pode criar PR manualmente |
| **Rate limiting** | Máximo de ~5 chamadas à API do GitHub por execução (pr view + pr list + pr create) — insignificante para rate limits |

### 7.3 Observabilidade

| Item | Especificação |
|---|---|
| **Logs estruturados** | Todos os steps têm `echo` statements informativos: branch detectada, decisão tomada, PR encontrado/criado |
| **Step summaries** | Step `Log summary` imprime resumo tabular ao final do workflow |
| **Anotações no step** | Uso de `##[info]`, `##[warning]`, `##[error]` para logs visíveis na UI do GitHub Actions |
| **PR body informativo** | O body do PR gerado contém: tipo, branch, merge commit, data, changelog, arquivos alterados |
| **Título do PR rastreável** | `[release] v1.2.0` ou `[hotfix] descricao` — identificável na lista de PRs |
| **GitHub UI** | Workflow executado é visível em Actions com logs completos |

### 7.4 Tratamento de Concorrência

O workflow usa:

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.head.ref }}
  cancel-in-progress: false
```

**Cenários de concorrência:**

| Cenário | Comportamento |
|---|---|
| Dois PRs diferentes mergeados simultaneamente (ex.: `release/1.0.0` e `release/2.0.0`) | Dois workflows executam em paralelo (grupos diferentes) — ambos criam PRs para `master` sem colisão |
| Mesmo PR mergeado duas vezes (ex.: reopen + merge) | `cancel-in-progress: false` permite execução sequencial. A segunda execução detecta PR já existente via `gh pr list` e não cria duplicata |
| Push na branch após PR mergeado (raro) | Não relevante — o trigger é `pull_request.closed`, não `push` |

### 7.5 Verificação de Hardening

- [ ] `pull-requests: write` é a única permissão de escrita
- [ ] `if: github.event.pull_request.merged == true` protege contra PRs fechados sem merge
- [ ] Filtro de branch `release/*` e `hotfix/*` — branches `feature/*` ignoradas
- [ ] `continue-on-error: true` garante resiliência
- [ ] `timeout-minutes: 5` evita jobs pendentes
- [ ] Verificação `gh pr list` garante idempotência
- [ ] `concurrency` group serializa execuções na mesma branch
- [ ] Token `${{ github.token }}` é efêmero e scoped
- [ ] Logs com informações úteis para debug e auditoria
- [ ] Nenhum secret adicional é exposto ou criado
- [ ] Body do PR inclui informações de rastreabilidade completas

---

## Apêndice A: Resumo das Alterações

| Arquivo | Tipo de Alteração | Descrição |
|---|---|---|
| `.github/workflows/auto-pr-master.yml` | **Criar** | Novo workflow para auto-PR de `release/*` e `hotfix/*` para `master` |
| `CONTRIBUTING.md` | **Alterar** (se existir) | Atualizar fluxo para mencionar auto-PR para master |

## Apêndice B: Exemplo de Execução Completa

### Cenário: Release 1.2.0 mergeada em develop

**Evento disparador:** PR #42 (`release/1.2.0` → `develop`) mergeado

**Log do workflow (condensado):**

```
Extract source branch info:
  head_ref: release/1.2.0
  base_ref: develop
  merge_commit: a1b2c3d4e5f6

Check if source branch should trigger:
  Branch release/1.2.0 -> trigger=true, prefix=release, version=1.2.0
  Titulo: [release] v1.2.0

Check for existing PR to master:
  Nenhum PR aberto encontrado para 'release/1.2.0' -> master.

Generate PR body:
  Coletados 12 commits do PR #42
  Body salvo em /tmp/pr_body_master.md

Create PR to master:
  PR criado: https://github.com/Daniel-Dos/gopayground/pull/43

Log summary:
  Branch de origem: release/1.2.0
  Disparou PR? true
  Resultado: PR criado com sucesso: https://github.com/.../pull/43
```

**Resultado:** PR #43 `[release] v1.2.0` → `master` criado automaticamente com body contendo changelog dos 12 commits.

## Apêndice C: Comportamento com GITHUB_TOKEN

```
PR mergeado em develop (com GITHUB_TOKEN)
         │
         ▼
Workflow auto-pr-master.yml executa
         │
         ▼
Cria PR para master usando GITHUB_TOKEN
         │
         ▼
PR #43 criado em master
         │
         ├── CI.yml? NÃO dispara (limitação do GITHUB_TOKEN)
         ├── pr-review.yml? NÃO dispara (limitação do GITHUB_TOKEN)
         └── Branch protection? EXIGE status checks + review
                  │
                  ▼
Revisor precisa aprovar manualmente
         │
         ├── Opção 1: Fechar e reabrir PR para disparar CI
         └── Opção 2: Fazer push vazio (git commit --allow-empty)
```

## Apêndice D: Referências

| Documento | Link |
|---|---|
| SDD GitFlow | [`/specs/workflows/gitflow/spec-sdd-gitflow.md`](../gitflow/spec-sdd-gitflow.md) |
| SDD Auto-PR | [`/specs/workflows/auto-pr/spec-sdd-auto-pr.md`](../auto-pr/spec-sdd-auto-pr.md) |
| GitHub CLI (`gh`) Docs | [https://cli.github.com/manual/](https://cli.github.com/manual/) |
| `gh pr create` | [https://cli.github.com/manual/gh_pr_create](https://cli.github.com/manual/gh_pr_create) |
| `gh pr list` | [https://cli.github.com/manual/gh_pr_list](https://cli.github.com/manual/gh_pr_list) |
| `gh pr view` | [https://cli.github.com/manual/gh_pr_view](https://cli.github.com/manual/gh_pr_view) |
| GitHub Actions `GITHUB_TOKEN` | [https://docs.github.com/en/actions/security-guides/automatic-token-authentication](https://docs.github.com/en/actions/security-guides/automatic-token-authentication) |
| Workflows não disparados por GITHUB_TOKEN | [https://docs.github.com/en/actions/security-guides/automatic-token-authentication#using-the-github_token-in-a-workflow](https://docs.github.com/en/actions/security-guides/automatic-token-authentication#using-the-github_token-in-a-workflow) |
