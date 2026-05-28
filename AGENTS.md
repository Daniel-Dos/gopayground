# AGENTS.md

Instruções para o agente principal. Versão em português (pt-BR).

## Regra de Idioma

Toda comunicação com o usuário, relatórios, documentação e comentários em
código DEVEM ser em português (pt-BR). Código-fonte Go (nomes de variáveis,
funções, tipos, testes) permanece em inglês — não traduzir.

## Fluxo Obrigatório de Desenvolvimento

Toda feature, correção ou refatoração DEVE seguir esta ordem de papéis.
Cada etapa é executada pelo subagent correspondente via `task` tool com
`subagent_type`.

```
Architect
    ↓  (cria specs em /specs/<id-feature>/)
Senior Engineer
    ↓  (implementa código e testes seguindo a spec)
AI Engineering
    ↓  (implementa pipelines de IA se aplicável)
Hardening Engineer
    ↓  (valida resiliência, concorrência, segurança)
Reviewer
    ↓  (revisa aderência à spec e qualidade)
Technical Writer
    ↓  (documenta o que foi construído)
Docker Specialist
    ↓  (cria/mantém Dockerfiles e docker-compose)
GitHub Specialist
    ↓  (configura repositório, CI/CD, PRs)
Frontend Engineer
    ↓  (interfaces web, dashboards)
Dashboard & Report Specialist
    ↓  (relatórios HTML, extração de métricas)
```

> Regra: nenhuma implementação começa sem spec aprovada.

## Skills stack

Skills são carregadas com o `skill` tool **antes** de iniciar qualquer
trabalho relevante. Usar os `name:` abaixo (correspondem ao campo `name:`
no frontmatter de cada `SKILL.md`):

| Skill `name:` | Quando carregar |
|---|---|
| `golang-pro` | Implementação/refatoração Go |
| `senior-software-engineer` | Revisão de código, debugging |
| `software-architecture` | Design arquitetural, análise de código |
| `software-architecture-design` | Decisões de estrutura de sistema |
| `distributed-systems` | Padrões de sistemas distribuídos |
| `kafka-development` | Configuração/uso de Kafka |
| `pulsar` | Configuração/uso de Pulsar |
| `security-and-hardening` | Hardening, validação de segurança |
| `code-review-checklist` | Revisão de código |
| `openspec-implementation` | Criação de specs SDD |
| `technical-writing` | Documentação técnica |
| `ai-engineer` | Pipelines de IA, RAG, agents |
| `docker-expert` | Docker, Docker Compose |
| `build-dashboard` | Dashboards HTML interativos |
| `firecrawl-dashboard-reporting` | Extração de métricas via Firecrawl |
| `frontend-design` | Interfaces web, HTML/CSS/JS |
| `excalidraw-diagram-generator` | Diagramas de arquitetura |
| `documentation-and-adrs` | ADRs, registros de decisão |

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

## Roteamento Automático de Subagentes

O usuário NUNCA deve precisar usar `@<subagent>` ou especificar manualmente
qual subagente chamar. O agente principal DEVE analisar automaticamente cada
mensagem do usuário e delegar ao subagente correto com base no domínio da
tarefa:

| Se a mensagem menciona... | Roteie para... |
|---|---|
| Docker, Dockerfile, compose, container, imagem, build de imagem | `docker-specialist` |
| Erro de compilação, código Go, teste, lint | `senior-engineer` |
| Spec, arquitetura, design, ADR | `architect` |
| Segurança, hardening, vulnerabilidade | `hardening-engineer` |
| Revisão, code review, qualidade | `reviewer` |
| Documentação, README | `documentation-writer` |
| GitHub, Actions, CI/CD, PR, release | `github-specialist` |
| Frontend, UI, HTML, CSS, dashboard | `frontend-engineer` |
| Relatório, métrica, Firecrawl | `dashboard-report-specialist` |
| IA, LLM, RAG, embedding, pipeline de IA | `ai-engineering` |

Regras:
- Antes de qualquer ação direta (editar código, rodar comandos), PARE e
  avalie se a tarefa pertence a um subagente. Se sim, delegue via `task`.
- Se houver dúvida entre múltiplos subagentes, delegue ao mais específico.
- Se a tarefa claramente não se encaixa em nenhum subagente, execute
  diretamente.

## Arquivos de Agentes

Os prompts detalhados de cada papel estão em `.opencode/agents/<nome>.md`.
Sempre passar o contexto relevante ao subagent ao delegar a tarefa.
