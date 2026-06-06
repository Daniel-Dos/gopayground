# 03 — Design

## Arquitetura Geral

### Diagrama de Fluxo (modo "serve" com config + telemetry)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          cmd/producer/main.go                                │
│                                                                             │
│  runServe(args):                                                             │
│                                                                             │
│  1. Carregar config:                                                         │
│     cfg := config.NewConfig()                                               │
│       ├── config.yaml (defaults)                                            │
│       ├── env vars (12-factor override)                                     │
│       └── (flags aplicadas DEPOIS para sobrescrever)                        │
│                                                                             │
│  2. Inicializar logger estruturado                                          │
│     slog.New(slog.NewJSONHandler(...))                                       │
│                                                                             │
│  3. Inicializar OpenTelemetry:                                              │
│     tp, err := telemetry.InitTracerProvider(ctx, cfg)                       │
│     mp, err := telemetry.InitMeterProvider(ctx, cfg)                        │
│     defer tp.Shutdown(...)                                                  │
│     defer mp.Shutdown(...)                                                  │
│     tracer := otel.Tracer(cfg.OTel.ServiceName)                             │
│     meter := otel.Meter(cfg.OTel.ServiceName)                               │
│                                                                             │
│  4. Aplicar flags SOBRE o cfg (override):                                   │
│     if flag.port != ""      → cfg.Server.Port = flag.port                   │
│     if flag.brokers != ""   → cfg.Kafka.Brokers = flag.brokers              │
│     if flag.topic != ""     → cfg.Kafka.Topic = flag.topic                  │
│                                                                             │
│  5. Conectar Kafka com cfg.Kafka.Brokers e cfg.Kafka.Topic:                │
│     syncProducer, err := kafka.NewSyncProducerWithRetry(..., brokers)       │
│     svc := producer.New(syncProducer, cfg.Kafka.Topic, v)                   │
│                                                                             │
│  6. Iniciar HTTP server em cfg.Server.Port:                                 │
│     http.ListenAndServe(":" + port, mux)                                    │
│                                                                             │
│  7. Shutdown: sinal → cancel() → tp.Shutdown() → mp.Shutdown() → server sai │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Diagrama de Componentes

```
┌──────────────────────────────────────────────────────────┐
│                   cmd/producer/main.go                    │
│                                                          │
│  ┌──────────────────────────────────────────────────────┐│
│  │  run()                                                ││
│  │  ├── sub == "serve"  → runServe(args)                 ││
│  │  └── sub == "publish" → runPublish(args) (inalterado) ││
│  └──────────────────────────────────────────────────────┘│
│                                                          │
│  ┌──────────────────────────────────────────────────────┐│
│  │  runServe(args)                                       ││
│  │                                                      ││
│  │  1. cfg = config.NewConfig()                          ││
│  │  2. Aplicar flags → cfg override                      ││
│  │  3. Init logger                                       ││
│  │  4. tp = telemetry.InitTracerProvider(ctx, cfg)       ││
│  │  5. mp = telemetry.InitMeterProvider(ctx, cfg)        ││
│  │  6. tp.Shutdown (defer)                               ││
│  │  7. mp.Shutdown (defer)                               ││
│  │  8. kafka.NewSyncProducerWithRetry(...)               ││
│  │  9. producer.New(syncProducer, topic, v)              ││
│  │  10. Iniciar HTTP server                              ││
│  │  11. Graceful shutdown                                ││
│  └──────────────────────────────────────────────────────┘│
└──────────────────────────────────────────────────────────┘

         │                           ▲
         ▼                           │
┌──────────────────┐       ┌──────────────────┐
│ internal/config   │       │ pkg/telemetry     │
│                  │       │                  │
│ Viper            │       │ InitTracerProvider│
│ config.yaml      │──────►│ InitMeterProvider │
│ env vars         │       │ otel.SetTracerProv│
│ Config struct    │       │ otel.SetMeterProv │
└──────────────────┘       └──────────────────┘
```

## Integração com `internal/config`

O pacote `internal/config` já fornece `config.NewConfig()` que:

1. Lê `config.yaml` (obrigatório) — contém defaults de toda a aplicação.
2. Chama `viper.AutomaticEnv()` — mapeia env vars para keys Viper,
   substituindo `.` por `_`. Ex: `kafka.brokers` → env var `KAFKA_BROKERS`.
3. Retorna um `config.Config` com todos os campos populados.

