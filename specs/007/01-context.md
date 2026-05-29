# 01 — Contexto

## Problema

O workflow `opencode.yml` atual em `.github/workflows/opencode.yml` executa uma série de passos desnecessários para a operação do agente OpenCode, resultando em:

1. **Tempo de execução excessivo** — ~4-5 minutos por execução, dos quais ~3 minutos são desperdiçados
2. **Custo elevado de tokens** — o OpenCode recebe o repositório inteiro como contexto, processando dezenas de milhares de linhas de código irrelevantes
3. **Complexidade desnecessária** — dependências de infraestrutura (Kafka, Redis, Go, Docker) que nada têm a ver com a tarefa do agente de IA

### Steps problemáticos

| Step | Problema | Impacto Estimado |
|---|---|---|
| `docker/setup-buildx-action@v3` | Só faz sentido se for buildar imagens Docker | +30s |
| `actions/setup-go@v6` | OpenCode não compila código Go; é um agente de IA | +20s |
| `Start Kafka (KRaft mode)` | Dependência de runtime, não do agente | +40s |
| `Start Redis` | Dependência de runtime, não do agente | +10s |
| `make build build-ui build-producer` | Compilação desnecessária para revisão/análise de código | +120s |
| OpenCode com repositório completo | Varre **todos** os arquivos como contexto | Lentidão extrema, alto custo de tokens |

**Total de tempo desperdiçado: ~220s (3min40s) por execução.**

## Motivação de Negócio

1. **Velocidade de feedback**: desenvolvedores que acionam o OpenCode via `/oc`, `/run` ou `/opencode` esperam resposta rápida. Uma execução de 5 minutos para um agente que poderia responder em <1 minuto é inaceitável.
2. **Custo de tokens**: o modelo `opencode/big-pickle` cobra por token de entrada. Processar o repositório inteiro multiplica o custo por execução em 10x-50x.
3. **Experiência do desenvolvedor**: um workflow mais rápido e enxuto incentiva o uso do OpenCode como ferramenta de revisão e assistência.
4. **Sustentabilidade**: reduzir a carga no runner GitHub Actions libera recursos para outros workflows (CI, testes).

## Sistemas Envolvidos

| Sistema | Função | Relação com a Mudança |
|---|---|---|
| **GitHub Actions** | Orquestrador CI/CD | Workflow será simplificado e otimizado |
| **OpenCode (anomalyco)** | Agente de IA | Receberá contexto otimizado (apenas lista de arquivos alterados) |
| **tj-actions/changed-files** | Ação para detectar arquivos alterados no PR | Substitui extração manual via GitHub REST API; usa .git local — rápido e sem dependência externa |

## Restrições Conhecidas

1. **Necessidade de fallback**: quando o trigger é `issue_comment` (comentário fora de PR), não há PR para extrair arquivos. Nesse caso, manter o comportamento de contexto completo.
2. **PRs de fork**: a action `tj-actions/changed-files@v46` funciona em PRs de fork pois opera sobre o .git local do repositório base.
3. **Limite de arquivos**: PRs com muitos arquivos alterados podem gerar um prompt com lista longa, mas sem risco de quebra de sintaxe.
4. **Compatibilidade retroativa**: o workflow continuará suportando os mesmos triggers (`pull_request`, `issue_comment`, `pull_request_review_comment`) e os mesmos comandos (`/oc`, `/opencode`, `/run`).
