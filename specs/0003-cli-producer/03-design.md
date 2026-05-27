# 03 — Design

## Estrutura de Pastas

```
cmd/
  producer/
    main.go               # Entry point, flag parsing, fluxo principal
    main_test.go           # Testes de integração do entry point

internal/
  producer/
    producer.go            # Lógica de negócio: geração, validação, publicação
    producer_test.go       # Testes unitários do producer

internal/
  models/                  # Já existente — reusado
  validator/               # Já existente — reusado
  dlq/                     # Já existente — reusado (SyncProducer interface)
```

## Diagrama de Fluxo

```
┌──────────────────────────────────────────────────────────────────┐
│                        CLI (cmd/producer/main.go)                │
│                                                                  │
│  1. Parse flags (flag stdlib)                                    │
│  2. Determinar fonte dos dados:                                  │
│     ├─ --payload "..."  → JSON string direta                     │
│     ├─ stdin pipe       → ler stdin                              │
│     ├─ --file path      → ler arquivo JSON                       │
│     └─ flags individuais + defaults → montar evento              │
│  3. Se --count > 1     → gerar N eventos (bulk)                 │
│  4. Se --dry-run       → print JSON e sair                       │
│  5. Validar cada evento com internal/validator                   │
│  6. Publicar no Kafka com sarama SyncProducer                    │
│  7. Exibir resultado (partição/offset ou erro)                   │
└──────────────────────────┬───────────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────────────┐
│                  internal/producer/producer.go                    │
│                                                                  │
│  - GenerateEvent(opts) → *models.PaymentEvent                    │
│  - ValidateEvent(data) → error (delega para validator)           │
│  - PublishEvent(ctx, msg) → (partition, offset, error)          │
│  - PublishBulk(ctx, events, rate) → []Result                    │
└──────────────────────────────────────────┬───────────────────────┘
                                           │
                                           ▼
┌──────────────────────────────────────────────────────────────────┐
│                    Kafka (tópico payment.events)                  │
│                                                                  │
│  syncProducer.SendMessage(msg)                                   │
│    → retorna partição, offset, erro                              │
└──────────────────────────────────────────────────────────────────┘
```

## Comandos e Flags

```
Usage:
  producer publish [flags]

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

### Comportamento da Fonte de Dados (Ordem de Precedência)

1. Se `--payload` for fornecido → usar como JSON direto.
2. Se stdin não estiver vazio (pipe) → ler do stdin.
3. Se `--file` for fornecido → ler eventos do arquivo.
4. Se `--count > 1` → gerar N eventos com dados sequenciais.
5. Caso contrário → gerar 1 evento com defaults + flags fornecidas.

## Componentes Detalhados

### 1. Entry Point (`cmd/producer/main.go`)

Responsabilidades:
- Definir e parsear flags usando `flag` stdlib.
- Detectar stdin pipe (`os.Stdin.Stat()`).
- Ler dados da fonte apropriada.
- Instanciar sarama SyncProducer.
- Instanciar Producer service.
- Executar publicação.
- Exibir resultado no formato adequado (texto ou JSON).
- Tratar sinais para graceful shutdown.

```go
func main() {
    // 1. Parse flags
    flags := parseFlags()

    // 2. Detectar fonte e obter eventos
    events, err := getEvents(flags)
    if err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }

    // 3. Validar eventos
    validator := validator.New()

    // 4. Se dry-run, exibir e sair
    if flags.dryRun {
        printEvents(events, flags.jsonOutput)
        return
    }

    // 5. Configurar Kafka producer
    config := sarama.NewConfig()
    config.Producer.Return.Successes = true
    config.Producer.Timeout = 10 * time.Second
    config.Net.DialTimeout = 5 * time.Second

    syncProducer, err := sarama.NewSyncProducer(strings.Split(flags.brokers, ","), config)
    if err != nil {
        fmt.Fprintf(os.Stderr, "error: failed to create producer: %v\n", err)
        os.Exit(1)
    }
    defer syncProducer.Close()

    // 6. Criar service e publicar
    svc := producer.New(syncProducer, flags.topic, validator)

    results := svc.Publish(ctx, events, flags.rate)

    // 7. Exibir resultados
    printResults(results, flags.jsonOutput)
}
```

### 2. Producer Service (`internal/producer/producer.go`)

Interface:

```go
type Service interface {
    Publish(ctx context.Context, events []*models.PaymentEvent, rate int) []Result
}

type Result struct {
    Event     *models.PaymentEvent
    Partition int32
    Offset    int64
    Error     error
}

type Config struct {
    Brokers string
    Topic   string
}
```

Implementação:

```go
type service struct {
    producer  sarama.SyncProducer
    topic     string
    validator validator.Validator
}

func New(producer sarama.SyncProducer, topic string, v validator.Validator) Service {
    return &service{
        producer:  producer,
        topic:     topic,
        validator: v,
    }
}

