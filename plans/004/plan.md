# Plano: Separar OpenCode review em workflow próprio + melhorar PR body

## Demanda
1. O step OpenCode review no `ci.yml` falha porque a action não suporta `push`
2. PR body do auto-pr é muito vago
3. Novo requisito: quando algo mergear em `develop`, auto-PR para `master`

## Pipeline
1. `github-specialist` — modificar `ci.yml` (remover OpenCode review + melhorar PR body) ✅
2. `github-specialist` — criar `opencode-review.yml` (workflow separado para PR review) ✅
3. `architect` — criar spec SDD para auto-PR develop→master ✅
4. `github-specialist` — implementar auto-PR develop→master (após spec aprovada) ✅

## Contexto para GitHub Specialist
- Arquivos a modificar:
  - `.github/workflows/ci.yml` — remover step OpenCode review, melhorar PR body
  - Criar `.github/workflows/opencode-review.yml` — novo workflow para revisão em PR
- Referência: https://opencode.ai/docs/pt-br/github/#exemplo-de-pull-request
- O body do PR deve ser bem formatado, em português, com: tipo, branch, commit,
  lista de arquivos alterados (via `git diff --name-only`), e status do CI
- A action `anomalyco/opencode/github@latest` NÃO suporta evento `push` —
  apenas `pull_request`, `issue_comment`, etc.

## Contexto para Architect
- Criar spec SDD para workflow que, ao mergear para `develop`, abra
  automaticamente um PR para `master`
