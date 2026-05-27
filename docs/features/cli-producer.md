# CLI Producer

> **Nota:** O producer agora também possui um modo **servidor HTTP** (`producer serve`) que expõe endpoints REST para publicação de eventos. Consulte [`docs/producer.md`](../producer.md) para a documentação completa do producer, incluindo ambos os modos de operação.

## O que é

O **CLI Producer** é uma ferramenta de linha de comando para publicar eventos de pagamento no tópico Kafka `payment.events` diretamente do terminal.

## Por que existe

Desenvolvedores e engenheiros de QA precisam de uma ferramenta rápida para:

- **Testar o consumer** sem dependência de sistemas externos
- **Simular cenários** específicos (pagamentos falhos, inválidos, etc.)
- **Executar testes de carga** controlados com bulk mode e rate limiting
- **Depurar** o comportamento do consumer com payloads customizados

## Como funciona

### Fluxo de Uso

```
Usuário invoca CLI
        │
        ▼
┌─────────────────────┐
│ 1. Parse flags      │ ← flag stdlib
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│ 2. Fonte dos dados  │
│    ├─ --payload     │ → JSON direto
│    ├─ stdin pipe    │ → echo | producer
│    ├─ --file        │ → arquivo JSON
│    └─ flags + auto  │ → geração automática
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│ 3. Validação        │ ← internal/validator (reusado do consumer)
└─────────┬───────────┘
          │
     ┌────┴────┐
     │ dry-run │ → Exibe JSON e sai
     └────┬────┘
          │ Normal
          ▼
┌─────────────────────┐
│ 4. Publicação Kafka │ ← sarama SyncProducer
│    (com timeout)    │
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│ 5. Exibir resultado │ → partition/offset ou erro
└─────────────────────┘
```

### Comandos e Flags

```
Uso: producer publish [flags]

Flags:
  --payment-id STRING    UUID v4 do pagamento (auto-gerado se vazio)
  --status STRING        Status: pending|confirmed|failed|refunded (default: confirmed)
  --amount FLOAT         Valor do pagamento > 0 (default: 100.00)
  --currency STRING      ISO 4217 (default: BRL)
  --description STRING   Descrição opcional (max 255 chars)

  --topic STRING         Tópico Kafka (default: payment.events)
  --brokers STRING       Brokers Kafka separados por vírgula (default: localhost:9092)

  --payload STRING       JSON payload direto (sobrescreve flags individuais)
  --file STRING          Arquivo JSON com array de eventos
  --count INT            Número de eventos em bulk mode (default: 1)
  --rate INT             Eventos por segundo em bulk mode (0 = sem limite)

  --dry-run              Apenas exibir JSON sem publicar
  --json-output          Saída em JSON (para scripting)
  --help                 Exibir ajuda

Stdin:
  echo '{"payment_id":"...","status":"confirmed"}' | producer publish
```

### Ordem de Precedência da Fonte de Dados

1. `--payload` → JSON direto (objeto único ou array)
2. **stdin pipe** → ler do terminal
3. `--file` → ler eventos de arquivo JSON (array ou objeto único)
4. `--count > 1` → gerar N eventos automáticos (bulk)
5. **Default** → gerar 1 evento com flags + defaults

### Modos de Saída

#### Modo Texto (default)

```
✓ Published a1b2c3d4-e5f6-7890-abcd-ef1234567890 → partition 0, offset 42
✗ Failed b2c3d4e5-f6a7-8901-bcde-f12345678901 → validation error: amount must be greater than 0
```

#### Modo JSON (`--json-output`)

```json
{"status":"success","payment_id":"a1b2c3d4-...","partition":0,"offset":42}
{"status":"error","payment_id":"b2c3d4e5-...","error":"validation error: ..."}
```

### Geração de Eventos

A CLI gera eventos automaticamente quando nenhuma fonte explícita é fornecida:

**Evento único** (default):
- `payment_id`: UUID v4 aleatório (se não fornecido)
- `status`: `confirmed` (default)
- `amount`: 100.00 (default)
- `currency`: BRL (default)
- `description`: vazio
- `timestamp`: atual (RFC3339)

