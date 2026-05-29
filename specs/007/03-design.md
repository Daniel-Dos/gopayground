# 03 — Design

## Diagrama de Arquitetura (Workflow Otimizado)

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

    subgraph "Concurrency"
        CG[Concurrency Group:<br/>${{ github.workflow }}-${{ github.ref }}<br/>Cancel-in-progress: true]
    end

    subgraph "Steps do Workflow Otimizado"
        S1[Checkout com fetch-depth: 0<br/>actions/checkout@v6]
        S2{Disparda via PR?}
        S3[Extrair arquivos alterados<br/>tj-actions/changed-files@v46<br/>Usa .git local]
        S4[Preparar prompt com lista de arquivos<br/>Step shell script]
        S5[Executar OpenCode<br/>anomalyco/opencode/github@latest<br/>prompt com lista de arquivos]
        S6[Executar OpenCode padrão<br/>anomalyco/opencode/github@latest<br/>contexto completo]
    end

    PR --> C
    IC --> C
    RC --> C
    C -->|sim| CG
    CG --> S1
    S1 --> S2
    S2 -->|sim| S3 --> S4 --> S5
    S2 -->|não| S6
    C -->|não| END([Fim - job ignorado])
```

## Fluxo de Dados

### Cenário A: Trigger via PR

```
[GitHub abre/sincroniza PR]
       │
       ▼
[Checkout com fetch-depth: 0]
       │
       ▼
[tj-actions/changed-files@v46]
  └─ Usa git diff --name-only local (rápido, 0-10s)
  └─ Output: all_changed_files (lista de arquivos alterados)
  └─ Não depende de API externa
       │
       ▼
[Prepare prompt (shell script)]
  └─ Monta prompt com lista de arquivos:
     "Revise este Pull Request. Os arquivos alterados sao:
      - path/arquivo1.go
      - path/arquivo2.go
      ...
      Analise cada arquivo alterado listado acima e forneca sugestoes de melhoria."
       │
       ▼
[anomalyco/opencode/github@latest]
  ├─ prompt: lista de arquivos (sem diff embutido)
  └─ OpenCode lê os arquivos do repositório local → análise completa
```

### Cenário B: Trigger via Comentário (sem PR)

```
[Usuário comenta /oc em Issue]
       │
       ▼
[Checkout simples]
       │
       ▼
[Prepare prompt (shell script)]
  └─ prompt = corpo do comentário
       │
       ▼
[anomalyco/opencode/github@latest]
  ├─ prompt: corpo do comentário
  └─ OpenCode varre repositório completo (comportamento original)
```

## Estrutura Final do Workflow

```yaml
name: opencode

on:
  pull_request:
    branches: [master]
    types: [opened, synchronize, reopened]
  issue_comment:
    types: [created]
  pull_request_review_comment:
    types: [created]

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  opencode:
    if: |
      github.event_name == 'pull_request' ||
      contains(github.event.comment.body, ' /oc') ||
      startsWith(github.event.comment.body, '/oc') ||
      contains(github.event.comment.body, ' /opencode') ||
      startsWith(github.event.comment.body, '/opencode') ||
      contains(github.event.comment.body, ' /run') ||
      startsWith(github.event.comment.body, '/run')
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      id-token: write
      contents: read
      pull-requests: write
      issues: write
      checks: write
    steps:
      - uses: actions/checkout@v6
        with:
          persist-credentials: false
          fetch-depth: 0

      # Extrai arquivos alterados usando tj-actions e passa a lista no prompt
      # para o OpenCode focar apenas nos arquivos relevantes do PR
      - name: Get changed files
        id: changed-files
        if: github.event_name == 'pull_request'
        uses: tj-actions/changed-files@v46

      - name: Prepare prompt
        id: prompt
        run: |
          if [ "${{ github.event_name }}" = "pull_request" ]; then
            FILES="${{ steps.changed-files.outputs.all_changed_files }}"
            echo "prompt<<OPROMPT_END" >> $GITHUB_OUTPUT
            echo "Revise este Pull Request. Os arquivos alterados sao:" >> $GITHUB_OUTPUT
            echo "" >> $GITHUB_OUTPUT
            for f in $FILES; do
              echo "- $f" >> $GITHUB_OUTPUT
            done
            echo "" >> $GITHUB_OUTPUT
            echo "Analise cada arquivo alterado listado acima e forneca sugestoes de melhoria." >> $GITHUB_OUTPUT
            echo "OPROMPT_END" >> $GITHUB_OUTPUT
          else
            echo "prompt<<OPROMPT_END" >> $GITHUB_OUTPUT
            echo "${{ github.event.comment.body }}" >> $GITHUB_OUTPUT
            echo "OPROMPT_END" >> $GITHUB_OUTPUT
          fi

      - name: Run opencode
        uses: anomalyco/opencode/github@latest
        env:
          OPENCODE_API_KEY: ${{ secrets.OPENCODE_API_KEY }}
        with:
          model: opencode/big-pickle
          prompt: ${{ steps.prompt.outputs.prompt }}
```

## Decisões Arquiteturais

| Decisão | Alternativa | Justificativa |
|---------|-------------|---------------|
| **`tj-actions/changed-files@v46`** para detectar arquivos | `actions/github-script@v7` (REST API) | Action especializada, 0-10s de execução, suporta PR/push/comment, gerencia resiliência internamente, mais simples que github-script e não depende de API externa |
| **Remover step de `git diff`** | Manter diff inline no prompt | O tj-actions já entrega a lista de arquivos; o OpenCode com acesso ao repositório checado pode ler o conteúdo diretamente; diff inline no prompt é desnecessário e aumenta tokens |
| **Remover `use_github_token: true`** | Passar GITHUB_TOKEN manualmente | O tj-actions já fornece a lista de arquivos alterados via output; o OpenCode não precisa chamar a API do GitHub para obter contexto do PR |
| **Lista de arquivos no prompt (em vez de diff)** | Incluir diff completo no prompt | Mais leve em tokens que o diff completo; o OpenCode consegue ler os arquivos do repositório local para análise detalhada |
| **Remover `pr-review.yml`** | Manter workflow redundante | Redundante com `opencode.yml` que já executa em PR; evita execução duplicada do OpenCode |
| **`fetch-depth: 0` no checkout** | `fetch-depth: 1` + fetch adicional | Mais simples; necessário para o tj-actions funcionar corretamente com git diff |
| **Fallback para contexto completo em comentários** | Extrair contexto do issue comment | A maioria dos comandos via comentário são pedidos de esclarecimento ou tarefas específicas; o contexto completo é aceitável para esses casos menos frequentes |
| **Prompt via output de step** | Inline no YAML | Mais seguro e legível; evita problemas de escaping de caracteres especiais no YAML |
