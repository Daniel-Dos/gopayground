# 06 — Riscos e Trade-offs

## Riscos Identificados

### R1 — Container Kafka não fica pronto a tempo
| Atributo | Valor |
|----------|-------|
| **Descrição** | Kafka em modo KRaft pode levar de 20 a 40s para iniciar completamente no GitHub Actions runner, mesmo com o status `running`. O health check atual verifica apenas o estado do container, não a prontidão do broker. |
| **Probabilidade** | Média |
| **Impacto** | Alto (build falha porque consumer/producer não conseguem conectar) |
| **Mitigação** | Adicionar verificação real de prontidão do Kafka: tentar abrir socket na porta 9092 ou usar `kafka-topics.sh --bootstrap-server localhost:9092 --list` dentro do container. |

### R2 — CLUSTER_ID duplicado em re-runs
| Atributo | Valor |
|----------|-------|
| **Descrição** | O `CLUSTER_ID` do Kafka precisa ser único por cluster. Se o container for removido e recriado com o mesmo `CLUSTER_ID`, o Kafka pode rejeitar por detectar conflito de metadados. |
| **Probabilidade** | Baixa (Kafka KRaft com `--rm` limpa os dados) |
| **Impacto** | Médio (Kafka falha ao iniciar) |
| **Mitigação** | Usar `CLUSTER_ID=opencode-cluster-$(date +%s)` para garantir unicidade. Já implementado na spec. |

### R3 — Secreta OPENCODE_API_KEY não configurada
| Atributo | Valor |
|----------|-------|
| **Descrição** | O segredo `OPENCODE_API_KEY` pode não estar configurado no repositório, fazendo o step "Run opencode" falhar silenciosamente ou com erro. |
| **Probabilidade** | Média (repositório Go pode não ter o segredo configurado) |
| **Impacto** | Alto (agente não executa) |
| **Mitigação** | Configurar o segredo no GitHub Settings > Secrets and variables > Actions. Adicionar verificação no workflow: `if: ${{ secrets.OPENCODE_API_KEY != '' }}` (não obrigatório pois isso exporia indiretamente a existência do segredo). |

### R4 — Makefile ausente ou broken no momento da execução
| Atributo | Valor |
|----------|-------|
| **Descrição** | O workflow executa `make build build-ui build-producer`. Se o Makefile for alterado ou quebrado em um PR, o build falha. |
| **Probabilidade** | Baixa |
| **Impacto** | Médio (build falha, agente não tem binários para testar) |
| **Mitigação** | O step de build é executado antes do OpenCode justamente para validar que o código compila. Falha é esperada e informativa. |

### R5 — Runner sem Docker
| Atributo | Valor |
|----------|-------|
| **Descrição** | Ubuntu-latest padrão tem Docker, mas actions personalizadas podem não ter. |
| **Probabilidade** | Muito baixa |
| **Impacto** | Alto (todos os steps de infraestrutura falham) |
| **Mitigação** | N/A (ubuntu-latest sempre tem Docker, e `docker/setup-buildx-action@v3` falharia se não houver). |

### R6 — Consumo excessivo de recursos (Kafka + Redis + Build)
| Atributo | Valor |
|----------|-------|
| **Descrição** | Kafka consome ~512MB de RAM, Redis ~50MB, build Go ~200MB. Total ~762MB — dentro do limite de ubuntu-latest (7GB+). |
| **Probabilidade** | Muito baixa |
| **Impacto** | Baixo |
| **Mitigação** | N/A — recursos suficientes. |

## Trade-offs

| Decisão | Prós | Contras |
|---------|------|---------|
| **Iniciar Kafka e Redis como containers separados (vs. docker-compose)** | + Simplicidade (2 comandos docker run)<br>+ Independência do docker-compose.yml (que pode ser alterado)<br>+ Alinhamento com o original (1 container = 1 comando) | - Perde declaração de dependências do compose<br>- Perde rede compartilhada<br>- Mais difícil de adicionar novos serviços |
| **Health check via docker inspect (vs. nc/curl no serviço)** | + Simples e rápido<br>+ Não depende de ferramentas adicionais | - Não verifica prontidão real do Kafka (apenas se o processo está rodando)<br>- Pode prosseguir antes do broker estar pronto |
| **Aguardar serviços vs. executar OpenCode imediatamente** | + Ambiente completo disponível para o agente<br>+ Menos falhas intermitentes | + Aumenta tempo total do workflow (~30s-60s adicional) |
| **Manter modelo opencode/big-pickle (vs. trocar)** | + Consistência com workflow atual do projeto<br>+ Modelo testado e estável | - Modelo pode ser superdimensionado para tarefas simples<br>- Custo maior por execução (se aplicável) |
| **Remover containers antes de iniciar (vs. assumir que não existem)** | + Idempotência em re-runs<br>+ Evita conflito de portas/nomes | + Tempo extra para remoção (irrelevante) |
| **Manter permissões do original (escrita em PRs e issues)** | + OpenCode pode criar/atualizar PRs e comentários | + Superfície de segurança ligeiramente maior |
| **timeout-minutes: 30 (vs. sem timeout)** | + Evita execução infinita em caso de hang | + Workflows complexos podem precisar de mais tempo (improvável) |
