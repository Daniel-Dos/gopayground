# 01 — Contexto

## Problema

O workflow `opencode.yml` atual neste repositório é uma **versão minimalista** que apenas realiza checkout e executa o agente OpenCode, sem configurar o ambiente de build ou as dependências de infraestrutura necessárias para o agente interagir com o projeto.

O repositório `github.com/Daniel-Dos/rust-ai` possui um workflow OpenCode completo que:

1. Faz checkout do código
2. Configura Docker (BuildKit)
3. Configura a toolchain da linguagem (Rust)
4. Sobe dependências de infraestrutura (NATS)
5. Compila o projeto
6. Executa o agente OpenCode com prompt condicional

Este projeto Go (`gopayground`) precisa de um workflow equivalente que:

- Configure Go em vez de Rust
- Inicie **Kafka** e **Redis** como dependências de infraestrutura (em vez de NATS)
- Execute o build multi-alvo (`make build build-ui build-producer`) em vez de `cargo build`
- Mantenha a mesma lógica de ativação por comandos (`/oc`, `/opencode`, `/run`)

## Motivação de Negócio

1. **Produtividade do desenvolvedor**: o agente OpenCode precisa de um ambiente completo (Kafka + Redis rodando, binários compilados) para executar tarefas de implementação, teste e depuração automaticamente.
2. **Consistência entre projetos**: manter a mesma estrutura de workflow OpenCode entre os repositórios Go e Rust reduz a carga cognitiva da equipe e permite reuso de padrões.
3. **Qualidade das respostas da IA**: com o ambiente configurado, o agente pode executar `make test`, `make build`, verificar lint e realizar validações reais em vez de apenas gerar código teoricamente correto.
4. **Gitflow padronizado**: seguir o mesmo fluxo de branches (feature a partir de develop, PR para master) que os demais workflows do repositório.

## Sistemas Envolvidos

| Sistema | Função | Relevância para o Workflow |
|---------|--------|----------------------------|
| **GitHub Actions** | Orquestrador CI/CD | Executa o workflow opencode.yml |
| **OpenCode (anomalyco)** | Agente de IA | Executa tarefas de desenvolvimento via comentários em PRs/Issues |
| **Apache Kafka (3.9.0)** | Message broker | Infraestrutura necessária para testes do consumer/producer |
| **Redis (7-alpine)** | Cache/estado | Infraestrutura necessária para testes do UI e consumer |
| **Docker (BuildKit)** | Container builder | Setup para build de imagens se necessário |
| **Go toolchain** | Compilador | Necessário para build dos binários (consumer, ui, producer) |

## Restrições Conhecidas

1. **Workflow atual é muito enxuto**: não prepara o ambiente, limitando a capacidade do agente OpenCode.
2. **Projeto Go 1.26.0**: usar `actions/setup-go@v6` com `go-version-file: go.mod` e `cache: true`.
3. **Kafka em KRaft mode**: não usa ZooKeeper; imagem `apache/kafka:3.9.0` com variáveis de ambiente específicas.
4. **Build multi-alvo**: `make build build-ui build-producer` — precisa de Go toolchain.
5. **Mesma estrutura de triggers**: PR para master, comentários em issues, comentários em revisões de PR.
6. **Modelo OpenCode**: `opencode/big-pickle` (mesmo do workflow atual).
7. **Branch**: `feature/opencode-workflow-go` a partir de `develop` (gitflow).
8. **Workflow YAML**: precisa ser semanticamente equivalente ao original do Rust, adaptado para Go.
