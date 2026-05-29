# Plano: Esclarecer Responsabilidades do Planner e Escopo dos Subagentes

## Demanda
O Planner está executando trabalho que deveria delegar (editou `opencode.yml`, `ci.yml` diretamente). O usuário exige:
1. Planner deve APENAS planejar e delegar — NUNCA implementar
2. O escopo de cada subagente deve ficar explícito no `AGENTS.md`
3. `senior-engineer` NÃO mexe em CI/CD, Docker, git, GitHub Actions

## Pipeline
1. `documentation-writer` — atualizar `AGENTS.md` com seção "Responsabilidades do Planner" e "Escopo dos Subagentes"
2. `architect` — criar ADR registrando a decisão de separação de responsabilidades

## Contexto para cada subagente

### documentation-writer
- Arquivo a modificar: `/media/daniel/DanielDias-jogos/desenvolvimento/projetos/Golang/teste/app/AGENTS.md`
- Adicionar seção **"🚫 Responsabilidades do Planner"** deixando explícito:
  - Planner APENAS planeja, orquestra e delega
  - NUNCA implementa código, edita arquivos, escreve testes, configura Docker, etc.
  - Qualquer trabalho técnico DEVE ser delegado ao subagente correto
- Adicionar seção **"📋 Escopo dos Subagentes"** com tabela clara:
  - `architect` → specs SDD, design arquitetural
  - `senior-engineer` → código Go (APENAS lógica de negócio, handlers, models, services, testes unitários)
  - `senior-engineer` → **NÃO** mexe em CI/CD, Docker, git, GitHub Actions, frontend, dashboards
  - `hardening-engineer` → resiliência, segurança, concorrência
  - `reviewer` → revisão de código e aderência à spec
  - `documentation-writer` → documentação técnica, ADRs, README
  - `docker-specialist` → Dockerfiles, docker-compose
  - `github-specialist` → CI/CD, workflows, branches, PRs, GitHub Actions
  - `frontend-engineer` → interfaces web, HTML, CSS, JS
  - `dashboard-report-specialist` → dashboards HTML, relatórios
  - `ai-engineering` → LLM, RAG, embeddings, agentes de IA

### architect
- Criar ADR em `/media/daniel/DanielDias-jogos/desenvolvimento/projetos/Golang/teste/app/docs/decisions/ADR-004-responsabilidades-planner-subagentes.md`
- Status: Accepted
- Context: Planner estava executando tarefas técnicas diretamente
- Decisão: Planner nunca implementa; cada subagente tem escopo bem definido
- Consequências: Pipeline mais disciplinado, qualidade consistente, responsabilidades claras
