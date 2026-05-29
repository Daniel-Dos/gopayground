# 04 — Plano de Implementação

## Tarefas

As tarefas devem ser executadas em ordem sequencial. Cada tarefa depende da conclusão da anterior.

### Tarefa 1: Remover steps desnecessários do workflow

**Descrição:** Remover do `opencode.yml` os seguintes steps:
- `docker/setup-buildx-action@v3`
- `actions/setup-go@v6`
- `Start Kafka (KRaft mode)`
- `Start Redis`
- `Build`

**Dependências:** Nenhuma (primeira tarefa)

**Esforço:** Baixo (~5 min)

**Critério de aceitação:** YAML válido sem referências a Docker, Go, Kafka, Redis ou Build.

---

### Tarefa 2: Adicionar concurrency group

**Descrição:** Adicionar o bloco `concurrency` no topo do workflow (após a seção `on:`, antes de `jobs:`):
```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
```

**Dependências:** Tarefa 1

**Esforço:** Baixo (~2 min)

**Critério de aceitação:** Workflow com `concurrency` configurado.

---

### Tarefa 3: Adicionar `timeout-minutes` ao job

**Descrição:** Adicionar `timeout-minutes: 10` no job `opencode` para evitar execuções infinitas.

**Dependências:** Tarefa 2

**Esforço:** Baixo (~1 min)

**Critério de aceitação:** Job com timeout de 10 minutos.

---

### Tarefa 4: Modificar checkout para incluir fetch-depth

**Descrição:** Modificar o step `actions/checkout@v6` para incluir `fetch-depth: 0`, necessário para que o `tj-actions/changed-files` funcione corretamente.

**Dependências:** Tarefa 3

**Esforço:** Baixo (~2 min)

**Critério de aceitação:** Checkout com `fetch-depth: 0`.

---

### Tarefa 5: Adicionar step de extração de arquivos com tj-actions

**Descrição:** Adicionar novo step usando `tj-actions/changed-files@v46` que:
1. Executa apenas quando o evento é `pull_request` (`if: github.event_name == 'pull_request'`)
2. Usa git diff --name-only local (não depende de API externa)
3. Define output `all_changed_files` com a lista de arquivos alterados

**Dependências:** Tarefa 4

**Esforço:** Baixo (~5 min)

**Critério de aceitação:** Step extrai arquivos corretamente para PRs e é pulado para comentários.

---

### Tarefa 6: Adicionar step de preparação do prompt

**Descrição:** Adicionar step `Prepare prompt` que:
1. Se for PR: monta prompt com a lista de arquivos alterados no formato:
   ```
   Revise este Pull Request. Os arquivos alterados sao:
   
   - path/arquivo1.go
   - path/arquivo2.go
   
   Analise cada arquivo alterado listado acima e forneca sugestoes de melhoria.
   ```
2. Se não for PR: usa o corpo do comentário como prompt
3. Usa delimitador `OPROMPT_END` para evitar problemas de escaping

**Dependências:** Tarefa 5

**Esforço:** Médio (~15 min)

**Script de referência:**
```bash
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
```

**Critério de aceitação:** Prompt correto para PRs (com lista de arquivos) e para comentários (corpo do comentário).

---

### Tarefa 7: Modificar step de execução do OpenCode

**Descrição:** Modificar o step `Run opencode` para:
1. Remover `use_github_token: true` (não necessário)
2. Usar o prompt preparado no step anterior (`steps.prompt.outputs.prompt`)
3. Manter `model: opencode/big-pickle`

**Dependências:** Tarefa 6

**Esforço:** Baixo (~5 min)

**Critério de aceitação:** OpenCode executa com prompt condicional correto.

---

### Tarefa 8: Deletar workflow redundante pr-review.yml

**Descrição:** Remover o arquivo `.github/workflows/pr-review.yml` por ser redundante com `opencode.yml` (que já executa em PR).

**Dependências:** Tarefa 7

**Esforço:** Baixo (~1 min)

**Critério de aceitação:** Arquivo `pr-review.yml` não existe mais no repositório.

---

### Tarefa 9: Validar sintaxe YAML do workflow

**Descrição:** Validar a sintaxe YAML do workflow modificado:
```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/opencode.yml'))"
```

**Dependências:** Tarefa 8

**Esforço:** Baixo (~5 min)

**Critério de aceitação:** YAML válido sem erros de sintaxe.

---

### Tarefa 10: Revisão final e testes

**Descrição:** Executar uma revisão completa:
1. Verificar se todos os requisitos funcionais (RF-001 a RF-009) foram atendidos
2. Verificar se todos os requisitos não-funcionais (RNF-001 a RNF-007) foram atendidos
3. Verificar os cenários de validação da `05-validation-checklist.md`
4. Garantir que os comentários no YAML explicam cada step

**Dependências:** Tarefa 9

**Esforço:** Baixo (~10 min)

**Critério de aceitação:** Checklist de validação completamente preenchido.

---

## Resumo de Dependências

```mermaid
flowchart LR
    T1[1: Remover steps] --> T2[2: Concurrency]
    T2 --> T3[3: Timeout]
    T3 --> T4[4: Checkout fetch-depth]
    T4 --> T5[5: Extrair arquivos tj-actions]
    T5 --> T6[6: Preparar prompt]
    T6 --> T7[7: Modificar OpenCode]
    T7 --> T8[8: Deletar pr-review.yml]
    T8 --> T9[9: Validar YAML]
    T9 --> T10[10: Revisão final]
```

## Estimativa de Esforço Total

| Tarefa | Esforço | Responsável |
|--------|---------|-------------|
| 1. Remover steps | Baixo | github-specialist |
| 2. Concurrency | Baixo | github-specialist |
| 3. Timeout | Baixo | github-specialist |
| 4. Checkout | Baixo | github-specialist |
| 5. Extrair arquivos (tj-actions) | Baixo | github-specialist |
| 6. Preparar prompt | Médio | github-specialist |
| 7. Modificar OpenCode | Baixo | github-specialist |
| 8. Deletar pr-review.yml | Baixo | github-specialist |
| 9. Validar YAML | Baixo | github-specialist |
| 10. Revisão final | Baixo | reviewer |

**Total estimado:** ~51 minutos de implementação