**O producer usará a mesma instância**, porém alguns campos do Config
não são relevantes para ele (Redis, DynamoDB, Worker, Retry, UI). Isso
não é um problema — eles simplesmente terão valores default/vazios.

### Mapeamento Viper (já existente)

```go
viper.AutomaticEnv()
viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
```

Isso significa que:
- `config.yaml` com `kafka.brokers: "localhost:9092"` é lido
- Env var `KAFKA_BROKERS` sobrescreve o valor do YAML
- O struct `Config.Kafka.Brokers` recebe o valor final

### Configurações relevantes para o producer

| Campo Config               | Lido de                       | Usado para              |
|----------------------------|--------------------------------|-------------------------|
| `cfg.Kafka.Brokers`        | `KAFKA_BROKERS` ou config.yaml | Conectar ao Kafka       |
| `cfg.Kafka.Topic`          | `KAFKA_TOPIC` ou config.yaml   | Tópico de publicação    |
| `cfg.Server.Port`          | `SERVER_PORT` ou config.yaml   | Porta do HTTP server    |
| `cfg.OTel.Endpoint`        | `OTEL_ENDPOINT` ou config.yaml | Endpoint OTLP gRPC      |
| `cfg.OTel.ServiceName`     | `OTEL_SERVICE_NAME` ou cfg     | Nome do serviço (traces)|

### Importante: Producer usa porta 8082, config.yaml tem server.port: 8080

O `config.yaml` define `server.port: 8080` (porta do consumer). O producer
deve usar porta 8082. Isso será resolvido por:

1. **Env var** no `docker-compose.yml`: `SERVER_PORT=8082` (a ser adicionada
   se necessário, ou mantendo o default do código).
2. **Default de código**: Se nem config.yaml nem env var nem flag definirem,
   o producer assume 8082 como porta padrão. Isso será feito via flag default
   (`--port 8082`) que, por ter maior precedência, resolve naturalmente.

**Estratégia**: O `serveFlags` mantém `port` com default `"8082"`. Após
carregar `config.NewConfig()`, se a flag `--port` foi fornecida, ela
sobrescreve `cfg.Server.Port`. Se não foi fornecida, o valor do
`serveFlags.port` (8082) é usado. Isso garante que o producer sempre
use 8082 como porta final.

### Tratamento de Conflito: Porta (Consumer 8080 vs Producer 8082)

```
config.yaml:         server.port = 8080  (default do consumer)
Env var (compose):   SERVER_PORT = 8082  (não definida atualmente)
Flag producer:       --port 8082         (default hardcoded no código)

Precedência: flag (8082) > env var (??) > config.yaml (8080) > code default
Resultado final: producer usa 8082 (via flag default)
```

O producer não precisa de `SERVER_PORT=8082` no compose porque a flag
`--port 8082` já garante o valor correto. Se no futuro o `command` do
compose for simplificado para apenas `["serve"]`, aí sim a env var
`SERVER_PORT=8082` deverá ser adicionada.

## Integração com `pkg/telemetry`

O pacote `pkg/telemetry` expõe:

```go
func InitTracerProvider(ctx context.Context, cfg config.Config) (*sdktrace.TracerProvider, error)
func InitMeterProvider(ctx context.Context, cfg config.Config) (*sdkmetric.MeterProvider, error)
```

Ambos usam `cfg.OTel.Endpoint` e `cfg.OTel.ServiceName`. O producer
chamará essas funções **após** carregar a config e aplicar flags.

### Fluxo de inicialização do OTel no producer

```go
// Carregar config
cfg := config.NewConfig()

// Aplicar flags SOBRE cfg
f, _ := parseServeFlagsArgs(args)
if f.port != "" {
    cfg.Server.Port, _ = strconv.Atoi(f.port) // ou manter como string
}
if f.brokers != "" {
    cfg.Kafka.Brokers = f.brokers
}
if f.topic != "" {
    cfg.Kafka.Topic = f.topic
}

// Inicializar OTel
ctx := context.Background()
tp, err := telemetry.InitTracerProvider(ctx, cfg)
if err != nil {
    logger.Error("falha ao inicializar tracer provider", "error", err)
    return 1
}
defer func() {
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()
    if err := tp.Shutdown(shutdownCtx); err != nil {
        logger.Error("erro ao desligar tracer provider", "error", err)
    }
}()

mp, err := telemetry.InitMeterProvider(ctx, cfg)
if err != nil {
    logger.Error("falha ao inicializar meter provider", "error", err)
    return 1
}
defer func() {
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()
    if err := mp.Shutdown(shutdownCtx); err != nil {
        logger.Error("erro ao desligar meter provider", "error", err)
    }
}()

tracer := otel.Tracer(cfg.OTel.ServiceName)
meter := otel.Meter(cfg.OTel.ServiceName)
```

