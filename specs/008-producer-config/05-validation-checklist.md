# 05 — Checklist de Validação

## Checklist de Aceitação

### Funcional

- [ ] **RF-001**: `ProducerConfig` struct existe em `internal/config/config.go` com campo `Port int`
- [ ] **RF-002**: `Config.Producer` é do tipo `ProducerConfig` com tag `mapstructure:"producer"`
- [ ] **RF-003**: `config.yaml` contém `producer.port: 8082`
- [ ] **RF-004**: `runServe()` usa `cfg.Producer.Port` na formatação da porta
- [ ] **RF-005**: `apply()` seta `cfg.Producer.Port` em vez de `cfg.Server.Port`
- [ ] **RF-006**: Se `cfg.Producer.Port == 0`, fallback para 8082 é aplicado
- [ ] **RF-007**: Flag `--port` continua existindo e funcionando como override
- [ ] **RF-008**: `docker-compose.yml` não passa `--port 8082` no command
- [ ] **RF-009**: `docs/producer.md` atualizado com a nova configuração

### Não Funcional

- [ ] **RNF-001**: Flag `--port` sobrescreve `config.yaml`, que sobrescreve env var, que sobrescreve fallback
- [ ] **RNF-002**: `cmd/consumer/main.go` não foi alterado
- [ ] **RNF-003**: `PRODUCER_PORT=9090` é lido e aplicado corretamente
- [ ] **RNF-004**: `producer serve` sem flags usa porta 8082 do `config.yaml`
- [ ] **RNF-005**: Serviço `producer` no docker-compose continua acessível em `:8082`

## Cenários de Teste Obrigatórios

### Cenário 1: Config padrão (sem flags)
```bash
# Garantir que config.yaml tenha producer.port: 8082
producer serve
# → Servidor inicia em :8082
```

### Cenário 2: Flag --port explícita
```bash
producer serve --port 9999
# → Servidor inicia em :9999 (flag sobrescreve YAML)
```

### Cenário 3: Variável de ambiente PRODUCER_PORT
```bash
PRODUCER_PORT=9090 producer serve
# → Servidor inicia em :9090 (env var lida pelo Viper)
```

### Cenário 4: Env var + flag (flag vence)
```bash
PRODUCER_PORT=9090 producer serve --port 7070
# → Servidor inicia em :7070 (flag vence)
```

### Cenário 5: Fallback (cfg.Producer.Port == 0)
```bash
# Remover producer.port do config.yaml temporariamente
producer serve
# → Servidor inicia em :8082 (fallback hardcoded)
```

### Cenário 6: Consumer não afetado
```bash
consumer serve
# → Consumer continua em :8080 (server.port no config.yaml)
```

### Cenário 7: Docker compose
```bash
docker compose up -d producer
curl http://localhost:8082/healthz
# → Resposta: {"status": "ok"}
```

### Cenário 8: Compatibilidade retroativa de scripts
```bash
# Scripts que usam --port continuam funcionando
producer serve --port 8082 --brokers localhost:9092
# → Servidor inicia em :8082 com brokers configurados
```

## Critérios de Hardening

- [ ] Porta inválida (ex: `--port abc`) é tratada com erro (parse de int falha, flag ignorada)
- [ ] Porta 0 não causa panic — o fallback para 8082 é aplicado
- [ ] Porta negativa é tratada — `strconv.Atoi` retorna erro, flag é ignorada
- [ ] `apply()` não sobrescreve `cfg.Server.Port` acidentalmente (confirmação visual do diff)
- [ ] Nenhuma race condition na leitura/escrita de `cfg.Producer.Port` (single-threaded na inicialização)
