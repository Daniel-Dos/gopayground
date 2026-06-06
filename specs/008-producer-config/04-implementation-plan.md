# 04 — Plano de Implementação

## Tarefas

### Tarefa 1: Adicionar `ProducerConfig` ao `internal/config/config.go`

**Descrição:** Adicionar o struct `ProducerConfig` com campo `Port` e incluir
`Producer ProducerConfig` no struct `Config`.

**Arquivo:** `internal/config/config.go`

**Mudanças:**

```go
// ProducerConfig contém configurações do servidor HTTP do producer.
type ProducerConfig struct {
    Port int `mapstructure:"port"`
}

type Config struct {
    Server   ServerConfig   `mapstructure:"server"`
    Producer ProducerConfig `mapstructure:"producer"`  // NOVO
    Kafka    KafkaConfig    `mapstructure:"kafka"`
    // ... demais campos inalterados
}
```

**Verificação:** `go build ./internal/config/` compila sem erros.

**Esforço:** Baixo

**Dependências:** Nenhuma

---

### Tarefa 2: Adicionar `producer.port` no `config.yaml`

**Descrição:** Adicionar o bloco `producer.port: 8082` no arquivo de configuração YAML.

**Arquivo:** `config.yaml`

**Mudanças:** Adicionar antes ou depois do bloco `server`:

```yaml
producer:
  port: 8082
```

**Verificação:** `go run .` (ou teste que carregue config) mostra o novo bloco sem erros.

**Esforço:** Baixo

**Dependências:** Tarefa 1 (o struct precisa existir para o YAML ser parseado)

---

### Tarefa 3: Modificar `serveFlags.apply()` para usar `cfg.Producer.Port`

**Descrição:** No método `apply()` de `serveFlags`, alterar `cfg.Server.Port`
para `cfg.Producer.Port`.

**Arquivo:** `cmd/producer/main.go`

**Mudanças:**

```go
func (f *serveFlags) apply(cfg *config.Config) {
    if f.port != "" {
        port, err := strconv.Atoi(f.port)
        if err == nil {
            cfg.Producer.Port = port  // ANTES: cfg.Server.Port = port
        }
    }
    // ... brokers e topic inalterados
}
```

**Verificação:** `go build ./cmd/producer/` compila sem erros.

**Esforço:** Baixo

**Dependências:** Tarefa 1

---

### Tarefa 4: Modificar `runServe()` para usar `cfg.Producer.Port`

**Descrição:** Alterar a linha que extrai a porta para usar `cfg.Producer.Port`,
e adicionar fallback para `8082` se o valor for zero.

**Arquivo:** `cmd/producer/main.go`

**Mudanças na variável `port`:**

```go
// ANTES:
port := fmt.Sprintf("%d", cfg.Server.Port)

// DEPOIS:
port := fmt.Sprintf("%d", cfg.Producer.Port)
```

Adicionar bloco de fallback **após** `f.apply(&cfg)` e **antes** do logger:

```go
// Fallback para porta default se config não foi carregada
if cfg.Producer.Port == 0 {
    cfg.Producer.Port = 8082
}
```

**Verificação:** `go build ./cmd/producer/` compila sem erros.

**Esforço:** Baixo

**Dependências:** Tarefas 1 e 3

---

### Tarefa 5: Remover `--port 8082` do `command` no `docker-compose.yml`

**Descrição:** Alterar o `command` do serviço `producer` para não passar `--port`
explicitamente, já que a porta virá do `config.yaml`.

**Arquivo:** `docker-compose.yml`

**Mudança:**

```yaml
# ANTES:
command: ["serve", "--port", "8082", "--brokers", "kafka:9092"]

# DEPOIS:
command: ["serve", "--brokers", "kafka:9092"]
```

**Verificação:** `docker compose config` mostra o comando atualizado.

**Esforço:** Baixo

**Dependências:** Tarefas 1-4

---

### Tarefa 6: Atualizar `docs/producer.md`

**Descrição:** Atualizar a documentação do producer para refletir a nova ordem de
precedência, incluindo `PRODUCER_PORT` na tabela de variáveis de ambiente e
removendo warnings sobre `--port` não vir do YAML.

**Arquivo:** `docs/producer.md`

**Mudanças específicas:**

1. **Seção "Configuração → Modo Servidor HTTP"** (linhas ~256-263):
   - Substituir o warning sobre `server.port` por uma explicação clara da nova
     ordem de precedência
   - Adicionar `PRODUCER_PORT` e `producer.port` na tabela de variáveis

2. **Seção "Observações Técnicas → Portas"** (linhas ~353-359):
   - Atualizar se necessário

3. **Seção "Docker Compose"** (linhas ~316-334):
   - Remover menção de que `--port` é passado explicitamente
   - Atualizar o exemplo YAML para não incluir `--port`

**Verificação:** Revisão visual do arquivo.

**Esforço:** Baixo

**Dependências:** Tarefas 1-5

---

## Dependências entre tarefas

```
Tarefa 1 (config.go)
    ├── Tarefa 2 (config.yaml)
    ├── Tarefa 3 (apply())
    └── Tarefa 4 (runServe())
            └── Tarefa 5 (docker-compose.yml)
                    └── Tarefa 6 (docs)
```

## Estimativa de Esforço

| Tarefa | Esforço | Responsável |
|--------|---------|-------------|
| 1 — `ProducerConfig` struct | Baixo | Senior Engineer |
| 2 — `producer.port` no YAML | Baixo | Senior Engineer |
| 3 — `apply()` usa `Producer.Port` | Baixo | Senior Engineer |
| 4 — `runServe()` usa `Producer.Port` | Baixo | Senior Engineer |
| 5 — docker-compose.yml | Baixo | Senior Engineer |
| 6 — docs/producer.md | Baixo | Documentation Writer |

**Esforço total:** Baixo (~1-2 horas de desenvolvimento)
