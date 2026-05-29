# Plano: Workflows GitHub Actions

## Demanda
Criar workflows profissionais e de fácil manutenção para o GitHub Actions do projeto **gopayground** (https://github.com/Daniel-Dos/gopayground), incluindo integração com OpenCode.

## Pipeline
1. `architect` — criar spec SDD em `/specs/workflows/`
2. `github-specialist` — implementar os workflows em `.github/workflows/`
3. `reviewer` — revisar qualidade e aderência à spec
4. `documentation-writer` — documentar os workflows

## Contexto para cada subagente

### Architect
- **Projeto**: Sistema de pagamentos Go com Kafka, Redis, DynamoDB, OpenTelemetry
- **Estrutura**: cmd/consumer, cmd/producer, cmd/ui
- **Makefile**: targets `test`, `lint`, `build`, `build-ui`, `build-producer`, `docker-up`, `docker-down`
- **Go version**: 1.26.0
- **Módulo**: `github.com/Daniel-Dos/gopayground`
- **OpenCode**: configurado em `opencode.json`, agente padrão `planner`
- **Já existe OpenCode App** instalado no repositório? Sim, foi configurado via `opencode github install`
- **Ambiente**: usa Kafka, Redis, Floci (DynamoDB local), OpenTelemetry Collector
- **Não existe diretório `.github/` ainda**
- **Documentação de referência**: https://opencode.ai/docs/pt-br/github/

### GitHub Specialist
- Implementar os workflows conforme spec criada pelo Architect
- Arquivos devem ficar em `.github/workflows/`
- Seguir boas práticas: secrets, cache, matrix, parallel jobs

### Reviewer
- Revisar se os workflows seguem a spec e as boas práticas

### Documentation Writer
- Documentar os workflows no diretório `docs/`
