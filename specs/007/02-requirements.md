# 02 — Requisitos

## Requisitos Funcionais (RF)

### RF-001 — Remover steps desnecessários
O workflow DEVE remover os seguintes steps do `opencode.yml`:
- `Setup Docker` (docker/setup-buildx-action)
- `Setup Go` (actions/setup-go)
- `Start Kafka (KRaft mode)`
- `Start Redis`
- `Build`

### RF-002 — Extrair arquivos alterados no PR
Quando o trigger for `pull_request`, o workflow DEVE extrair a lista de arquivos alterados no PR usando `tj-actions/changed-files@v46`.

### RF-003 — Passar lista de arquivos como contexto para o OpenCode
O prompt enviado ao OpenCode DEVE incluir a lista de arquivos alterados como contexto primário, substituindo a varredura do repositório inteiro. O OpenCode lê o conteúdo dos arquivos diretamente do repositório checado.

### RF-004 — Fallback para triggers sem PR
Quando o trigger NÃO for `pull_request` (ex: `issue_comment` sem PR associado), o workflow DEVE manter o comportamento original (contexto completo do repositório).

### RF-005 — Adicionar concurrency group
O workflow DEVE usar `concurrency` para cancelar execuções anteriores do mesmo PR/grupo:
```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
```

### RF-006 — Manter triggers existentes
O workflow DEVE manter os mesmos eventos de trigger:
- `pull_request` para branch `master` com types `[opened, synchronize, reopened]`
- `issue_comment` com type `[created]`
- `pull_request_review_comment` com type `[created]`

### RF-007 — Manter condição de execução
O job DEVE manter a mesma condição de execução para comandos em comentários:
- ` /oc`, `/oc`, ` /opencode`, `/opencode`, ` /run`, `/run`

### RF-008 — Manter prompt condicional
O prompt DEVE ser diferenciado conforme o trigger:
- Se for PR: prompt com a lista de arquivos alterados
- Se for comentário com comando: corpo do comentário (comportamento original)

### RF-009 — Remover `pr-review.yml`
O workflow `pr-review.yml` DEVE ser deletado por ser redundante com `opencode.yml`.

## Requisitos Não-Funcionais (RNF)

### RNF-001 — Performance
O workflow DEVE executar em **menos de 2 minutos** para PRs de até 50 arquivos alterados (vs ~5 minutos atuais).

### RNF-002 — Eficiência de tokens
O contexto enviado ao OpenCode DEVE ser reduzido em **pelo menos 90%** em volume de texto comparado ao repositório completo.

### RNF-003 — Tratamento de limite de arquivos
PRs com muitos arquivos alterados DEVEM ter a lista completa mesmo assim, sem truncamento. O tj-actions já gerencia a lista de forma segura.

### RNF-004 — Compatibilidade com PRs de fork
O workflow DEVE funcionar corretamente para PRs vindos de forks. O `tj-actions/changed-files` opera no .git local e não depende de permissões de API.

### RNF-005 — Idempotência na extração
O step de extração de arquivos alterados DEVE ser idempotente: executado múltiplas vezes no mesmo PR, deve produzir o mesmo resultado.

### RNF-006 — Tolerância a falha na extração
Se o step de extração de arquivos falhar, o workflow DEVE fazer fallback para o comportamento de contexto completo, sem interromper a execução.

### RNF-007 — Manutenibilidade
O workflow DEVE conter comentários YAML explicando cada step e a lógica de fallback, facilitando manutenção futura.

## Fora de Escopo

- **Não** modificar o workflow `ci.yml`
- **Não** modificar o `opencode.json` ou `AGENTS.md`
- **Não** alterar o modelo OpenCode (`opencode/big-pickle`)
- **Não** adicionar novos secrets ou variáveis de ambiente
- **Não** implementar processamento em lote para PRs muito grandes
- **Não** modificar o `Makefile` ou qualquer código-fonte Go do projeto
- **Não** alterar a branch de destino (master) ou os types de trigger
- **Não** gerar `git diff` inline no prompt (lista de arquivos é suficiente)
- **Não** usar `use_github_token: true` (tj-actions já entrega a lista; OpenCode lê do repositório local)
