# 04 — Plano de Implementação

## Tarefas

### Tarefa 1: Alterar `newServeFlagSet` para usar defaults vazios

**Descrição:** Modificar a função `newServeFlagSet` para que as flags
`--port`, `--brokers` e `--topic` tenham default `""` em vez de valores
hardcoded. Isso permite que o algoritmo de mesclagem distinga "flag não
fornecida" de "flag fornecida com valor".

**Arquivo:** `cmd/producer/main.go`

**Mudança:**
```go
// Antes:
fs.StringVar(&f.port, "port", "8082", "HTTP server port")
fs.StringVar(&f.brokers, "brokers", "localhost:9092", "Brokers Kafka separados por virgula")
fs.StringVar(&f.topic, "topic", "payment.events", "Topico Kafka")

// Depois:
fs.StringVar(&f.port, "port", "", "HTTP server port (default: config/env)")
fs.StringVar(&f.brokers, "brokers", "", "Brokers Kafka separados por virgula (default: config/env)")
fs.StringVar(&f.topic, "topic", "", "Topico Kafka (default: config/env)")
```

**Esforço:** Baixo

---

### Tarefa 2: Adicionar método `apply(cfg *config.Config)` ao `serveFlags`

**Descrição:** Adicionar um método que aplica as flags sobre a config
carregada. Este método só sobrescreve campos se a flag tiver sido
explicitamente fornecida (diferente de `""`).

**Arquivo:** `cmd/producer/main.go`

**Código a adicionar:**
```go
// apply aplica as flags sobre a configuração carregada.
// Flags com valor vazio são ignoradas (não fornecidas).
func (f *serveFlags) apply(cfg *config.Config) {
    if f.port != "" {
        port, err := strconv.Atoi(f.port)
        if err == nil {
            cfg.Server.Port = port
        }
    }
    if f.brokers != "" {
        cfg.Kafka.Brokers = f.brokers
    }
    if f.topic != "" {
        cfg.Kafka.Topic = f.topic
    }
}
```

> **Nota:** O `config.ServerConfig.Port` é `int`. Será convertido de string.

**Esforço:** Baixo

---

### Tarefa 3: Modificar os imports no `cmd/producer/main.go`

**Descrição:** Adicionar os imports necessários para config, telemetry,
otel, strconv e context.

**Arquivo:** `cmd/producer/main.go`

**Imports a adicionar:**
```go
import (
    "context"
    "strconv"
    "time"

    "github.com/Daniel-Dos/gopayground/internal/config"
    "github.com/Daniel-Dos/gopayground/pkg/telemetry"

    "go.opentelemetry.io/otel"
)
```

> `context` já existe no import. Apenas adicionar `strconv`, `time`
> (se não existir), `config`, `telemetry` e `otel`.

**Esforço:** Baixo

---

### Tarefa 4: Reescrever a função `runServe` com config + telemetry

**Descrição:** Modificar `runServe` para seguir o novo fluxo:

1. Carregar config via `config.NewConfig()`
2. Parsear flags e aplicar sobre a config
3. Inicializar logger
4. Inicializar OpenTelemetry (TracerProvider + MeterProvider)
5. Conectar ao Kafka usando valores da config
6. Iniciar HTTP server na porta da config
7. Shutdown gracioso com desligamento do OTel

**Arquivo:** `cmd/producer/main.go`

**Estrutura da nova `runServe`:**
```go
func runServe(args []string) int {
    // 1. Carregar configuração (config.yaml + env vars)
    cfg := config.NewConfig()

    // 2. Parsear flags e aplicar sobre config
    f, err := parseServeFlagsArgs(args)
    if err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        return 1
    }
    f.apply(&cfg)

    // 3. Logger estruturado
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
    slog.SetDefault(logger)

    // 4. Converter brokers string → []string
    brokers := strings.Split(cfg.Kafka.Brokers, ",")
    topic := cfg.Kafka.Topic
    port := fmt.Sprintf("%d", cfg.Server.Port)

    logger.Info("iniciando servidor HTTP do produtor",
        "service", cfg.OTel.ServiceName,
        "port", port,
        "brokers", cfg.Kafka.Brokers,
        "topic", topic,
    )

    // 5. Contexto raiz para shutdown
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // 6. Capturar sinais
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        sig := <-sigCh
        logger.Info("sinal de desligamento recebido", "signal", sig.String())
        cancel()
    }()

    // 7. Inicializar OpenTelemetry
    tp, err := telemetry.InitTracerProvider(ctx, cfg)
    if err != nil {
        logger.Error("falha ao inicializar tracer provider", "error", err)
        return 1
    }
    defer func() {
        shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer shutdownCancel()
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
        shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer shutdownCancel()
        if err := mp.Shutdown(shutdownCtx); err != nil {
            logger.Error("erro ao desligar meter provider", "error", err)
        }
    }()

    tracer := otel.Tracer(cfg.OTel.ServiceName)
    meter := otel.Meter(cfg.OTel.ServiceName)
    _ = tracer  // reservado para uso futuro em handlers
    _ = meter   // reservado para uso futuro em métricas

    // 8. Conectar ao Kafka com retry
    saramaCfg := kafka.NewProducerSaramaConfig(kafka.DefaultProducerConfig())
    syncProducer, err := kafka.NewSyncProducerWithRetry(ctx, brokers, saramaCfg)
    if err != nil {
        logger.Error("cannot connect to Kafka", "error", err)
        return 1
    }
    defer func() {
        if err := syncProducer.Close(); err != nil {
            logger.Error("kafka producer close error", "error", err)
        }
    }()

    v := validator.New()
    svc := producer.New(syncProducer, topic, v)

    // 9. Publicar eventos de startup (best-effort)
    go func() {
        startupCtx, startupCancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer startupCancel()

        events := producer.GenerateBulkEvents(10)
        results := svc.Publish(startupCtx, events, 0)
        published := 0
        for _, r := range results {
            if r.Error == nil {
                published++
            }
        }
        logger.Info("publicação inicial concluída", "total", len(events), "published", published)
    }()

    // 10. Configurar servidor HTTP
    mux := http.NewServeMux()
    mux.HandleFunc("POST /publish", handlePublish(svc, logger))
    mux.HandleFunc("POST /publish/bulk", handlePublishBulk(svc, logger))
    mux.HandleFunc("GET /healthz", handleHealthz(logger))

    addr := ":" + port
    httpServer := &http.Server{
        Addr:         addr,
        Handler:      mux,
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 10 * time.Second,
    }

    // 11. Goroutine de shutdown gracioso
    go func() {
        <-ctx.Done()
        logger.Info("shutting down HTTP server")
        shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer shutdownCancel()
        if err := httpServer.Shutdown(shutdownCtx); err != nil {
            logger.Error("HTTP server shutdown error", "error", err)
        }
    }()

    logger.Info("producer server ready", "addr", addr)
    if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        logger.Error("HTTP server error", "error", err)
        return 1
    }

    logger.Info("producer server stopped")
    return 0
}
```