### Logs com identificação de serviço

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
slog.SetDefault(logger)

logger.Info("iniciando servidor HTTP do produtor",
    "service", cfg.OTel.ServiceName,
    "port", cfg.Server.Port,
    "brokers", cfg.Kafka.Brokers,
    "topic", cfg.Kafka.Topic,
)
```

## Flags vs Config: Algoritmo de Mesclagem

```
function mergeConfigWithFlags(cfg Config, flags serveFlags) Config:
    if flags.port != "":
        cfg.Server.Port = parseInt(flags.port)  // ou manter string
    if flags.brokers != "":
        cfg.Kafka.Brokers = flags.brokers
    if flags.topic != "":
        cfg.Kafka.Topic = flags.topic
    return cfg
```

> Nota: A flag `--brokers` no producer atual usa default `"localhost:9092"`.
> Com a mesclagem, se a flag for omitida, o valor vindo do
> `serveFlags.brokers` será `"localhost:9092"` (default da flag), e esse
> valor sobrescreverá o config. Para evitar esse problema, o default das
> flags **deve ser vazio** (`""`) em vez do valor real, para que a
> mesclagem distinga "flag não fornecida" de "flag fornecida com valor X".

### Novo comportamento proposto para `newServeFlagSet`

```go
func newServeFlagSet(f *serveFlags) *flag.FlagSet {
    fs := flag.NewFlagSet("serve", flag.ContinueOnError)

    // Default vazio para permitir detecção de "não fornecido"
    fs.StringVar(&f.port, "port", "", "HTTP server port (default: 8082)")
    fs.StringVar(&f.brokers, "brokers", "", "Brokers Kafka separados por virgula (default: env/config)")
    fs.StringVar(&f.topic, "topic", "", "Topico Kafka (default: env/config)")

    fs.Usage = func() {
        fmt.Fprintf(os.Stderr, "Uso: producer serve [flags]\n\nFlags:\n")
        fs.PrintDefaults()
    }

    return fs
}
```

Dessa forma:
- Se `--port 8083` é passado → `f.port = "8083"` → `cfg.Server.Port = 8083`
- Se `--port` não é passado → `f.port = ""` → mantém o valor do config/env
- O default real é definido primeiro pelo config.yaml, depois env var,
  depois flag default (8082 via `serveFlags`)

**Mas isso muda o comportamento atual** se alguém chamar `producer serve`
sem flags e sem config.yaml — o brokers será vazio. Para manter
compatibilidade, podemos usar um valor sentinela (ex: `"__default__"`)
no lugar de `""`, mas isso adiciona complexidade desnecessária.

**Decisão:** Manter os defaults atuais nas flags (`"8082"`, `"localhost:9092"`,
`"payment.events"`). A "mesclagem" será feita de forma que a flag SEMPRE
tenha a palavra final, mas como o default da flag é igual ao do config.yaml/env,
o resultado prático é idêntico. A única diferença é que a flag sempre vence,
o que significa que se o config.yaml tiver um valor diferente, a flag default
o sobrescreve. **Isso é aceitável** porque o config.yaml é fonte de defaults
e as env vars forçam override via Viper (não via flag).

Na prática:
1. `config.yaml` define `kafka.brokers: "localhost:9092"`
2. Env var `KAFKA_BROKERS=kafka:9092` sobrescreve no Viper
3. Flag `--brokers` com default `"localhost:9092"` não muda nada
4. Se o usuário explicitamente passar `--brokers kafka:9092`, a flag vence
5. Se o usuário não passar `--brokers`, o valor default `"localhost:9092"`
   da flag sobrescreveria a env var. **Problema!**

**Solução definitiva:** Para evitar o problema acima, os defaults das flags
no modo "serve" devem ser `""` e o default real deve vir da config:

```go
fs.StringVar(&f.port, "port", "8082", "HTTP server port")
fs.StringVar(&f.brokers, "brokers", "localhost:9092", "Brokers Kafka separados por virgula")
fs.StringVar(&f.topic, "topic", "payment.events", "Topico Kafka")
```

E a mesclagem deve considerar: se a flag foi **explicitamente** fornecida
(diferente do default), usar flag. Caso contrário, usar config.

```go
type serveFlags struct {
    port         string
    brokers      string
    topic        string
    flagSet      *flag.FlagSet  // para consultar se foi alterada
}
```

No Go puro, detectar se uma flag foi alterada é feito através de
`flag.FlagSet.Lookup("port").Value.String() != flagSet.Lookup("port").DefValue`.

**Estratégia final recomendada:** Mudar os defaults das flags para `""`
e usar o config + env como fonte primária. O `serveFlags` ganha seu próprio
método `apply(cfg *config.Config)` que verifica se cada flag foi setada:

```go
func (f *serveFlags) apply(cfg *config.Config) {
    if f.port != "" {
        port, _ := strconv.Atoi(f.port)
        cfg.Server.Port = port
    }
    if f.brokers != "" {
        cfg.Kafka.Brokers = f.brokers
    }
    if f.topic != "" {
        cfg.Kafka.Topic = f.topic
    }
}
```

Isso funciona porque:
- Se `--port 8083` → `f.port = "8083"` → aplica
- Se nenhuma flag → `f.port = ""` → não aplica, mantém config/env
- O default de `config.yaml` + env var garantem que haja sempre um valor

Para compatibilidade com scripts que chamam `producer serve` sem flags,
o `config.yaml` e as env vars no `docker-compose.yml` já fornecem os
valores corretos. Em ambiente local (fora do Docker), o `config.yaml`
tem `kafka.brokers: "localhost:9092"`, o que é suficiente.

## Modo "publish" — Inalterado

O `runPublish()` continua usando apenas `flag` stdlib, sem tocar em
`internal/config` ou `pkg/telemetry`. Nenhuma alteração.

```go
func runPublish(args []string) int {
    f, err := parsePublishFlags(args)
    // ... exatamente como antes ...
    // Sem config.NewConfig(), sem OTel
}
```

## Tratamento de Shutdown

O desligamento gracioso deve seguir esta ordem:

```
1. SIGINT/SIGTERM recebido
2. cancel() → context cancelado
3. HTTP server.Shutdown() é chamado (goroutine)
4. HTTP server para de aceitar conexões
5. (após server parar) defer roda:
   a. syncProducer.Close()
   b. tp.Shutdown(shutdownCtx)  — desliga OTel tracing
   c. mp.Shutdown(shutdownCtx)  — desliga OTel metrics
