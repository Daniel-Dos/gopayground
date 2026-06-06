# 01 — Contexto

## Problema

Atualmente, a porta do servidor HTTP do **producer** está hardcoded no código-fonte
em `cmd/producer/main.go` como default da flag `--port "8082"`:

```go
fs.StringVar(&f.port, "port", "8082", "HTTP server port (default: 8082)")
```

Isso causa dois problemas:

1. **Porta do producer não configurável via `config.yaml`** — O arquivo `config.yaml`
   define apenas `server.port: 8080`, que é a porta do **consumer**. O producer
   ignora completamente o YAML para sua porta.

2. **Env var `SERVER_PORT` não funciona para o producer** — A flag `--port` tem
   default `"8082"`, e no método `apply()` o valor da flag (mesmo o default) sempre
   sobrescreve `cfg.Server.Port`. Como Viper mapeia `SERVER_PORT` → `server.port`,
   a env var nunca tem efeito porque a flag default tem precedência maior.

Além disso, o `docker-compose.yml` explicitamente passa `--port 8082` no `command`
do serviço `producer`, o que é redundante e esconde o fato de que a porta poderia
vir da configuração.

## Motivação de Negócio

- **Padronização 12-Factor**: Todo serviço deve ter sua porta configurável via
  `config.yaml` + env vars, seguindo o mesmo padrão já adotado pelo consumer.
- **Operabilidade**: Operadores esperam que variáveis de ambiente como `PRODUCER_PORT`
  funcionem sem precisar saber de flags específicas.
- **Manutenibilidade**: Eliminar hardcoding reduz surpresas e facilita mudanças de
  porta em diferentes ambientes (dev, staging, produção).

## Sistemas Envolvidos

| Sistema | Papel |
|---------|-------|
| `cmd/producer/main.go` | Entrada do producer — será modificado para usar `cfg.Producer.Port` |
| `internal/config/config.go` | Definição de tipos de configuração — receberá `ProducerConfig` |
| `config.yaml` | Arquivo de configuração YAML — receberá `producer.port: 8082` |
| `docker-compose.yml` | Orquestração Docker — remover `--port 8082` do command |
| `docs/producer.md` | Documentação — atualizar seção de configuração |

## Restrições Conhecidas

- Compatibilidade retroativa: `producer serve --port 9090` deve continuar funcionando
- O modo CLI (`publish`) não carrega `config.yaml` e não é afetado
- O consumer (`cmd/consumer/main.go`) não é afetado — continua usando `server.port`
- O campo `ProducerConfig` em `internal/kafka/provider.go` (config Sarama) **não** deve
  ser confundido com o novo `ProducerConfig` em `internal/config/config.go`
