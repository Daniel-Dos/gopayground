# 07 — Hardening

Este documento define os requisitos de resiliência, segurança e observabilidade para o workflow `opencode.yml` otimizado.

## Estratégia de Resiliência do Workflow

### Falha na Extração de Arquivos (tj-actions/changed-files)

O step `Get changed files` usa `continue-on-error: true` para garantir que uma falha do `tj-actions/changed-files@v46` não interrompa o job. Em caso de erro (exit code != 0), o step falha silenciosamente, o output `all_changed_files` fica vazio, e o `Prepare prompt` detecta a ausência de arquivos e usa o fallback para contexto completo.

**Comportamento em caso de erro:**
- Workflow NÃO falha (continue-on-error)
- Output `all_changed_files` fica vazio/ausente
- `Prepare prompt` detecta `github.event_name != 'pull_request'` (condicional do step) e usa prompt = comment.body
- OpenCode executa com comportamento original (contexto completo)

### Falha no Shell por Lista de Arquivos Muito Longa

O `tj-actions/changed-files` gerencia a lista de arquivos de forma segura, sem risco de "Argument list too long" no shell. O output `all_changed_files` já vem formatado para iteração no shell.

```bash
# Iteração segura - tj-actions já entrega a lista formatada
for f in $FILES; do
  echo "- $f" >> $GITHUB_OUTPUT
done
```

## Timeout em Cada Operação

| Operação | Timeout Esperado | Configuração |
|----------|-----------------|--------------|
| Checkout do repositório | 60s | Default do actions/checkout |
| Extração de arquivos (tj-actions) | 10s | Gerenciado pela action (0-10s típico) |
| Preparação do prompt | 5s | Operação local rápida (shell script) |
| Execução do OpenCode | 5 min | Gerenciado pela action |
| **Workflow inteiro** | **10 min** | `timeout-minutes: 10` no job |

## Proteção Contra Falha Parcial

O workflow DEVE seguir o princípio de **falha suave** (graceful degradation):

1. **Se a extração de arquivos falhar →** prompt sem lista de arquivos (apenas cabeçalho)
2. **Se o tj-actions não executar (comentário) →** prompt com corpo do comentário
3. **Se o OpenCode falhar →** o workflow falha (não há fallback para o agente em si)

Nenhuma falha na parte de otimização DEVE impedir o OpenCode de executar com o comportamento original.

## Observabilidade

### Logs Estruturados

Cada step DEVE produzir logs informativos:

```
# Step: Get changed files
"Found 5 changed files in PR #42"
"Nenhum output — step foi pulado (evento não é pull_request)"

# Step: Prepare prompt
"Preparing prompt for pull_request event"
"Preparing prompt for issue_comment event"

# Step: Run opencode
"Running opencode with PR context (file list mode)"
"Running opencode with full context (comment mode)"
```

### Métricas Coletadas

O workflow DEVE expor as seguintes informações nos logs para facilitar debugging:

- `changed_files_count`: número de arquivos alterados (implícito pela lista)
- `execution_mode`: 'pr_context' (com lista de arquivos) ou 'full_context' (comportamento original)
- `trigger_type`: 'pull_request', 'issue_comment', ou 'pull_request_review_comment'

## Tratamento de Concorrência

O `concurrency` group já gerencia a concorrência:

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
```

**Comportamento:**
- Múltiplos pushes no mesmo PR: apenas a execução mais recente prossegue
- Execuções em PRs diferentes: paralelas (grupos diferentes)
- Comentários concorrentes no mesmo PR: a última execução cancela a anterior

## Segurança Operacional

### Permissões do Job

```yaml
permissions:
  id-token: write    # Necessário para autenticação OIDC
  contents: read     # Apenas leitura do repositório
  pull-requests: write  # Necessário para criar/atualizar comentários no PR
  issues: write      # Necessário para criar/atualizar comentários em issues
  checks: write      # Necessário para criar/atualizar checks
```

### Proteção de Secrets

- `OPENCODE_API_KEY` DEVE permanecer em `secrets.OPENCODE_API_KEY`
- Nenhum step DEVE fazer echo ou log do valor da API key
- `GITHUB_TOKEN` é gerenciado automaticamente pelo GitHub Actions
- Não é necessário `use_github_token: true` — o OpenCode lê arquivos do repositório local

### Validação de Input

O prompt do OpenCode é construído internamente pelo workflow. Para eventos de PR, o prompt contém apenas texto fixo + lista de arquivos (nomes controlados pelo git). Para comentários, o corpo do comentário é passado como prompt, o que é o comportamento padrão e desejado. A injeção de prompt não é uma preocupação de segurança aqui, pois o usuário já tem permissão para comentar.

## Checklist de Hardening

- [ ] Step `Get changed files` tem `continue-on-error: true` para falha suave
- [ ] Step de extração de arquivos (tj-actions) gerencia resiliência internamente
- [ ] Lista de arquivos vazia não quebra o workflow
- [ ] Prompt é construído via output de step (escapamento seguro com `OPROMPT_END`)
- [ ] `timeout-minutes: 10` no job
- [ ] `concurrency` group configurado
- [ ] Permissões mínimas necessárias no job
- [ ] Nenhum secret é exposto nos logs
- [ ] Workflow tem fallback completo para contexto original
- [ ] `pr-review.yml` deletado (elimina execução redundante do OpenCode)