6. main retorna
```

**Importante:** O `tp.Shutdown()` e `mp.Shutdown()` devem ser chamados
**após** o HTTP server parar, não antes. Como estão em `defer`, e o
`httpServer.Shutdown()` é chamado na goroutine de shutdown, a ordem
correta é garantida: a goroutine sinaliza o server para parar, e quando
`ListenAndServe` retorna, os defers disparam.

## Contratos de Interface (inalterados)

```go
// Producer service (já existente em internal/producer)
type Service interface {
    Publish(ctx context.Context, events []*models.PaymentEvent, rate int) []Result
}

// Validador (já existente em internal/validator)
type Validator interface {
    Validate(ctx context.Context, data []byte) (*models.PaymentEvent, error)
}

// SyncProducer (já existente em internal/kafka)
func NewSyncProducerWithRetry(ctx context.Context, brokers []string, config *sarama.Config) (sarama.SyncProducer, error)
```

## Estrutura Final de `cmd/producer/main.go`

Apenas a função `runServe` será modificada. O restante do arquivo
(public flags, publish logic, handlers, helpers) permanece idêntico.

```
cmd/producer/main.go
├── imports        → adicionar: config, telemetry, otel, strconv, time
├── flags struct   → inalterado (publishFlags, serveFlags)
├── flag funcs     → newServeFlagSet: defaults vazios (""); parseServeFlagsArgs: igual
├── main/run       → inalterado
├── runPublish     → inalterado
├── runServe       → MODIFICADO
│                    ├── config.NewConfig()
│                    ├── flags.apply(&cfg)
│                    ├── logger setup
│                    ├── telemetry.InitTracerProvider()
│                    ├── telemetry.InitMeterProvider()
│                    ├── Kafka connection (usa cfg.Kafka.Brokers, cfg.Kafka.Topic)
│                    ├── HTTP server (usa cfg.Server.Port)
│                    └── graceful shutdown
├── handlers       → inalterados
├── helpers        → inalterados
└── writeJSON      → inalterado
```
