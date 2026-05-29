# 05 — Checklist de Validação

## Checklist de Aceitação (QA)

### Estrutura do Workflow

- [ ] O workflow `opencode.yml` NÃO contém mais os steps:
  - `docker/setup-buildx-action`
  - `actions/setup-go`
  - `Start Kafka`
  - `Start Redis`
  - `make build build-ui build-producer`
- [ ] O workflow possui bloco `concurrency` configurado
- [ ] O job possui `timeout-minutes: 10`
- [ ] O checkout possui `fetch-depth: 0`
- [ ] O workflow NÃO usa `use_github_token: true` no step do OpenCode
- [ ] O workflow `pr-review.yml` foi deletado

### Extração de Arquivos

- [ ] O step `Get changed files` usa `tj-actions/changed-files@v46`
- [ ] O step executa apenas quando `github.event_name == 'pull_request'` (condicional `if:`)
- [ ] Para PRs, o step extrai corretamente a lista de arquivos alterados no output `all_changed_files`
- [ ] Para comentários (não-PR), o step é pulado (condicional false)
- [ ] O step funciona em PRs de fork (usa .git local, não API externa)

### Prompt e Execução do OpenCode

- [ ] Para PRs: o prompt inclui a lista de arquivos alterados no formato:
  ```
  Revise este Pull Request. Os arquivos alterados sao:
  
  - path/arquivo1.go
  - path/arquivo2.go
  
  Analise cada arquivo alterado listado acima e forneca sugestoes de melhoria.
  ```
- [ ] Para comentários (não-PR): o prompt mantém o comportamento original (corpo do comentário)
- [ ] O prompt NÃO inclui diff inline (apenas lista de arquivos)
- [ ] O prompt NÃO inclui "Execute o /run" (removido na implementação final)
- [ ] O delimitador `OPROMPT_END` é usado para delimitar o prompt no output do step

### YAML e Sintaxe

- [ ] `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/opencode.yml'))"` não reporta erros
- [ ] Não há referências a variáveis/contextos que não existem
- [ ] As expressões `${{ }}` estão todas corretamente fechadas
- [ ] Os `if:` condicionais usam sintaxe válida

## Cenários de Teste Obrigatórios

### Cenário 1: PR normal com poucos arquivos (1-10 arquivos)

**Entrada:** PR para `master` com 3 arquivos alterados (2 modificados, 1 adicionado)

**Verificações:**
- [ ] Step `Get changed files` roda (evento é pull_request)
- [ ] Output `all_changed_files` contém os 3 arquivos
- [ ] Prompt enviado ao OpenCode contém a lista dos 3 arquivos
- [ ] Workflow completo executa em < 2 minutos

### Cenário 2: PR com muitos arquivos (>100)

**Entrada:** PR com 150 arquivos alterados

**Verificações:**
- [ ] Step `Get changed files` extrai todos os 150 arquivos
- [ ] Prompt contém lista completa dos 150 arquivos
- [ ] Workflow não quebra por argumento muito longo (tj-actions gerencia internamente)

### Cenário 3: Comentário em Issue (sem PR)

**Entrada:** Comentário `/oc execute task X` em uma Issue

**Verificações:**
- [ ] Step `Get changed files` é pulado (evento não é pull_request)
- [ ] Step `Prepare prompt` usa o corpo do comentário
- [ ] OpenCode executa com prompt = "execute task X"
- [ ] Workflow usa contexto completo (comportamento original)

### Cenário 4: Comentário em PR Review

**Entrada:** Comentário `/run` na revisão de um PR

**Verificações:**
- [ ] Se `pull_request_review_comment` carregar PR no payload, deve-se verificar se o step de changed-files roda ou não
- [ ] Atualmente o step só roda em `pull_request` event, então em `pull_request_review_comment` ele é pulado
- [ ] Prompt usa o corpo do comentário (contexto completo)

### Cenário 5: PR vindo de fork

**Entrada:** PR de um fork externo

**Verificações:**
- [ ] `tj-actions/changed-files` funciona em PRs de fork (usa .git local)
- [ ] Step `Get changed files` não falha por falta de permissão
- [ ] Lista de arquivos é extraída corretamente

### Cenário 6: Concurrency em ação

**Entrada:** Dois pushes consecutivos no mesmo PR em rápida sucessão

**Verificações:**
- [ ] Segunda execução cancela a primeira
- [ ] Apenas a última execução conclui

## Critérios de Hardening

- [ ] Step de extração de arquivos não falha o workflow se o tj-actions encontrar erro (tj-actions gerencia resiliência internamente)
- [ ] Lista de arquivos vazia não quebra o loop `for f in $FILES`
- [ ] Prompt não contém caracteres que possam quebrar o YAML
- [ ] Workflow não expõe secrets nos logs (nenhum echo de variável sensível)
- [ ] Timeout de 10 minutos evita execuções infinitas