**Bulk mode** (`--count N`):
- Gera N eventos com dados sequenciais
- `amount` varia: 10, 20, 30... (N × 10)
- `description`: "Bulk event X of N"
- Cada evento tem UUID único

### Rate Limiting

Em bulk mode, `--rate` controla o número de eventos por segundo:

```bash
# 10 eventos por segundo
producer publish --count 50 --rate 10
```

- Usa `time.Ticker` para controle de taxa
- Precisão de ±5% para rates moderados (1-50 ev/s)
- `--rate 0` (ou omitido) = sem limite (máxima velocidade)

## Configuração

| Parâmetro     | Default          | Flag          | Descrição                          |
|---------------|------------------|---------------|------------------------------------|
| Brokers Kafka | `localhost:9092` | `--brokers`   | Lista separada por vírgula         |
| Tópico        | `payment.events` | `--topic`     | Tópico Kafka destino               |
| Timeout Kafka | 10s              | (fixo)        | Timeout do SyncProducer            |
| Dial timeout  | 5s               | (fixo)        | Timeout de conexão TCP             |

## Códigos de Saída

| Situação                                   | Exit Code |
|--------------------------------------------|-----------|
| Todos eventos publicados com sucesso       | 0         |
| Algum evento falhou (validação/Kafka)      | 1         |
| Erro de parse de flags                     | 1         |
| Erro de I/O (arquivo)                      | 1         |
| Erro de conexão Kafka                      | 1         |
| Ctrl+C durante publicação                  | 1         |

## Exemplos de Uso

### Publicar evento com flags individuais

```bash
producer publish \
  --payment-id "a1b2c3d4-e5f6-7890-abcd-ef1234567890" \
  --status confirmed \
  --amount 150.00 \
  --currency BRL \
  --description "Pagamento de teste"
```

### Publicar com JSON direto

```bash
producer publish --payload '{
  "payment_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "status": "confirmed",
  "amount": 150.00,
  "currency": "BRL"
}'
```

### Modo dry-run (apenas validar e exibir)

```bash
producer publish --dry-run

# Com saída JSON
producer publish --dry-run --json-output
```

### Bulk mode

```bash
# Publicar 100 eventos
producer publish --count 100

# Publicar 50 eventos a 5 por segundo
producer publish --count 50 --rate 5
```

### Pipe do stdin

```bash
echo '{"payment_id":"abc-123","status":"confirmed","amount":100,"currency":"BRL"}' | producer publish
```

### Arquivo JSON

```bash
# Arquivo com array de eventos
producer publish --file events.json

# Arquivo com objeto único
echo '{"payment_id":"abc","status":"confirmed","amount":100,"currency":"BRL"}' > event.json
producer publish --file event.json
```

### Saída JSON para scripting

```bash
producer publish --count 10 --json-output | jq -s 'map(select(.status == "success")) | length'
```

### Simular cenários de erro

```bash
# Amount inválido (deve falhar na validação)
producer publish --amount -10

# Status inválido
producer publish --status invalid

# UUID inválido
producer publish --payment-id "not-a-uuid"
```

## Edge Cases

| Cenário                              | Comportamento                                               |
|--------------------------------------|-------------------------------------------------------------|
| `--count 0`                          | Tratado como 1 (gera 1 evento)                              |
| `--payload` + `--file` conflitantes  | `--payload` tem prioridade                                   |
| Stdin vazio (sem pipe)               | Usa default/flags individuais                                |
| Pipe com conteúdo vazio              | Erro "stdin is empty", exit 1                                |
| Arquivo JSON mal formatado           | Erro descritivo, exit 1                                      |
| Arquivo > 10 MB                      | Rejeitado, exit 1                                            |
| Kafka broker offline                 | Erro de conexão, exit 1                                      |
| Ctrl+C durante bulk                  | Publicação interrompida, resultados parciais exibidos        |
| Evento inválido no meio do bulk      | Evento pula (Result com erro), restante continua             |
| Payload com campos extras            | Aceito (tolerância do json.Unmarshal)                        |
| `--rate` muito alto (>100/s)         | Rate real pode ser menor que o configurado (imprecisão do Ticker) |
