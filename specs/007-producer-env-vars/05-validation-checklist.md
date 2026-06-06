# 05 — Checklist de Validação

## Checklist de Aceitação

### Configuração e Ambiente

- [ ] **C01 — Config via config.yaml:** `producer serve` sem flags lê
      `config.yaml` e usa `kafka.brokers`, `kafka.topic` e `server.port`.
- [ ] **C02 — Config via env vars:** `KAFKA_BROKERS=kafka:9092 producer serve`
      sobrescreve o valor do config.yaml.
- [ ] **C03 — Config via flags:** `producer serve --port 9090 --brokers x:9092`
      sobrescreve config.yaml e env vars.
- [ ] **C04 — Ordem de precedência:** Flags > Env vars > config.yaml > defaults.
      Testar com os 3 níveis simultaneamente.
- [ ] **C05 — Flags com default vazio:** Se `--port` não for passado, o valor
      vindo de config.yaml ou env var é mantido.

### OpenTelemetry

- [ ] **C06 — TracerProvider inicializado:** Log mostra "tracer provider
      initialized" ou similar sem erros.
- [ ] **C07 — MeterProvider inicializado:** Log mostra "meter provider
      initialized" ou similar sem erros.
- [ ] **C08 — Shutdown do OTel:** Ao receber SIGTERM, `tp.Shutdown()` e
      `mp.Shutdown()` são chamados (verificar logs).
- [ ] **C09 — Service name:** Traces e métricas usam `cfg.OTel.ServiceName`
      (default: `payment-producer`).
- [ ] **C10 — OTel endpoint:** Traces são exportados para `cfg.OTel.Endpoint`
      (default: `localhost:4317`).

### Modo "serve" (HTTP)

- [ ] **C11 — Health check:** `GET /healthz` retorna `{"status":"ok"}`.
- [ ] **C12 — Publish endpoint:** `POST /publish` com JSON válido publica
      no Kafka e retorna partição/offset.
- [ ] **C13 — Publish bulk:** `POST /publish/bulk` com `{"count":5}`
      publica N eventos.
- [ ] **C14 — Graceful shutdown:** SIGTERM desliga o servidor HTTP sem
      erros, após finalizar requisições em andamento.
- [ ] **C15 — Porta correta:** Servidor HTTP inicia na porta especificada
      (default: 8082).

### Modo "publish" (CLI) — Regressão

- [ ] **C16 — Publish com --dry-run:** `producer publish --dry-run --count 2`
      exibe JSON sem publicar.
- [ ] **C17 — Publish com --payload:** `producer publish --payload '{...}'`
      publica o payload fornecido.
- [ ] **C18 — Publish com stdin:** `echo '{...}' | producer publish`
      funciona como antes.
- [ ] **C19 — Publish com --file:** `producer publish --file events.json`
      funciona como antes.
- [ ] **C20 — JSON output:** `--json-output` produz JSON por linha.
- [ ] **C21 — Exit codes:** Sucesso → 0, erro → 1.

### Integração

- [ ] **C22 — Build:** `go build ./cmd/producer/` compila sem erros.
- [ ] **C23 — Vet:** `go vet ./cmd/producer/` não reporta problemas.
- [ ] **C24 — Race detector:** `go run -race ./cmd/producer/ serve` inicia
      sem data races aparentes.
- [ ] **C25 — Docker:** `docker-compose up -d producer` inicia o container
      e o health check passa.

---

## Cenários de Teste Obrigatórios

### Cenário 1: Producer serve com config.yaml apenas

```bash
# Garantir que config.yaml tem kafka.brokers: localhost:9092
./producer serve
# Verificar logs:
#   "iniciando servidor HTTP do produtor"
#   "service": "payment-producer"
#   "port": "8082"
#   "brokers": "localhost:9092"
#   "topic": "payment.events"
```

### Cenário 2: Producer serve com env vars

```bash
KAFKA_BROKERS=kafka:9092 KAFKA_TOPIC=test.topic SERVER_PORT=9999 \
  OTEL_ENDPOINT=otel:4317 OTEL_SERVICE_NAME=payment-producer-test \
  ./producer serve
# Verificar logs com os valores corretos
```

### Cenário 3: Producer serve com flags (sobrescrevendo tudo)

```bash
./producer serve --port 7777 --brokers remote:9092 --topic custom.topic
# Verificar que porta, brokers e topic vieram das flags
```

### Cenário 4: Producer serve sem flags — fallback para config.yaml

```bash
# Remover env vars KAFKA_BROKERS, KAFKA_TOPIC, SERVER_PORT
unset KAFKA_BROKERS KAFKA_TOPIC SERVER_PORT
./producer serve
# Verificar que usou os valores do config.yaml
```

### Cenário 5: Publicação real via HTTP

```bash
# Producer rodando
curl -X POST http://localhost:8082/publish \
  -H "Content-Type: application/json" \
  -d '{"amount":100,"currency":"BRL"}'
# Verificar resposta: status 200, partition, offset
```

### Cenário 6: Graceful shutdown com OTel

```bash
# Producer rodando em background
./producer serve &
PID=$!
sleep 2
kill -TERM $PID
wait $PID
# Verificar logs: "shutting down HTTP server", "producer server stopped"
# Verificar que tp.Shutdown e mp.Shutdown foram chamados
```

### Cenário 7: Regressão do modo publish

```bash
# Testar todos os modos de publish
./producer publish --dry-run
./producer publish --dry-run --json-output
./producer publish --payment-id "abc123" --status pending --dry-run
echo '{"payment_id":"test","status":"confirmed","amount":50,"currency":"USD"}' | ./producer publish --dry-run
```

---

## Critérios de Hardening

- [ ] **H01 — Timeout no OTel:** `otlptracegrpc.WithTimeout(5s)` e
      `otlpmetricgrpc.WithTimeout(5s)` — já definidos em `pkg/telemetry`.
- [ ] **H02 — Timeout no shutdown OTel:** `tp.Shutdown()` e `mp.Shutdown()`
      com timeout de 15s (definido no código do producer).
- [ ] **H03 — Timeout no HTTP server:** ReadTimeout e WriteTimeout de 10s
      (já existentes, inalterados).
- [ ] **H04 — Graceful shutdown do HTTP:** `httpServer.Shutdown()` com
      timeout de 15s (já existente, inalterado).
- [ ] **H05 — Kafka retry:** `NewSyncProducerWithRetry` com retry na conexão
      (já existente, inalterado).
- [ ] **H06 — Contexto propagado:** Handlers HTTP usam `context.WithTimeout`
      (já existente, inalterado).
- [ ] **H07 — Logs estruturados:** JSON handler com campos de serviço.
- [ ] **H08 — Sem segredos em logs:** Config de OTel e Kafka não contêm
      segredos; endpoint OTel não é sensível.

---

## Histórico de Revisão

| Versão | Data       | Autor     | Descrição           |
|--------|------------|-----------|---------------------|
| 1.0    | 2026-06-06 | Architect | Versão inicial      |
