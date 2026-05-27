# 06 — Riscos e Tradeoffs

## Riscos

### R1 — Falha do Kafka Broker

| Atributo       | Detalhe                                                                    |
|----------------|----------------------------------------------------------------------------|
| **Descrição**  | Um ou mais brokers Kafka ficam indisponíveis.                               |
| **Impacto**    | Consumer fica bloqueado até reconexão. Mensagens não são perdidas (retenção). |
| **Probabilidade** | Média (depende da infraestrutura).                                        |
| **Mitigação**  | Múltiplos brokers (3+). Consumer configurado com `net.MaxOpenRequests` e     |
|                | `Consumer.Retry.Backoff`. Consumer group permite rebalanceamento ao         |
|                | reconectar. Monitoramento de lag via métricas.                              |

### R2 — Inconsistência Redis vs DynamoDB

| Atributo       | Detalhe                                                                    |
|----------------|----------------------------------------------------------------------------|
| **Descrição**  | Status no Redis pode divergir do histórico no DynamoDB em caso de falha     |
|                | parcial (Redis ok, DynamoDB falha e vice-versa).                            |
| **Impacto**    | Status no Redis mostra um estado, mas DynamoDB não tem o registro.          |
| **Probabilidade** | Baixa (falhas parciais são raras).                                        |
| **Mitigação**  | O Redis é considerado o "source of truth" para status atual. DynamoDB é o   |
|                | armazenamento permanente completo. Inconsistências são toleradas e          |
|                | reconciliadas por processo externo (fora do escopo).                         |
| **Residual**   | Aceito. Consistência eventual entre Redis e DynamoDB.                       |

### R3 — Mensagens Duplicadas no Kafka

| Atributo       | Detalhe                                                                    |
|----------------|----------------------------------------------------------------------------|
| **Descrição**  | Kafka pode entregar a mesma mensagem mais de uma vez (at-least-once).       |
| **Impacto**    | Processamento duplicado sem idempotência causaria registros duplicados.     |
| **Probabilidade** | Alta (comportamento normal do Kafka).                                     |
| **Mitigação**  | Idempotência via Redis com `SET NX` + TTL de 24h. DynamoDB com              |
|                | `ConditionExpression` para evitar duplicatas na tabela de histórico.        |

### R4 — Pressão de Throughput

| Atributo       | Detalhe                                                                    |
|----------------|----------------------------------------------------------------------------|
| **Descrição**  | Picos de tráfego acima de 1.000 msg/s podem sobrecarregar Redis ou         |
|                | DynamoDB, causando timeouts e degradação.                                   |
| **Impacto**    | Aumento de latência p99, aumento de retries, possível backlog no Kafka.    |
| **Probabilidade** | Média (depende do comportamento da fonte).                                |
| **Mitigação**  | Worker pool configurável. Circuit breaker para DynamoDB. Redis cluster.     |
|                | Auto-scaling horizontal (Kafka consumer group).                             |
| **Residual**   | Necessário monitoramento contínuo e alertas de lag.                         |

### R5 — Falha Parcial (Redis ok, DynamoDB falha)

| Atributo       | Detalhe                                                                    |
|----------------|----------------------------------------------------------------------------|
| **Descrição**  | DynamoDB retorna erro/timeout, Redis está operacional.                      |
| **Impacto**    | Status no Redis é atualizado, histórico não é registrado. Mensagem vai      |
|                | para retry. Se retry exaurir, vai para DLQ.                                 |
| **Probabilidade** | Média.                                                                     |
| **Mitigação**  | Retry com backoff para DynamoDB. Falha parcial não impede atualização do    |
|                | status no Redis. DLQ preserva mensagem para reprocessamento manual.         |

### R6 — Falha Parcial (DynamoDB ok, Redis falha)

