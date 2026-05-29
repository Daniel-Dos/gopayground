# Plano 007 — Otimizar Workflow OpenCode no GitHub Actions

## Demanda

O workflow `opencode.yml` atualmente:
1. Faz checkout completo do repositório
2. Sobe Docker Buildx (desnecessário)
3. Instala Go (desnecessário)
4. Sobe Kafka via Docker (desnecessário)
5. Sobe Redis via Docker (desnecessário)
6. Executa `make build build-ui build-producer` (desnecessário)
7. Só então executa o opencode, que varre a **pasta inteira** do repositório como contexto

Isso torna a execução extremamente lenta, pois o agente opencode recebe todo o código-fonte como contexto.

**Observação do usuário:** quando usado `/oc` em comentário com caminho de arquivo explícito, o opencode é mais rápido e preciso.

## Diagnóstico

| Etapa atual | Problema | Impacto |
|---|---|---|
| `docker/setup-buildx-action` | Só faz sentido se for buildar imagens | +30s desnecessários |
| `actions/setup-go` | OpenCode não compila código Go | +20s desnecessários |
| Kafka + Redis Docker | Dependências de runtime, não de agente IA | +60s desnecessários |
| `make build build-ui build-producer` | Compilação desnecessária para revisão de código | +120s desnecessários |
| OpenCode sem contexto filtrado | Lê **todos** os arquivos do repositório | Lentidão extrema, custo de tokens alto |

## Pipeline Proposto

1. **`architect`** — criar spec em `/specs/007/` detalhando as alterações no workflow
2. **`senior-engineer`** — implementar as mudanças no `opencode.yml`
3. **`reviewer`** — revisar se o workflow está correto
4. **`documentation-writer`** — atualizar documentação se necessário

## Contexto para cada subagente

### Architect

**Instruções:** Criar SDD spec em `/specs/007/` com as seguintes decisões:

#### 1. Remover tudo que é Docker e Build

Remover os seguintes steps do `opencode.yml`:
- `docker/setup-buildx-action` (linha 35-37)
- `Start Kafka (KRaft mode)` (linhas 44-60)
- `Start Redis` (linhas 62-64)
- `Build` (linhas 66-67)

Justificativa: O agente opencode é um **assistente de IA para revisão de código**, não executa a aplicação. Go, Kafka, Redis e build são necessários apenas para o **runtime** da aplicação, não para o agente.

#### 2. Otimizar o checkout

- Adicionar `fetch-depth: 0` no `actions/checkout@v6` (ou fazer fetch da branch base) para poder comparar diffs
- Ou usar `fetch-depth: 1` e depois buscar a branch base com `git fetch origin <base>`

#### 3. Extrair apenas os arquivos alterados no PR

Criar um step que usa a **GitHub API** para obter a lista de arquivos modificados no PR, usando uma das abordagens:

**Abordagem A (recomendada): Usar `github-script` + API**
```yaml
- name: Get changed files
  id: changed-files
  uses: actions/github-script@v7
  with:
    script: |
      const { data } = await github.rest.pulls.listFiles({
        owner: context.repo.owner,
        repo: context.repo.repo,
        pull_number: context.issue.number || context.payload.pull_request.number
      });
      const files = data.map(f => f.filename).join('\n');
      core.setOutput('files', files);
      core.setOutput('count', data.length.toString());
```

**Abordagem B (alternativa): Usar `git diff`**
```yaml
- name: Get changed files via git diff
  id: changed-files-git
  run: |
    git fetch origin ${{ github.base_ref }} --depth=1
    git diff --name-only origin/${{ github.base_ref }}...HEAD > changed-files.txt
    echo "files=$(cat changed-files.txt | tr '\n' ' ')" >> $GITHUB_OUTPUT
```

#### 4. Passar apenas os arquivos alterados como contexto para o opencode

Construir o prompt dinamicamente incluindo apenas os arquivos alterados. Exemplo:

```yaml
- name: Build prompt with changed files
  id: build-prompt
  run: |
    FILES="${{ steps.changed-files.outputs.files }}"
    PROMPT="Revise apenas os arquivos alterados neste PR:\n\n$FILES\n\n${{ github.event.comment.body }}"
    echo "prompt=$PROMPT" >> $GITHUB_OUTPUT
```

Ou melhor: gerar o diff completo do PR e passar como contexto:

