# 06 — Riscos e Trade-offs

## Riscos

### R01 — Producer Service Indisponível

**Probabilidade**: Média (serviço standalone pode não estar rodando)
**Impacto**: Alto (impossibilidade de publicar)

**Mitigação**:
- Handler faz HTTP POST com timeout de 10s
- Se conexão falhar, retorna 502 com mensagem "Producer Service indisponível"
- A UI e a dashboard continuam funcionando normalmente (apenas publish afetado)
- Log estruturado de erro com detalhes da falha

### R02 — EventBus (Redis) Indisponível

**Probabilidade**: Baixa (Redis é serviço essencial, já usado pela dashboard)
**Impacto**: Baixo (não afeta o publish — a UI não publica mais no EventBus)

**Nota**: O EventBus (Redis Pub/Sub) agora é responsabilidade exclusiva do
Consumer (spec `0001-kafka-payment-consumer`), que publica eventos após
processá-los. A UI apenas consome o EventBus para o SSE feed da dashboard.

### R03 — Rate Limiting / Abuso

**Probabilidade**: Baixa (ferramenta dev, sem exposição pública)
**Impacto**: Baixo (poucos usuários simultâneos)

**Mitigação**:
- Rate limit de 5 req/s por IP (1 request a cada 200ms)
- Implementação: mapa `sync.Map` com timestamps por IP
- Cleanup periódico de entradas antigas

### R04 — Payload Gigante

**Probabilidade**: Baixa (descrição free-text pode ser longa)
**Impacto**: Baixo (apenas 1 evento por request)

**Mitigação**:
- `http.MaxBytesReader` limita a 100KB
- Validador já limita description a 255 caracteres
- Frontend também valida maxlength=255

### R05 — Concorrência / Race Condition

**Probabilidade**: Baixa (stdlib `net/http` é thread-safe)
**Impacto**: Médio

**Mitigação**:
- `http.Client` do stdlib é thread-safe e pode ser reusado por múltiplas goroutines
- Cada request HTTP cria seu próprio contexto com timeout (`context.WithTimeout`)
- Rate limit usa `sync.Map` (thread-safe)
- Múltiplas requests simultâneas são serializadas pelo `http.Transport` interno

### R06 — Duplicidade de Eventos

**Probabilidade**: Média (usuário pode clicar "Publicar" duas vezes)
**Impacto**: Baixo (sistema já lida com idempotência no consumer)

**Mitigação**:
- Botão "Publicar" desabilitado durante envio (frontend)
- Sistema de idempotência já existe no consumer (spec `0001`)

## Trade-offs

### T01 — Chamar Producer via HTTP (vs. Kafka embutido na UI)

**Decisão**: UI removeu o produtor Kafka embutido e passou a chamar o
Producer Service via HTTP.

**Prós**:
- **Separação de responsabilidades**: UI não precisa saber de Kafka, brokers,
  tópicos, ou configurar Sarama
- **Menos dependências**: UI não depende mais de `sarama` (Kafka client)
- **Menos pontos de falha**: UI não precisa de conectividade de rede com Kafka
- **Escalabilidade**: Producer Service pode ser escalado independentemente
- **Reuso**: O mesmo Producer Service atende CLI e UI (endpoints HTTP unificados)
- **Deploy independente**: UI pode ser atualizada sem afetar a lógica de publicação

**Contras**:
- **Latência adicional**: ~1-2ms extra por chamada HTTP (vs. publicação direta)
- **Nova dependência de rede**: UI precisa alcançar o Producer Service via HTTP
- **Nova falha potencial**: Producer Service pode estar offline
- **Manutenção de mais um serviço**: Producer Service precisa ser deployado e
  monitorado separadamente

**Justificativa**: Aceito porque:
- A UI já depende de Redis e DynamoDB (Producer Service é mais claro que
  adicionar Kafka como dependência direta)
- O Producer Service já existe (spec `0003-cli-producer`) — é o mesmo código
  com um wrapper HTTP
- A latência adicional é negligenciável para casos de uso manuais (desenvolvedor/QA)
- A separação permite que o Producer Service seja usado por CLI, UI e futuros
  clientes (integrações, scripts)

### T02 — Delegar Publicação ao Producer (vs. Publicar Direto na UI)

**Decisão**: A UI não publica mais diretamente no Kafka nem no EventBus.
Toda publicação é delegada ao Producer Service via HTTP.

**Alternativa considerada**: Manter SyncProducer embutido na UI (arquitetura anterior).

**Motivo da rejeição**:
- Acoplamento desnecessário: UI não precisa conhecer detalhes de infraestrutura Kafka
- Duplicação de lógica: CLI e UI teriam código de publicação duplicado
- Dificuldade de manutenção: mudanças na lógica de publicação exigiriam alterar
  dois lugares (CLI + UI)
- O Producer Service já existe e faz tudo que a UI precisa

### T03 — Frontend Vanilla vs. Framework

**Decisão**: HTML + CSS + JS vanilla (sem frameworks)

**Prós**:
- Zero dependências novas
- Consistente com o dashboard existente (também vanilla)
- Carregamento instantâneo (sem bundle, sem build step)
- Fácil manutenção (qualquer dev pode editar)

**Contras**:
- Mais verboso que React/Vue para estado complexo
- Sem componentização nativa

**Justificativa**: O estado da página é simples (1 formulário + 1 tabela + 1 preview).
Não justifica adicionar framework.

### T04 — Endpoint Bulk Separado vs. Mesmo Endpoint

**Decisão**: Endpoint separado `POST /api/publish/bulk`

**Alternativa**: Enviar array de eventos para `POST /api/publish`

**Motivo da decisão**: Clareza de código, simplicidade de validação,
e reuso de `producer.GenerateBulkEvents()`.

## Impacto em Specs Anteriores

| Spec                    | Impacto                                        |
|-------------------------|------------------------------------------------|
| 0001 (consumer)         | Nenhum (consumer continua idêntico)            |
| 0002 (dashboard)        | Baixo (adição de link de navegação no header + docs) |
| 0003 (CLI producer)     | **Médio**: O `producer.Service` (spec `0003`) agora também atende via HTTP.
                           A interface `Service.Publish()` ganhou um wrapper HTTP
                           para receber requisições da UI. O header `source`
                           permanece `"cli-producer"` independente da origem.
                           O CLI continua funcionando exatamente como antes.      |
| Documentação            | Novo: documentação é copiada para `static/docs/` no build e servida em `/docs/` |