func (s *service) Publish(ctx context.Context, events []*models.PaymentEvent, rate int) []Result {
    results := make([]Result, 0, len(events))

    var ticker *time.Ticker
    if rate > 0 {
        ticker = time.NewTicker(time.Second / time.Duration(rate))
        defer ticker.Stop()
    }

    for i, event := range events {
        // Rate limiting
        if ticker != nil && i > 0 {
            select {
            case <-ticker.C:
            case <-ctx.Done():
                results = append(results, Result{
                    Event: event,
                    Error: ctx.Err(),
                })
                return results
            }
        }

        // Validate
        eventData, _ := json.Marshal(event)
        _, err := s.validator.Validate(ctx, eventData)
        if err != nil {
            results = append(results, Result{Event: event, Error: err})
            continue
        }

        // Publish
        msg := &sarama.ProducerMessage{
            Topic: s.topic,
            Key:   sarama.StringEncoder(event.PaymentID),
            Value: sarama.ByteEncoder(eventData),
            Headers: []sarama.RecordHeader{
                {Key: []byte("source"), Value: []byte("cli-producer")},
                {Key: []byte("timestamp"), Value: []byte(time.Now().UTC().Format(time.RFC3339))},
            },
        }

        partition, offset, err := s.producer.SendMessage(msg)
        if err != nil {
            results = append(results, Result{Event: event, Error: err})
            continue
        }

        results = append(results, Result{
            Event:     event,
            Partition: partition,
            Offset:    offset,
        })
    }

    return results
}
```

### 3. Geração de Eventos (`internal/producer/producer.go`)

```go
// GenerateEvent cria um PaymentEvent a partir de flags (ou defaults).
func GenerateEvent(paymentID, status string, amount float64, currency, description string) *models.PaymentEvent {
    if paymentID == "" {
        paymentID = uuid.New().String()
    }
    if status == "" {
        status = "confirmed"
    }
    if amount <= 0 {
        amount = 100.00
    }
    if currency == "" {
        currency = "BRL"
    }

    return &models.PaymentEvent{
        PaymentID:   paymentID,
        Status:      status,
        Amount:      amount,
        Currency:    currency,
        Description: description,
        Timestamp:   time.Now().UTC().Format(time.RFC3339),
    }
}

// GenerateBulkEvents gera N eventos com dados sequenciais (bulk).
// O payment_id recebe sufixo numérico para evitar duplicatas de idempotência.
func GenerateBulkEvents(count int, prefix string) []*models.PaymentEvent {
    events := make([]*models.PaymentEvent, count)
    for i := 0; i < count; i++ {
        events[i] = GenerateEvent(
            uuid.New().String(),
            "confirmed",
            float64(i+1)*10.0,         // amount varia: 10, 20, 30...
            "BRL",
            fmt.Sprintf("Bulk event %d of %d", i+1, count),
        )
    }
    return events
}
```

### 4. Tratamento de Stdin

```go
func isStdinPipe() bool {
    stat, err := os.Stdin.Stat()
    if err != nil {
        return false
    }
    return (stat.Mode() & os.ModeCharDevice) == 0
}

func readStdin() ([]byte, error) {
    return io.ReadAll(os.Stdin)
}
```

### 5. Tratamento de Arquivo

```go
func readEventsFromFile(path string) ([]*models.PaymentEvent, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read file error: %w", err)
    }

    var events []*models.PaymentEvent
    if err := json.Unmarshal(data, &events); err != nil {
        // Tenta como objeto único
        var single models.PaymentEvent
        if err2 := json.Unmarshal(data, &single); err2 != nil {
            return nil, fmt.Errorf("json unmarshal error: %w", err)
        }
        events = []*models.PaymentEvent{&single}
    }

    if len(events) == 0 {
        return nil, fmt.Errorf("no events found in file")
    }

    return events, nil
}
```

### 6. Saída Formatada

**Modo texto (default):**
```
✓ Published event a1b2c3d4-e5f6-7890-abcd-ef1234567890 → partition 0, offset 42
✓ Published event b2c3d4e5-f6a7-8901-bcde-f12345678901 → partition 2, offset 15
✗ Failed event ... → validation error: amount must be greater than 0
```

**Modo JSON (`--json-output`):**
```json
{"status":"success","payment_id":"a1b2...","partition":0,"offset":42}
{"status":"error","payment_id":"b2c3...","error":"validation error: ..."}
```

---

## Contratos de Interface

```go
// Producer service
type ProducerService interface {
    Publish(ctx context.Context, events []*models.PaymentEvent, rate int) []Result
}

// Validador (reusado do consumer)
type Validator interface {
    Validate(ctx context.Context, data []byte) (*models.PaymentEvent, error)
}

// SyncProducer (já definido em internal/dlq, será reusado ou ter seu próprio)
type SyncProducer interface {
    SendMessage(msg *sarama.ProducerMessage) (partition int32, offset int64, err error)
}
```

---

## Configuração

| Parâmetro          | Default          | Flag           | Descrição                          |
|--------------------|------------------|----------------|------------------------------------|
| Brokers Kafka      | localhost:9092   | `--brokers`    | Lista separada por vírgula         |
| Tópico             | payment.events   | `--topic`      | Tópico Kafka destino               |
| Kafka timeout      | 10s              | (fixo)         | Timeout do producer                |
| Dial timeout       | 5s               | (fixo)         | Timeout de conexão TCP             |
