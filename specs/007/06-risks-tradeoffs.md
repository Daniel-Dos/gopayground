# 06 — Riscos e Trade-offs

## Riscos Identificados

### Risco 1: Falha na extração de arquivos via tj-actions (R-001)

| Atributo | Valor |
|----------|-------|
| **Descrição** | O `tj-actions/changed-files@v46` pode falhar em cenários extremos: repositório sem histórico suficiente (fetch-depth raso), conflitos de merge não resolvidos, ou limitações de disco no runner. |
| **Probabilidade** | Baixa |
| **Impacto** | Médio — sem a lista de arquivos, o OpenCode recebe contexto completo |
| **Mitigação** | O tj-actions gerencia resiliência internamente (tratamento de erros, validação de inputs). Se falhar, o output `all_changed_files` fica vazio e o OpenCode executa com contexto completo (comportamento original). |

### Risco 2: Lista de arquivos vazia em PR sem alterações (R-002)

| Atributo | Valor |
|----------|-------|
| **Descrição** | Um PR pode ser aberto sem alterações (ex: erro de criação). O tj-actions retornaria lista vazia. |
| **Probabilidade** | Baixa |
| **Impacto** | Baixo — o OpenCode receberia prompt sem arquivos para analisar |
| **Mitigação** | O loop `for f in $FILES` simplesmente não itera, resultando em prompt sem arquivos específicos. O OpenCode ainda pode analisar o repositório como um todo. |

### Risco 3: `${{ github.event_name }}` não corresponde ao esperado (R-003)

| Atributo | Valor |
|----------|-------|
| **Descrição** | Embora raro, eventos like `pull_request_target` ou `workflow_dispatch` poderiam mudar o comportamento. O workflow usa `github.event_name == 'pull_request'` como condição. |
| **Probabilidade** | Muito baixa |
| **Impacto** | Baixo — se o evento não for reconhecido, usa fallback de contexto completo |
| **Mitigação** | O condicional `if:` no step já isola o tj-actions apenas para eventos `pull_request`. Outros eventos usam o comportamento original. |

### Risco 4: PR de fork com restrições de checkout (R-004)

| Atributo | Valor |
|----------|-------|
| **Descrição** | PRs vindos de forks podem ter o checkout limitado. O `tj-actions/changed-files` opera sobre o .git local do repositório base, que está disponível mesmo em forks. |
| **Probabilidade** | Baixa |
| **Impacto** | Baixo — a action funciona com o checkout padrão |
| **Mitigação** | `persist-credentials: false` no checkout evita problemas de token. O tj-actions não precisa de credenciais extras para operar (usa git diff local). |

### Risco 5: Caracteres especiais no prompt quebram o YAML (R-005)

| Atributo | Valor |
|----------|-------|
| **Descrição** | O prompt pode conter caracteres especiais (aspas, backticks, `$`). O nome dos arquivos pode conter espaços ou caracteres incomuns. |
| **Probabilidade** | Baixa |
| **Impacto** | Médio — prompt mal formatado pode afetar a análise do OpenCode |
| **Mitigação** | Uso de delimitador `OPROMPT_END` via `GITHUB_OUTPUT` (escapamento gerenciado pelo GitHub Actions). Arquivos com espaços são raros em projetos Go, mas o shell script itera com `for f in $FILES` que preserva nomes. |

### Risco 6: Remoção de steps quebra dependências existentes (R-006)

| Atributo | Valor |
|----------|-------|
| **Descrição** | Embora os steps de Docker, Go, Kafka, Redis e Build sejam desnecessários para o agente OpenCode, pode haver scripts ou ações que dependam desses serviços para funcionar. |
| **Probabilidade** | Baixa |
| **Impacto** | Baixo — o OpenCode atualmente não utiliza esses serviços; a remoção é segura |
| **Mitigação** | Revisão do código do OpenCode e dos prompts atuais para confirmar que nenhum comando depende de Kafka/Redis rodando. Atualmente o OpenCode apenas analisa código e faz sugestões. |

## Trade-offs

### Trade-off 1: `fetch-depth: 0` (checkout completo) vs `fetch-depth: 1` (checkout raso)

| Opção | Prós | Contras |
|-------|------|---------|
| **`fetch-depth: 0`** | Simples, garante que o tj-actions funcione com git diff | Checkout leva alguns segundos a mais (histórico completo) |
| **`fetch-depth: 1` + `git fetch origin <base>`** | Checkout mais rápido | Mais complexo, precisa de lógica adicional para buscar a branch base |

**Decisão:** Usar `fetch-depth: 0` por simplicidade. O tempo adicional de checkout (~5-10s) é insignificante comparado ao tempo economizado pela remoção dos steps desnecessários (~220s).

### Trade-off 2: `tj-actions/changed-files` vs `actions/github-script` (REST API)

| Opção | Prós | Contras |
|-------|------|---------|
| **`tj-actions/changed-files@v46`** | 0-10s de execução, não depende de API externa, funciona em forks, gerencia resiliência internamente, output pronto para uso | Menos metadados (não fornece status individual de cada arquivo) |
| **`actions/github-script@v7` (REST API)** | Fornece status do arquivo (added/modified/deleted), patches parciais | Depende de API externa, sujeito a rate limit, mais lento, requer try/catch manual |

**Decisão:** Usar `tj-actions/changed-files@v46`. Mais rápido, mais simples e sem dependência externa. O status individual dos arquivos não é necessário para o caso de uso (o OpenCode lê o conteúdo diretamente).

### Trade-off 3: Lista de arquivos no prompt vs diff completo

| Opção | Prós | Contras |
|-------|------|---------|
| **Lista de arquivos** | Muito mais leve em tokens (<1KB vs dezenas de KB), OpenCode lê arquivos do repositório local | OpenCode precisa de acesso ao repositório (já tem com checkout) |
| **Diff completo** | OpenCode vê as alterações sem precisar ler arquivos | Muito mais tokens, pode exceder limite de prompt, mais lento para gerar |

**Decisão:** Usar lista de arquivos. O OpenCode já tem acesso ao repositório checado e pode ler cada arquivo individualmente. A economia de tokens é significativa.

### Trade-off 4: Com `use_github_token: true` vs sem

| Opção | Prós | Contras |
|-------|------|---------|
| **Sem `use_github_token`** | Mais simples, não requer configuração extra de token, o OpenCode usa o repositório local | OpenCode não tem acesso à API do GitHub |
| **Com `use_github_token: true`** | OpenCode pode consultar API do GitHub | Complexidade adicional, não necessário pois o tj-actions já entrega os arquivos |

**Decisão:** Remover `use_github_token: true`. O tj-actions já fornece a lista de arquivos alterados via output. O OpenCode lê os arquivos do repositório local, sem precisar de chamadas de API.

### Trade-off 5: Cancelar execuções anteriores (cancel-in-progress)

| Opção | Prós | Contras |
|-------|------|---------|
| **Com `cancel-in-progress: true`** | Economiza recursos, evita execuções obsoletas | Pode cancelar uma execução que o usuário estava acompanhando |
| **Sem cancelamento** | Previsível, nenhuma execução é interrompida | Várias execuções concorrentes para o mesmo PR desperdiçam recursos |

**Decisão:** Usar `cancel-in-progress: true`. O caso de uso principal (revisão de código) se beneficia de sempre processar a versão mais recente do PR.
