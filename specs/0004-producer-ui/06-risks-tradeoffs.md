# 06 — Riscos e Trade-offs

## Riscos

### R01 — Kafka Broker Indisponível

**Probabilidade**: Média (em desenvolvimento, Kafka pode não estar rodando)
**Impacto**: Alto (impossibilidade de publicar)

**Mitigação**:
- Handler verifica se `kafkaProducer` é `nil` antes de publicar
- Retorna 502 com mensagem clara "Kafka não disponível"
- Servidor não falha na inicialização se Kafka estiver offline
- Log estruturado de warning na inicialização

### R02 — EventBus (Redis) Indisponível

**Probabilidade**: Baixa (Redis é serviço essencial, já usado pela dashboard)
**Impacto**: Médio (evento é publicado no Kafka mas não aparece no SSE feed)

**Mitigação**:
- Publicação no EventBus é feita **após** sucesso no Kafka
- Se EventBus falhar, loga warning mas não falha a request
- Evento será processado pelo consumer e aparecerá na dashboard via polling
- O EventBus já trata subscriber lento com dropping (spec existente)

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

**Probabilidade**: Baixa (SyncProducer do Sarama é thread-safe)
**Impacto**: Médio

**Mitigação**:
- `sarama.SyncProducer.SendMessage` é thread-safe
- Múltiplas goroutines (várias requests simultâneas) podem publicar sem race
- EventBus.Publish usa Redis Pub/Sub que é thread-safe

### R06 — Duplicidade de Eventos

**Probabilidade**: Média (usuário pode clicar "Publicar" duas vezes)
**Impacto**: Baixo (sistema já lida com idempotência no consumer)

**Mitigação**:
- Botão "Publicar" desabilitado durante envio (frontend)
- Sistema de idempotência já existe no consumer (spec `0001`)

## Trade-offs

### T01 — Injetar SyncProducer no Server (vs. serviço separado)

**Decisão**: Injetar diretamente no Server/Handlers da UI

**Prós**:
- Simplicidade: sem novo microsserviço, sem nova porta, sem nova goroutine
- Código reusa tipos e interfaces existentes (`models.PaymentEvent`, `validator`)
- Latência mínima (publicação direta no Kafka)

**Contras**:
- Acopla UI ao Kafka (se Kafka não estiver disponível, UI ainda funciona mas sem publish)
- Adiciona dependência de infraestrutura ao serviço web (precisa de Kafka brokers configurados)
- Aumenta complexidade do `cmd/ui/main.go` (precisa conectar Kafka)

**Justificativa**: Aceito porque:
- A UI já depende de Redis e DynamoDB (Kafka é mais uma dependência de infra)
- O consumer já faz o contrário (conecta Kafka + Redis + DynamoDB)
- A equipe já conhece Sarama e o padrão de injeção

### T02 — Publicar no EventBus (Redis Pub/Sub) vs. Esperar Consumer

**Decisão**: Publicar no EventBus imediatamente após Kafka

**Alternativa considerada**: Publicar apenas no Kafka e esperar o consumer
processar e publicar no EventBus.

**Motivo da rejeição**:
- Latência maior (evento publicado na UI só apareceria após consumer processar)
- Mais complexidade de depuração
- Se o consumer estiver offline, evento nunca aparece na dashboard
- O custo de publicar no Redis Pub/Sub é baixíssimo (~1ms)

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
| 0003 (CLI producer)     | Nenhum (CLI continua funcionando)              |
| Documentação            | Novo: documentação é copiada para `static/docs/` no build e servida em `/docs/` |
