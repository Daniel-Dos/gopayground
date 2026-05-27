# 04 — Plano de Implementação

## Ordem de Implementação

```
 1. internal/producer/producer.go   → Service, geração, resultados
 2. internal/producer/producer_test.go → Testes unitários
 3. cmd/producer/main.go             → Entry point, flag parsing, integração
 4. cmd/producer/main_test.go        → Testes de integração do CLI
 5. Makefile                         → Atualizar com build/run producer
```

---

## Passo 1 — Producer Service

**Arquivos**: `internal/producer/producer.go`

### O que implementar

```go
package producer

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "exemplo.com/teste/internal/models"
    "exemplo.com/teste/internal/validator"
    "github.com/IBM/sarama"
    "github.com/google/uuid"
)

// SyncProducer define a interface mínima para publicação Kafka.
// Reutiliza o mesmo contrato de internal/dlq para consistência.
type SyncProducer interface {
    SendMessage(msg *sarama.ProducerMessage) (partition int32, offset int64, err error)
}

// Service define o contrato principal do producer CLI.
type Service interface {
    Publish(ctx context.Context, events []*models.PaymentEvent, rate int) []Result
}

// Result representa o resultado da publicação de um evento.
type Result struct {
    Event     *models.PaymentEvent
    Partition int32
    Offset    int64
    Error     error
}

type service struct {
    producer  SyncProducer
    topic     string
    validator validator.Validator
}

func New(producer SyncProducer, topic string, v validator.Validator) Service {
    return &service{
        producer:  producer,
        topic:     topic,
        validator: v,
    }
}

func (s *service) Publish(ctx context.Context, events []*models.PaymentEvent, rate int) []Result {
    // ... implementação conforme design.md
}
```

### Funções auxiliares de geração

```go
// GenerateEvent cria um PaymentEvent a partir de parâmetros.
func GenerateEvent(paymentID, status string, amount float64, currency, description string) *models.PaymentEvent

// GenerateBulkEvents gera N eventos com dados sequenciais para bulk.
func GenerateBulkEvents(count int) []*models.PaymentEvent
```

### O que NÃO implementar aqui

- Parse de flags (fica no cmd)
- Leitura de stdin/arquivo (fica no cmd)
- Formatação de saída (fica no cmd)

### Testes

- `Publish` com sucesso (mock SyncProducer).
- `Publish` com erro de validação.
- `Publish` com erro do Kafka.
- `Publish` com rate limiting.
- `Publish` com contexto cancelado.
- `GenerateEvent` com todos os campos.
- `GenerateEvent` com campos vazios (autogeração).
- `GenerateBulkEvents` produz N eventos.
- `GenerateBulkEvents` eventos têm dados sequenciais.

---

## Passo 2 — Testes do Producer Service

**Arquivos**: `internal/producer/producer_test.go`

### Dependências de teste

- `github.com/stretchr/testify/assert`
- `github.com/stretchr/testify/require`
- Mock de `SyncProducer` (manual ou testify mock)

### Estratégia de mock

```go
type mockSyncProducer struct {
    sendMessageFn func(msg *sarama.ProducerMessage) (int32, int64, error)
}

func (m *mockSyncProducer) SendMessage(msg *sarama.ProducerMessage) (int32, int64, error) {
    return m.sendMessageFn(msg)
}
```

### Casos de teste

1. Publicação de evento único com sucesso.
2. Evento inválido (amount = 0) retorna Result com erro.
3. Kafka broker retorna erro.
4. Rate limit de 10 eventos/segundo: 5 eventos completam em ~500ms.
5. Contexto cancelado interrompe bulk.
6. Geração de evento com UUID auto.
7. Geração de evento com campos explícitos.
8. Bulk de 100 eventos retorna 100 resultados.

---

## Passo 3 — Entry Point (main)

**Arquivos**: `cmd/producer/main.go`

### Estrutura

```go
package main

import (
    "flag"
    "fmt"
    "io"
    "os"
    "os/signal"
    "strings"
    "syscall"
    "time"

    "exemplo.com/teste/internal/models"
    "exemplo.com/teste/internal/producer"
    "exemplo.com/teste/internal/validator"
    "github.com/IBM/sarama"
)

type flags struct {
    paymentID   string
    status      string
    amount      float64
    currency    string
    description string
    topic       string
    brokers     string
    payload     string
    file        string
    count       int
    rate        int
    dryRun      bool
    jsonOutput  bool
}

func parseFlags() flags { ... }
func getEvents(f flags) ([]*models.PaymentEvent, error) { ... }
func printResults(results []producer.Result, jsonOutput bool) { ... }
func printEvents(events []*models.PaymentEvent, jsonOutput bool) { ... }
```

### Fluxo principal

```go
func main() {
    f := parseFlags()

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle Ctrl+C
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-sigCh
        cancel()
    }()

    events, err := getEvents(f)
    if err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }

    v := validator.New()

    if f.dryRun {
        printEvents(events, f.jsonOutput)
        return
    }

    // Config sarama
    config := sarama.NewConfig()
    config.Producer.Return.Successes = true
    config.Producer.Timeout = 10 * time.Second

    syncProducer, err := sarama.NewSyncProducer(strings.Split(f.brokers, ","), config)
    if err != nil {
        fmt.Fprintf(os.Stderr, "error: cannot connect to Kafka: %v\n", err)
        os.Exit(1)
    }
    defer syncProducer.Close()

    svc := producer.New(syncProducer, f.topic, v)
    results := svc.Publish(ctx, events, f.rate)

    printResults(results, f.jsonOutput)

    // Exit code: 0 se todos sucesso, 1 se algum erro
    for _, r := range results {
        if r.Error != nil {
            os.Exit(1)
        }
    }
}
```

### Função `getEvents`

Ordem de precedência:
1. `--payload` → unmarshal como objeto único.
2. stdin pipe → unmarshal como array ou objeto.
3. `--file` → ler arquivo, unmarshal como array ou objeto.
4. `--count > 1` → `producer.GenerateBulkEvents(count)`.
5. Default → `producer.GenerateEvent(flags...)`.

---

## Passo 4 — Testes do Main

**Arquivos**: `cmd/producer/main_test.go`

### Estratégia

- Testar `parseFlags` com argumentos variados.
- Testar `getEvents` com payload, stdin, file, flags.
- Testar `printResults` capturando stdout/stderr.
- Testar código de saída em falha.

### Observação

- Testes de integração com Kafka real podem ser adicionados como
  `//go:build integration` para não depender de infra nos testes unitários.

---

## Passo 5 — Atualizar Makefile

Adicionar targets:

```makefile
build-producer:
	go build -o bin/producer ./cmd/producer

run-producer:
	go run ./cmd/producer --help
```