**Esforço:** Alto

---

### Tarefa 5: Verificar que `runPublish` permanece inalterado

**Descrição:** Confirmar que a função `runPublish` não foi alterada.
Ela continua usando `parsePublishFlags`, sem importar config ou telemetry.

**Arquivo:** `cmd/producer/main.go`

**Verificação:**
- A função `runPublish` não chama `config.NewConfig()`
- A função `runPublish` não chama `telemetry.InitTracerProvider` ou `InitMeterProvider`
- Os imports adicionados na Tarefa 3 não são usados por `runPublish`
- Flags de publish continuam com seus defaults originais

**Esforço:** Baixo (verificação)

---

### Tarefa 6: Testar que o build compila e o binário funciona

**Descrição:** Verificar que o projeto compila sem erros, que o modo
"serve" inicia com config/env/flags e que o modo "publish" continua
funcionando.

**Comandos:**
```bash
go build ./cmd/producer/
go vet ./cmd/producer/
```

**Testes manuais:**
```bash
# Serve com config.yaml (default local)
./producer serve

# Serve com flag (deve sobrescrever)
./producer serve --port 9090

# Serve com env var (deve sobrescrever config.yaml)
KAFKA_BROKERS=broker:9092 ./producer serve

# Publish (deve funcionar como antes)
./producer publish --dry-run

# Publish com flags (deve funcionar como antes)
./producer publish --payment-id "abc" --status confirmed --dry-run
```

**Esforço:** Médio

---

### Tarefa 7: Verificar compatibilidade com docker-compose

**Descrição:** Subir o ambiente com docker-compose e verificar que
o producer inicia corretamente usando as env vars definidas.

**Comando:**
```bash
docker-compose up -d producer
docker-compose logs producer
```

**Verificações:**
- Logs mostram `service: payment-producer`
- Logs mostram `brokers: kafka:9092`
- Logs mostram `topic: payment.events`
- Health check responde em `:8082/healthz`
- Eventos publicados via HTTP POST são entregues ao Kafka
- OpenTelemetry está configurado (tracer provider inicializado)

**Esforço:** Médio

---

## Dependências entre Tarefas

```
Tarefa 1 (newServeFlagSet)
    ↓
Tarefa 2 (apply method)
    ↓
Tarefa 3 (imports)
    ↓
Tarefa 4 (runServe rewrite) — depende de 1, 2, 3
    ↓
Tarefa 5 (verificar publish) — pode ser paralela com 4
    ↓
Tarefa 6 (build + test local) — depende de 4
    ↓
Tarefa 7 (docker-compose) — depende de 6
```

## Estimativa de Esforço

| Tarefa | Descrição                           | Esforço  |
|--------|-------------------------------------|----------|
| 1      | Alterar defaults das flags serve    | Baixo    |
| 2      | Adicionar método apply()            | Baixo    |
| 3      | Adicionar imports                   | Baixo    |
| 4      | Reescrever runServe                 | Alto     |
| 5      | Verificar publish inalterado        | Baixo    |
| 6      | Build e testes locais               | Médio    |
| 7      | Verificação docker-compose          | Médio    |

**Total estimado:** ~4-6 horas de desenvolvimento

---

## Entregáveis

1. Arquivo `cmd/producer/main.go` modificado (apenas runServe e funções auxiliares)
2. Build bem-sucedido (`go build ./cmd/producer/`)
3. Testes manuais de regressão no modo publish
4. Testes manuais do modo serve com config/env/flags
5. Verificação docker-compose funcional

## Riscos na Implementação

- **Risco de regressão:** Mudar os defaults das flags de `"8082"` para `""`
  pode quebrar scripts que dependem do comportamento anterior. **Mitigação:**
  O config.yaml em ambientes locais fornece os mesmos defaults.
- **Risco de porta incorreta:** Sem flag `--port` e sem env `SERVER_PORT`,
  o config lê `server.port: 8080` do config.yaml (default do consumer).
  **Mitigação:** Garantir que o docker-compose defina `SERVER_PORT=8082`
  ou manter `--port 8082` no `command` do compose.
