# 07 — Hardening

## Estratégia de Retry e Backoff

Esta feature não introduz novas operações de rede. O retry e backoff existentes
para conexão Kafka (`kafka.NewSyncProducerWithRetry`) permanecem inalterados.

## Timeout em Cada Operação

| Operação | Timeout | Observação |
|----------|---------|------------|
| Leitura de config.yaml | N/A (bloqueante na inicialização) | Se falhar, `log.Fatalf` — processo não inicia |
| Parse de flags `--port` | N/A (instantâneo) | Operação local, sem IO |
| Aplicação do fallback | N/A (instantâneo) | Operação local, sem IO |

Nenhum timeout novo é necessário — as operações são todas locais e síncronas.

## Proteção Contra Falha Parcial

A configuração da porta é carregada **antes** de qualquer operação de rede.
Se a porta for inválida (ex: valor não inteiro na flag), o erro é tratado
localmente e a flag é ignorada (não sobrescreve). O servidor HTTP só inicia
com uma porta válida.

**Cenários de falha parcial cobertos:**

| Cenário | Comportamento |
|---------|---------------|
| `--port abc` | `strconv.Atoi` falha → flag ignorada, valor do YAML/env prevalece |
| `producer.port` ausente no YAML | Viper retorna zero → fallback 8082 |
| `PRODUCER_PORT=abc` | Viper retorna zero (int não consegue parsear) → fallback 8082 |
| Múltiplas fontes com valores diferentes | Ordem de precedência resolve: flag > YAML > env > fallback |

## Observabilidade

### Logs

Sugestão de log a ser adicionado no fallback:

```go
if cfg.Producer.Port == 0 {
    cfg.Producer.Port = 8082
    slog.Warn("producer port not configured, using default",
        "port", cfg.Producer.Port)
}
```

Log existente que já mostra a porta em uso (em `runServe`):

```go
logger.Info("iniciando servidor HTTP do produtor",
    "service", cfg.OTel.ServiceName,
    "port", port,
    "brokers", cfg.Kafka.Brokers,
    "topic", topic,
)
```

Este log já é suficiente para confirmar qual porta o producer está usando.

### Métricas

Nenhuma métrica nova necessária — a porta não varia durante o ciclo de vida do processo.

### Tracing

Nenhum span novo necessário — a porta é uma configuração estática.

## Tratamento de Concorrência

`cfg.Producer.Port` é lido apenas na inicialização (`runServe`), em uma única
goroutine (a main). Não há concorrência na leitura ou escrita deste campo.

**Não há race condition** porque:
1. `config.NewConfig()` → escrita única
2. `f.apply(&cfg)` → escrita única (na mesma goroutine)
3. Leitura subsequente para formatar a porta → mesma goroutine

## Segurança Operacional

| Aspecto | Status |
|---------|--------|
| Porta baixa (< 1024) | O sistema operacional bloqueia se não for root; o Go retorna `permission denied` — comportamento padrão seguro |
| Bind para 0.0.0.0 (todas interfaces) | Comportamento padrão do `http.Server`; desejável em container Docker |
| Porta conflitante | Se a porta já estiver em uso, `ListenAndServe` retorna `EADDRINUSE` — erro tratado |

## Checklist de Hardening

- [ ] Porta inválida na flag (`--port abc`) é ignorada, não causa crash
- [ ] Porta zero no YAML aciona fallback seguro (8082)
- [ ] Fallback é logado como warning para visibilidade operacional
- [ ] Nenhuma race condition na leitura/escrita de `cfg.Producer.Port`
- [ ] O docker-compose.yml não expõe porta conflitante (8082 permanece)
- [ ] Health check no docker-compose continua funcional (`wget http://localhost:8082/healthz`)
