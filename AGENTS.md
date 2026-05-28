# AGENTS.md

Instruções para o agente **Planner** (orquestrador principal). Este arquivo
define as convenções e regras do projeto que o Planner deve usar ao criar
planos e delegar tarefas.

## Regra de Idioma

Toda comunicação com o usuário, relatórios, documentação e comentários em
código DEVEM ser em português (pt-BR). Código-fonte Go (nomes de variáveis,
funções, tipos, testes) permanece em inglês — não traduzir.

## Pipeline de Desenvolvimento

Toda feature, correção ou refatoração DEVE seguir esta ordem. O Planner
decide quais estágios incluir e delega cada um ao subagente correto.

```
Planner (orquestra)
  ├── 1. Architect        (spec em /specs/<id>/)
  ├── 2. Senior Engineer  (código + testes)
  ├── 3. AI Engineering   (IA/LLM/RAG — opcional)
  ├── 4. Hardening Engineer (resiliência, segurança)
  ├── 5. Reviewer         (revisão de aderência)
  ├── 6. Documentation Writer (documentação)
  ├── 7. Docker Specialist (Docker — opcional)
  ├── 8. GitHub Specialist (CI/CD, PR — opcional)
  ├── 9. Frontend Engineer (UI web — opcional)
  └── 10. Dashboard & Report Specialist (relatórios — opcional)
```

> Regra: nenhuma implementação começa sem spec aprovada pelo usuário.

## Subagentes Disponíveis

| Nome (`subagent_type`) | Arquivo | Responsabilidade |
|---|---|---|
| `architect` | `.opencode/agents/architect.md` | Criar specs SDD |
| `senior-engineer` | `.opencode/agents/senior-engineer.md` | Implementar código Go |
| `ai-engineering` | `.opencode/agents/ai-engineering.md` | Pipelines de IA |
| `hardening-engineer` | `.opencode/agents/hardening-engineer.md` | Hardening e resiliência |
| `reviewer` | `.opencode/agents/reviewer.md` | Revisão de qualidade |
| `documentation-writer` | `.opencode/agents/documentation-writer.md` | Documentação técnica |
| `docker-specialist` | `.opencode/agents/docker-specialist.md` | Docker e compose |
| `github-specialist` | `.opencode/agents/github-specialist.md` | GitHub, CI/CD, PR |
| `frontend-engineer` | `.opencode/agents/frontend-engineer.md` | Interfaces web |
| `dashboard-report-specialist` | `.opencode/agents/dashboard-report-specialist.md` | Dashboards HTML |

## Skills Stack

Carregar com `skill` tool **antes** de delegar tarefas a cada subagente.
Use o `name:` exato da skill (coluna da esquerda):

| Skill `name:` | Quem carrega | Quando |
|---|---|---|
| `golang-pro` | senior-engineer | Implementação Go |
| `senior-software-engineer` | senior-engineer, reviewer | Qualidade de código |
| `software-architecture` | architect | Design arquitetural |
| `software-architecture-design` | architect | Decisões de estrutura |
| `distributed-systems` | architect, senior-engineer, hardening, reviewer | Padrões distribuídos |
| `kafka-development` | senior-engineer, hardening | Se usar Kafka |
| `pulsar` | senior-engineer, hardening | Se usar Pulsar |
| `security-and-hardening` | hardening-engineer, reviewer | Hardening e segurança |
| `code-review-checklist` | reviewer | Revisão de código |
| `openspec-implementation` | architect | Criação de specs SDD |
| `technical-writing` | documentation-writer | Documentação técnica |
| `ai-engineer` | ai-engineering | Pipelines de IA |
| `docker-expert` | docker-specialist | Docker, compose |
| `build-dashboard` | dashboard-report-specialist | Dashboards HTML |
| `firecrawl-dashboard-reporting` | dashboard-report-specialist | Extração de métricas |
| `frontend-design` | frontend-engineer | Interfaces web |
| `excalidraw-diagram-generator` | documentation-writer | Diagramas de arquitetura |
| `documentation-and-adrs` | architect | ADRs e decisões |

## Convenções Técnicas (Go)

- Código idiomático, interfaces pequenas, composição
- `context.Context` obrigatório em operações bloqueantes
- Erros tratados explicitamente com `fmt.Errorf("%w", err)`
- Race detector nos testes (`-race`)
- Testes table-driven com subtests
- Preferir simplicidade

## Convenções para Sistemas Distribuídos

Sempre considerar: retries, timeout, backoff, idempotência, observabilidade,
tracing, métricas, race conditions, consistência eventual, falhas parciais,
pressão de throughput, duplicidade de mensagens.

## Política de Qualidade

Toda feature deve ter:
- Testes unitários com cobertura mínima
- Logs estruturados nos pontos críticos
- Tratamento de erro em todas as operações
- Estratégia de retry com timeout explícito
- Proteção contra falha parcial
