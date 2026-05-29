# ADR-014: Responsabilidades do Planner e Escopo dos Subagentes

## Status
Accepted

## Date
2026-05-29

## Context
O Planner é o orquestrador principal do pipeline de desenvolvimento, responsável por analisar demandas, criar planos e delegar tarefas aos subagentes. No entanto, observou-se que:

- O Planner estava executando tarefas técnicas diretamente (edição de YAML, código, configurações) em vez de delegar
- O escopo de cada subagente não estava claramente definido
- O senior-engineer estava sendo acionado para alterações em CI/CD, Docker e GitHub Actions, que fogem de sua especialidade
- Isso resultava em retrabalho, inconsistências e desrespeito ao pipeline definido

É necessário formalizar as responsabilidades do Planner e o escopo de cada subagente para garantir a disciplina do pipeline.

## Decision

1. **Planner nunca implementa:** O Planner deve APENAS planejar, orquestrar e delegar. Qualquer trabalho técnico (código, configuração, infraestrutura) DEVE ser delegado ao subagente apropriado via `task` tool.

2. **Escopo dos subagentes é rigoroso:**
   - `architect` → specs SDD, design arquitetural, ADRs
   - `senior-engineer` → código Go (handlers, models, services, repositories, testes). **NÃO** mexe em CI/CD, Docker, git, GitHub Actions, frontend ou dashboards.
   - `hardening-engineer` → resiliência, segurança, concorrência
   - `reviewer` → revisão de código e aderência à spec
   - `documentation-writer` → documentação técnica, ADRs, README
   - `docker-specialist` → Dockerfiles, docker-compose
   - `github-specialist` → CI/CD, GitHub Actions, workflows, branches, PRs
   - `frontend-engineer` → interfaces web (HTML, CSS, JS)
   - `dashboard-report-specialist` → dashboards HTML, relatórios
   - `ai-engineering` → LLM, RAG, embeddings, agentes de IA

3. **Pipeline obrigatório:** Toda demanda não-trivial DEVE seguir o pipeline completo: Architect → Senior Engineer → Hardening → Reviewer → Documentation, mais os especialistas opcionais conforme necessidade.

4. **Validação em cada etapa:** O Planner DEVE validar o resultado de cada etapa antes de prosseguir para a próxima.

## Alternatives Considered

- **Manter o status quo (Planner implementa):** Rejeitado. Leva a inconsistências, retrabalho e violação das responsabilidades definidas.
- **Criar um subagente "universal":** Rejeitado. Fere o princípio de especialização e torna a qualidade dependente de um único agente.
- **Permitir que senior-engineer faça CI/CD:** Rejeitado. CI/CD, Docker e git são responsabilidades de especialistas específicos (github-specialist, docker-specialist).

## Consequences

- **Positivo:** Pipeline mais disciplinado, qualidade consistente, responsabilidades claras, menos retrabalho
- **Positivo:** Cada subagente foca no que faz de melhor
- **Positivo:** O Planner pode orquestrar múltiplos fluxos em paralelo
- **Negativo:** Mais chamadas de task para uma única demanda (compensado pela especialização)
- **Negativo:** Necessidade de treinar a equipe (humana e agente) nos novos papéis
