# 02 — Requisitos

## Requisitos Funcionais

| ID | Descrição | Prioridade |
|----|-----------|------------|
| RF-001 | Adicionar `ProducerConfig` struct em `internal/config/config.go` com campo `Port int` | Alta |
| RF-002 | Adicionar campo `Producer ProducerConfig` ao struct `Config` | Alta |
| RF-003 | Adicionar `producer.port: 8082` no `config.yaml` | Alta |
| RF-004 | Em `cmd/producer/main.go`, `runServe()` deve usar `cfg.Producer.Port` em vez de `cfg.Server.Port` | Alta |
| RF-005 | Em `cmd/producer/main.go`, `apply()` deve setar `cfg.Producer.Port` em vez de `cfg.Server.Port` | Alta |
| RF-006 | Se `cfg.Producer.Port == 0` (config ausente), fallback para `8082` | Média |
| RF-007 | Flag `--port` continua existindo com default `""` (vazio) — só sobrescreve config se passada explicitamente | Média |
| RF-008 | No `docker-compose.yml`, remover `--port 8082` do `command` do serviço `producer` | Alta |
| RF-009 | Atualizar `docs/producer.md` para refletir a nova ordem de precedência | Média |

## Requisitos Não Funcionais

| ID | Descrição | Categoria |
|----|-----------|-----------|
| RNF-001 | A ordem de precedência deve ser: flag explícita > env var > config.yaml > fallback 8082 | Configuração |
| RNF-002 | Nenhuma alteração no comportamento do consumer (`cmd/consumer/main.go`) | Compatibilidade |
| RNF-003 | A variável de ambiente `PRODUCER_PORT` deve ser mapeada automaticamente pelo Viper | Configuração |
| RNF-004 | `producer serve` sem flags deve ler a porta do `config.yaml` | Usabilidade |
| RNF-005 | O servidor HTTP existente no docker-compose deve continuar na porta 8082 (via YAML) | Compatibilidade |

## Fora de Escopo

- ❌ Alterar o modo CLI (`publish`) — ele não carrega `config.yaml` e continuará usando flags apenas
- ❌ Renomear ou remover a flag `--port` — ela permanece como override explícito
- ✅ Adicionada validação de porta (range 1-65535) no `apply()` com warning log
- ❌ Alterar `ServerConfig` ou `server.port` — exclusivo do consumer
- ❌ Modificar o `ProducerConfig` em `internal/kafka/provider.go` — é específico do Sarama
