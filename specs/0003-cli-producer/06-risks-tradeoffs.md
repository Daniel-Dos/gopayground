# 06 — Riscos e Tradeoffs

## Riscos

### R1 — Publicação de Eventos Inválidos no Tópico

| Atributo       | Detalhe                                                                 |
|----------------|-------------------------------------------------------------------------|
| **Descrição**  | Usuário publica payload inválido que passa pela validação da CLI mas     |
|                | não é válido para o consumer (divergência de validação).                 |
| **Impacto**    | Evento inválido no tópico → consumer rejeita → DLQ.                     |
| **Probabilidade** | Baixa (validação é compartilhada com o consumer).                     |
| **Mitigação**  | Reuso do mesmo `internal/validator` usado pelo consumer.                 |
|                | A validação é exatamente a mesma — o que passa na CLI passa no consumer. |
| **Residual**   | Mínimo. Se houver divergência no futuro, ambos usarão o mesmo pacote.    |

### R2 — Flood no Kafka em Bulk sem Rate Limit

| Atributo       | Detalhe                                                                 |
|----------------|-------------------------------------------------------------------------|
| **Descrição**  | Usuário executa `--count 100000` sem `--rate` e sobrecarrega o Kafka.   |
| **Impacto**    | Pico de produção, possível throttling, latência aumentada.              |
| **Probabilidade** | Média (usuário pode não saber o que está fazendo).                    |
| **Mitigação**  | Documentar que `--rate` é recomendado para counts altos.                |
|                | A CLI não é uma ferramenta de produção — assume-se uso responsável.     |
| **Residual**   | Aceito. Ferramenta de desenvolvimento, não de produção.                 |

### R3 — Exposição de Dados Sensíveis

| Atributo       | Detalhe                                                                 |
|----------------|-------------------------------------------------------------------------|
| **Descrição**  | Usuário publica dados reais de pagamento como teste e eles aparecem     |
|                | nos logs da CLI, no terminal, ou em output JSON para script.            |
| **Impacto**    | Vazamento de dados financeiros.                                         |
| **Probabilidade** | Baixa (ferramenta de desenvolvedor, ambiente controlado).             |
| **Mitigação**  | Documentar que a CLI é para testes com dados fictícios.                 |
|                | Output JSON não mascarar valores (assumindo ambiente de dev).           |
| **Residual**   | Aceito. Responsabilidade do usuário.                                    |

### R4 — Rate Limiting Impreciso

| Atributo       | Detalhe                                                                 |
|----------------|-------------------------------------------------------------------------|
| **Descrição**  | `time.Ticker` não é preciso para rates muito altos (> 100/s) devido a   |
|                | latência de publicação + scheduling do Go.                               |
| **Impacto**    | Rate real pode ser ligeiramente menor que o configurado.                |
| **Probabilidade** | Alta para rates altos.                                                 |
| **Mitigação**  | Ticker simples é adequado para ferramenta de dev (~1-50 ev/s).          |
|                | Para rates precisos, usar token bucket (fora do escopo).               |
| **Residual**   | Aceito. Precisão de ±5% é suficiente para testes de carga manuais.      |

### R5 — Perda de Mensagem se CLI é Interrompida

| Atributo       | Detalhe                                                                 |
|----------------|-------------------------------------------------------------------------|
| **Descrição**  | Usuário faz Ctrl+C durante bulk mode. Mensagens não publicadas são      |
|                | perdidas (não há transação).                                            |
| **Impacto**    | Perda de eventos não publicados.                                        |
| **Probabilidade** | Média (usuário pode cancelar bulk longo).                             |
| **Mitigação**  | Context cancel interrompe graciosamente. Mensagens já publicadas foram  |
|                | confirmadas (SyncProducer síncrono).                                    |
| **Residual**   | Aceito. Ferramenta de teste, não de produção.                           |

---

## Tradeoffs

### T1 — `flag` stdlib vs Cobra