| Atributo       | Detalhe                                                                    |
|----------------|----------------------------------------------------------------------------|
| **Descrição**  | Redis retorna erro/timeout, DynamoDB está operacional.                      |
| **Impacto**    | Idempotência não é verificada (fallback otimista → processa). Status não    |
|                | é atualizado. Histórico no DynamoDB é registrado.                           |
| **Probabilidade** | Média.                                                                     |
| **Mitigação**  | `IsProcessed` com fallback otimista (assume não processado). `UpdateStatus` |
|                | não bloqueia processamento. Mensagem pode ser reprocessada se Redis voltar.  |

### R7 — Retry Storm

| Atributo       | Detalhe                                                                    |
|----------------|----------------------------------------------------------------------------|
| **Descrição**  | Múltiplas mensagens falhando simultaneamente geram uma avalanche de        |
|                | retries, sobrecarregando ainda mais os sistemas downstream.                 |
| **Impacto**    | Degradação adicional, aumento de latência, timeouts em cascata.             |
| **Probabilidade** | Baixa (depende de falhas downstream).                                     |
| **Mitigação**  | Jitter no backoff (±25%) distribui as tentativas. Circuit breaker no        |
|                | DynamoDB corta chamadas rápidas quando degradado.                           |

### R8 — Perda de Mensagem na DLQ

| Atributo       | Detalhe                                                                    |
|----------------|----------------------------------------------------------------------------|
| **Descrição**  | Se o DLQ Producer falha (Kafka broker indisponível), a mensagem é perdida. |
| **Impacto**    | Mensagem com falha permanente não é preservada para análise.               |
| **Probabilidade** | Baixa (DLQ é redundante, mas não 100% garantido).                         |
| **Mitigação**  | Log crítico emitido. Em produção, DLQ deve ser cluster separado ou          |
|                | altamente disponível. Possível extensão futura: fallback para arquivo local.|

---

## Tradeoffs

### T1 — Consistência Eventual vs Consistência Forte

| Opção                    | Prós                                          | Contras                                         |
|--------------------------|-----------------------------------------------|-------------------------------------------------|
| **Consistência Eventual** | Baixa latência, alta disponibilidade,          | Risco de leitura de status defasado,             |
| (escolhida)              | tolerância a falhas parciais.                  | reconciliação necessária.                        |
| Consistência Forte       | Estado sempre correto.                         | Maior latência, menor disponibilidade,           |
|                          |                                               | dependência de transação distribuída.            |

**Decisão**: Consistência eventual entre Redis (status) e DynamoDB (histórico).
O status no Redis é o estado atual "mais recente conhecido". Se o DynamoDB
falha, o Redis ainda reflete a última atualização. A reconciliação pode ser
feita por um job externo que varre o DynamoDB e atualiza o Redis.

### T2 — TTL de Idempotência vs Armazenamento Permanente

| Opção                           | Prós                                         | Contras                                        |
|---------------------------------|----------------------------------------------|------------------------------------------------|
| **TTL de 24h no Redis**         | Simples, automático, sem custo de storage.   | Mensagens com mais de 24h podem ser             |
| (escolhida)                     |                                              | reprocessadas (raro).                           |
| TTL maior (7 dias)              | Cobertura maior.                             | Mais memória Redis.                             |
| Armazenamento permanente        | Cobertura total.                             | Complexidade, custo, lookup mais lento.         |
| (DynamoDB + cache Redis)        |                                              |                                                 |

**Decisão**: TTL de 24h no Redis. É o suficiente para o caso de uso típico
(consumo contínuo). Se uma mensagem com mais de 24h chegar, será reprocessada,
mas o DynamoDB aceitará duplicatas (ConditionalCheckFailed → ignorado).

### T3 — Retry Síncrono vs Assíncrono

| Opção                    | Prós                                          | Contras                                         |
|--------------------------|-----------------------------------------------|-------------------------------------------------|
| **Retry Síncrono**       | Simples, mensagem não é commited até sucesso,  | Bloqueia o worker durante backoff.               |
| (escolhida)              | ordenação preservada.                         |                                                 |
| Retry Assíncrono         | Não bloqueia worker, maior throughput.         | Complexidade (fila interna), difícil garantir   |
| (republicar no Kafka)    |                                               | ordenação, risco de loop infinito.              |