```yaml
- name: Generate PR diff
  run: |
    git fetch origin ${{ github.base_ref }} --depth=1
    git diff origin/${{ github.base_ref }}...HEAD > pr.diff
  - name: Run opencode
    uses: anomalyco/opencode/github@latest
    env:
      OPENCODE_API_KEY: ${{ secrets.OPENCODE_API_KEY }}
    with:
      model: opencode/big-pickle
      use_github_token: true
      prompt: |
        Analise este diff de Pull Request:
        $(cat pr.diff)
```

#### 5. Simplificar as condições de trigger

Manter apenas as condições relevantes para o opencode como assistente de PR.

#### 6. Considerar `concurrency` para evitar execuções concorrentes

Adicionar grupo de concorrência (igual ao `pr-review.yml`) para cancelar execuções anteriores do mesmo PR.

### Senior Engineer

**Referência:** Spec em `/specs/007/` (após criada pelo architect)

**Instruções:** Implementar as alterações no arquivo `.github/workflows/opencode.yml`:
1. Remover steps de Docker, Go, Kafka, Redis, Build
2. Adicionar step de extração de arquivos alterados
3. Construir prompt otimizado
4. Adicionar `concurrency` group
5. Ajustar permissões se necessário
6. Testar sintaxe do YAML com `yamlint` ou similar

### Reviewer

**Referência:** Spec em `/specs/007/` + `opencode.yml` modificado

**Instruções:** Revisar:
1. A spec está correta e completa?
2. O YAML é sintaticamente válido?
3. As permissões do GitHub Actions estão corretas?
4. O fluxo de extração de arquivos funciona para todos os triggers (PR, comment)?
5. Não há dependências desnecessárias?
6. O opencode continuará funcionando para comentários com `/oc`?

### Documentation Writer

**Instruções:** Atualizar documentação se necessário (apenas se houver mudanças na interface de uso).

## Riscos e Considerações

1. **Trigger de comentário**: Quando o trigger é `issue_comment`, o `github.base_ref` pode não estar disponível. É necessário tratar esse caso — se não for um PR, usar o contexto completo.
2. **PR vindo de fork**: O token `GITHUB_TOKEN` pode não ter acesso ao PR de fork. A abordagem com `use_github_token: true` pode ser suficiente.
3. **Múltiplos arquivos**: Se o PR tiver muitos arquivos (>100), o prompt pode ficar muito grande. Considerar truncar ou processar em lote.
4. **Fallback**: Se a extração de arquivos falhar, manter fallback para o comportamento atual (contexto completo).

## Estrutura Final Esperada do `opencode.yml`

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
    permissions:
      id-token: write
      contents: read
      pull-requests: write
      issues: write
      checks: write
    steps:
      - name: Checkout repository
        uses: actions/checkout@v6
        with:
          persist-credentials: false
          fetch-depth: 0   # necessário para git diff

      - name: Get changed files
        id: changed-files
        uses: actions/github-script@v7
        with:
          script: |
            const isPR = context.payload.pull_request;
            if (isPR) {
              const { data } = await github.rest.pulls.listFiles({
                owner: context.repo.owner,
                repo: context.repo.repo,
                pull_number: isPR.number
              });
              const files = data.map(f => f.filename);
              core.setOutput('files', files.join('\n'));
              core.setOutput('count', files.length.toString());
              core.setOutput('has_files', 'true');
            } else {
              core.setOutput('has_files', 'false');
            }

      - name: Generate PR diff
        if: steps.changed-files.outputs.has_files == 'true'
        run: |
          git diff origin/${{ github.base_ref }}...HEAD -- ${{ steps.changed-files.outputs.files }} > pr.diff
          echo "diff_size=$(wc -c < pr.diff)" >> $GITHUB_OUTPUT

      - name: Run opencode
        uses: anomalyco/opencode/github@latest
        env:
          OPENCODE_API_KEY: ${{ secrets.OPENCODE_API_KEY }}
        with:
          model: opencode/big-pickle
          use_github_token: true
          prompt: |
            ${{ github.event_name == 'pull_request' && 'Execute o /run' || github.event.comment.body }}
            ${{ github.event_name != 'pull_request' && contains(github.event.comment.body, '/run') && 'Execute o /run' || '' }}

            ${{ steps.changed-files.outputs.has_files == 'true' && '## Arquivos alterados neste PR:' || '' }}
```

> **Nota:** A estrutura acima é **ilustrativa**. A spec final detalhará a implementação exata.