| Opção                   | Prós                                        | Contras                                      |
|-------------------------|---------------------------------------------|----------------------------------------------|
| **`flag` stdlib**       | Zero dependências, suficiente para o escopo | Sem subcomandos aninhados, help automático    |
| (escolhida)             |                                             | menos rico.                                  |
| Cobra / pflag           | Subcomandos, help rico, autocomplete.       | Nova dependência, complexidade desnecessária.|

**Decisão**: `flag` stdlib. O CLI tem apenas um comando (`publish`) com flags.
Não justifica adicionar uma nova dependência.

### T2 — Binário Separado vs Subcomando do Consumer

| Opção                   | Prós                                        | Contras                                      |
|-------------------------|---------------------------------------------|----------------------------------------------|
| **Binário separado**    | Independência, sem acoplamento, build       | Dois binários para gerenciar.                |
| (escolhida)             | isolado.                                     |                                              |
| Subcomando do consumer  | Único binário.                              | Consumer ganha dependência de CLI, fica      |
|                         |                                             | mais complexo.                               |

**Decisão**: Binário separado `cmd/producer`. O consumer é um serviço
long-running; a CLI é uma ferramenta interativa. Misturar os dois aumentaria
a complexidade e tamanho do consumer.

### T3 — SyncProducer vs AsyncProducer

| Opção                   | Prós                                        | Contras                                      |
|-------------------------|---------------------------------------------|----------------------------------------------|
| **SyncProducer**        | Feedback síncrono (partition, offset), erro | Menor throughput (1 msg por requisição).     |
| (escolhida)             | imediato.                                    |                                              |
| AsyncProducer           | Alto throughput, pipeline de mensagens.     | Perda de feedback imediato, recuperação de   |
|                         |                                             | erro mais complexa.                          |

**Decisão**: SyncProducer. A CLI é interativa — o usuário espera confirmação
de cada evento. Bulk mode com rate limiting mantém o throughput controlado.

### T4 — Validação no Producer vs Confiar no Consumer

| Opção                           | Prós                                        | Contras                                      |
|---------------------------------|---------------------------------------------|----------------------------------------------|
| **Validar na CLI** (escolhida)  | Feedback imediato, não polui o tópico.      | Código duplicado (compartilhado via pacote). |
| Confiar apenas no consumer      | Simplicidade da CLI.                        | Eventos inválidos vão para DLQ, feedback     |
|                                 |                                             | demorado.                                    |

**Decisão**: Reusar `internal/validator` na CLI. A validação é o mesmo pacote
usado pelo consumer, garantindo consistência sem duplicação.

### T5 — JSON Output por Linha vs Array

| Opção                           | Prós                                        | Contras                                      |
|---------------------------------|---------------------------------------------|----------------------------------------------|
| **JSON Lines** (escolhido)      | Streaming, fácil de pipear, jq -s .         | Não é um array JSON válido (mas é parseável  |
|                                 | funciona.                                    | linha a linha).                              |
| Array JSON único                | JSON válido completo.                       | Precisa saber o total antes, não streaming.  |

**Decisão**: JSON Lines. Mais adequado para scripting (`jq -s .` para array,
`while read -r line; do ...` para streaming).

---

## Decisões Arquiteturais (ADRs)

### ADR-001: Producer Service como Pacote Separado

**Contexto**: A lógica de publicação deve ser testável isoladamente do CLI.

**Decisão**: Criar `internal/producer` com interface `Service` e implementação.

**Justificativa**: Separação clara entre lógica de negócio (publicar) e
apresentação (CLI). Facilita testes unitários e possível reuso futuro.

### ADR-002: Reuso do Validator Existente

**Contexto**: A CLI precisa validar eventos antes de publicar.

**Decisão**: Importar e usar `internal/validator.New()` diretamente.

**Justificativa**: Consistência garantida com o consumer. Sem duplicação de
lógica de validação. Se as regras mudarem, ambos mudam junto.

### ADR-003: Sem Dependências Novas

**Contexto**: O projeto já possui sarama, uuid, validator, testify.

**Decisão**: Construir a CLI apenas com stdlib + dependências já existentes.

**Justificativa**: Evitar inchaço de go.mod. `flag` stdlib é suficiente.
Mantém o build rápido e o binário pequeno.