**Decisão**: Retry síncrono com blocking. Para 3 tentativas rápidas (100ms,
300ms, 900ms), o bloqueio máximo por mensagem é ~1.6s. Com 10 workers, o
throughput não é significativamente impactado. Retry assíncrono seria mais
complexo e introduziria riscos de loop.

### T4 — Worker Pool Fixo vs Elástico

| Opção                    | Prós                                          | Contras                                         |
|--------------------------|-----------------------------------------------|-------------------------------------------------|
| **Worker Pool Fixo**     | Previsível, controle de recursos.             | Subutilização em baixa carga.                   |
| (escolhida)              |                                               |                                                 |
| Worker Pool Elástico     | Adaptável à carga.                            | Complexo, risco de sobrecarga, tuning delicado. |

**Decisão**: Worker pool fixo configurável via variável de ambiente. O operador
ajusta conforme a carga típica. Para variações sazonais, um HPA (Horizontal
Pod Autoscaler) pode ajustar o número de réplicas do consumer.

### T5 — Validação Restritiva vs Permissiva

| Opção                           | Prós                                         | Contras                                        |
|---------------------------------|----------------------------------------------|------------------------------------------------|
| **Validação Restritiva**        | Qualidade dos dados garantida, segurança.    | Mensagens com formato diferente são rejeitadas, |
| (escolhida)                     |                                              | maior taxa de DLQ.                              |
| Validação Permissiva            | Menos rejeições, tolerância a variações.     | Dados inconsistentes, riscos de segurança.      |

**Decisão**: Validação restritiva. O schema do payload é bem definido. Mensagens
que não se encaixam são erro do produtor e devem ser corrigidas na origem.

### T6 — DLQ com Tópico Separado vs Mesmo Tópico + Flag

| Opção                           | Prós                                         | Contras                                        |
|---------------------------------|----------------------------------------------|------------------------------------------------|
| **DLQ em tópico separado**      | Isolamento, consumidor dedicado, sem risco   | Complexidade operacional (mais 1 tópico).       |
| (escolhida)                     | de loop de consumo.                          |                                                 |
| DLQ no mesmo tópico + flag      | Simples, sem tópico extra.                   | Risco de loop infinito, consumidor precisa      |
|                                 |                                              | filtrar, difícil isolar.                        |

**Decisão**: Tópico dedicado `payment.events.dlq`. Isolamento total, consumidor
separado pode tratar com regras diferentes (alertas, reprocessamento manual).

---

## Decisões Arquiteturais (ADRs)

### ADR-001: Kafka como Message Broker

**Contexto**: Necessário sistema de mensageria distribuído, com garantia
at-least-once e replay de mensagens.

**Decisão**: Kafka (via sarama).

**Justificativa**: Maduro, suporte a consumer groups, partições para
paralelismo, retenção configurável, ecossistema amplo.

### ADR-002: Redis para Idempotência e Status

**Contexto**: Necessário armazenamento de baixa latência para verificação de
idempotência e status atual.

**Decisão**: Redis.

**Justificativa**: Operação `SET NX` atômica para idempotência, TTL nativo,
latência sub-milissegundo, pipeline para operações atômicas.

### ADR-003: DynamoDB para Histórico

**Contexto**: Necessário armazenamento permanente, escalável, com chave
composta para evitar duplicatas.

**Decisão**: DynamoDB.

**Justificativa**: Gerenciado, escalável, `ConditionExpression` nativa,
integração com AWS ecosystem, sem schema rígido.

### ADR-004: Retry Síncrono (não republicar no Kafka)

**Contexto**: Mensagens com falha recuperável precisam ser retentadas.

**Decisão**: Retry síncrono com backoff exponencial.

**Justificativa**: Simplicidade, evita loops de repúblicação, preserva
ordenação dentro da partição. Mensagens não são commited até sucesso ou DLQ.
