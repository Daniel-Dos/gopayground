# 07 — Hardening

## 1. Resiliência

### 1.1 Timeouts Explícitos

| Operação                 | Timeout | Implementação                                    |
|--------------------------|---------|--------------------------------------------------|
| Kafka SyncProducer       | 10s     | `sarama.Config.Producer.Timeout = 10 * time.Second` |
| Conexão TCP com Kafka    | 5s      | `sarama.Config.Net.DialTimeout = 5 * time.Second`   |
| Validação de payload     | 5s      | `context.WithTimeout(ctx, 5*time.Second)`            |
| Leitura de arquivo       | N/A     | `os.ReadFile` (sem timeout — aceito para dev tool)   |

**Configuração do sarama**:

```go
config := sarama.NewConfig()
config.Producer.Return.Successes = true
config.Producer.Timeout = 10 * time.Second
config.Net.DialTimeout = 5 * time.Second
config.Producer.MaxMessageBytes = 100 * 1024 // 100KB max message size
```

### 1.2 Retry de Publicação

**Decisão**: **Sem retry automático na CLI**. Se a publicação falha, o erro
é reportado ao usuário imediatamente.

**Justificativa**: Ferramenta interativa de desenvolvimento. O usuário vê o
erro e decide se quer tentar novamente. Retry automático em CLI só
confundiria (usuário pode achar que publicou quando não publicou).

Exceção: em bulk mode, se um evento falha, os demais continuam sendo
publicados (erro não interrompe o lote).

### 1.3 Graceful Shutdown

**Sequência**:

```
1. Usuário pressiona Ctrl+C (SIGINT/SIGTERM)
2. Context é cancelado
3. Producer.Publish() recebe ctx.Done()
4. Publicação em andamento é abortada (se em bulk)
5. Mensagens já publicadas foram confirmadas (SyncProducer síncrono)
6. defer syncProducer.Close() é executado
7. Processo exit(1) se estava no meio de um bulk
```

**Implementação**:

```go
func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-sigCh
        fmt.Fprintln(os.Stderr, "\ninterrupted, shutting down...")
        cancel()
    }()

    // ...
    results := svc.Publish(ctx, events, f.rate)
    // ...
}
```

### 1.4 Comportamento sob Falha Parcial (Bulk Mode)

| Cenário                          | Comportamento                                               |
|----------------------------------|-------------------------------------------------------------|
| Um evento inválido no bulk       | Evento pula (Result com erro), restante continua.           |
| Kafka cai durante bulk           | Eventos restantes falham com erro de conexão.               |
| Arquivo JSON com alguns inválidos| Eventos válidos publicados, inválidos reportados como erro. |
| Stdin pipe fechado no meio       | Leitura até EOF, publica o que foi lido.                    |
| Ctrl+C durante bulk              | Publicação interrompida, resultados parciais exibidos.      |

---

## 2. Concorrência

### 2.1 Modelo de Concorrência

- **Single-threaded**: a CLI publica eventos sequencialmente (1 por vez).
  Não há concorrência interna no bulk — o rate limiting usa `time.Ticker`
  entre publicações.
- **SyncProducer**: sarama gerencia a conexão com Kafka internamente com
  suas próprias goroutines, mas `SendMessage` é thread-safe.
- **Context**: propagado para todas as operações bloqueantes.
- **Rate limiting**: `time.Ticker` no mesmo fluxo (não cria goroutines
  extras para controle de taxa).

### 2.2 Proteção contra Goroutine Leaks

- A única goroutine explícita criada é o signal handler (Ctrl+C).
- `syncProducer.Close()` é chamado via `defer`.
- `ticker.Stop()` é chamado via `defer` dentro de `Publish`.

### 2.3 Race Conditions

- Não há estado compartilhado entre eventos — cada evento é independente.
- O resultado de cada evento é armazenado em um slice indexado (não
  concorrente).
- `sync.Producer` do sarama é thread-safe.

---

## 3. Observabilidade

### 3.1 Logs e Saída

A CLI não usa `slog` — a saída é direta no terminal (stdout/stderr).

**Formato texto (default)**:

```
✓ Published abc-123 → partition 0, offset 42
✗ Failed def-456 → validation error: 'amount' must be greater than 0
```

**Formato JSON (`--json-output`)**:

```json
{"status":"success","payment_id":"abc-123","partition":0,"offset":42}
{"status":"error","payment_id":"def-456","error":"validation error: 'amount' must be greater than 0"}
```

**Proibido exibir**:
- Senhas, tokens, secrets (nenhum secret manipulado pela CLI).
- Detalhes internos de infraestrutura (IPs, paths completos de binários).

### 3.2 Erros para stderr

- Erros de I/O, validação, e Kafka são SEMPRE escritos em stderr.
- Resultados bem-sucedidos vão para stdout.
- Isso permite pipear apenas sucessos: `producer publish --json-output | jq -s .`

### 3.3 Códigos de Saída

| Situação               | Exit Code |
|------------------------|-----------|
| Todos eventos publicados com sucesso | 0 |
| Algum evento falhou (validação/Kafka) | 1 |
| Erro de parse de flags | 1 |
| Erro de I/O (arquivo) | 1 |
| Erro de conexão Kafka | 1 |
| Ctrl+C durante publicação | 1 (se não concluiu) |

---

## 4. Segurança Operacional

### 4.1 Gerenciamento de Secrets

- A CLI não lida com secrets.
- Conecta ao Kafka sem autenticação (ambiente de desenvolvimento).
- Para ambientes com SASL/SSL, o sarama `Config` pode ser estendido
  futuramente via variáveis de ambiente (fora do escopo desta spec).

### 4.2 Payload Validation

- Mesma validação do consumer (reuso do `internal/validator`).
- Tamanho máximo do payload: **10 KB** (herdado do validator).
- Caracteres de controle rejeitados (campo `description`).
- JSON mal formatado → erro claro.

### 4.3 File Size Limit

- Arquivos `--file` maiores que **10 MB** devem ser rejeitados.

```go
const maxFileSize = 10 * 1024 * 1024 // 10 MB

func readEventsFromFile(path string) ([]*models.PaymentEvent, error) {
    info, err := os.Stat(path)
    if err != nil {
        return nil, fmt.Errorf("file error: %w", err)
    }
    if info.Size() > maxFileSize {
        return nil, fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), maxFileSize)
    }
    // ...
}
```

### 4.4 Tratamento de Erro Seguro

- Erros de validação exibem mensagem amigável: `validation error: ...`.
- Erros de Kafka exibem mensagem genérica: `kafka error: connection refused`.
- Não expor detalhes internos (stack trace, configuração completa).
- Erros de arquivo exibem path relativo ou nome, não caminho absoluto completo.

### 4.5 Stdin Seguro

- `io.ReadAll(os.Stdin)` com limite implícito de memória (não há limite
  explícito — documentar para o usuário não pipear arquivos enormes).
- Recomendar `--file` para arquivos grandes em vez de stdin.

---

## 5. Produção

### 5.1 Esta CLI Não é para Uso em Produção

A ferramenta é explicitamente para **desenvolvimento e QA**. Em produção,
eventos de pagamento devem ser publicados pela API de pagamentos ou gateway
externo.

### 5.2 Configuração Recomendada para Desenvolvimento

| Parâmetro          | Dev Local      | CI              |
|--------------------|----------------|-----------------|
| `--brokers`        | localhost:9092 | kafka:9092      |
| `--topic`          | payment.events | payment.events  |
| `--rate` (bulk)    | 5-10           | 20-50           |
| `--count` (bulk)   | 10-100         | 100-1000        |

### 5.3 Dependências (go.mod)

Nenhuma nova dependência é adicionada. As existentes são:

| Dependência                        | Uso                      |
|------------------------------------|--------------------------|
| `github.com/IBM/sarama`            | Kafka SyncProducer       |
| `github.com/google/uuid`           | Geração de UUID          |
| `github.com/go-playground/validator/v10` | Validação (via internal/validator) |
| `github.com/stretchr/testify`      | Testes                   |

---

## 6. Plano de Testes de Hardening

### 6.1 Testes de Falha

| Teste                          | Descrição                                               |
|--------------------------------|---------------------------------------------------------|
| Kafka broker morto             | CLI reporta erro de conexão, exit 1.                    |
| Broker volta após falha        | Executar de novo após broker subir — funciona.          |
| Arquivo JSON corrompido        | Erro claro de parse, exit 1.                            |
| Stdin pipe com dados inválidos | Erro de validação, exit 1.                              |
| Bulk parcial                   | 10 eventos, 2 inválidos → 8 sucesso, 2 erro, exit 1.   |
| Ctrl+C durante bulk            | Interrompe, exibe resultados parciais.                  |
| Flags conflitantes             | `--payload` + `--file` → `--payload` tem prioridade.   |

### 6.2 Testes de Estresse

| Cenário                        | Carga              | Comportamento Esperado            |
|--------------------------------|--------------------|-----------------------------------|
| Bulk 1000 eventos sem rate     | 1000 eventos       | Publica o mais rápido possível    |
| Bulk 1000 eventos com rate 50  | 1000 eventos       | Leva ~20 segundos (±5%)           |
| Payload grande (8 KB)          | 1 evento           | Publica sem erro                  |
| Payload excessivo (>10 KB)     | 1 evento           | Erro de validação (max 10KB)      |

### 6.3 Testes de Concorrência

| Teste                          | Descrição                                               |
|--------------------------------|---------------------------------------------------------|
| Race detector                  | `go test -race ./...` passa sem data races.             |
| Ctrl+C repetido                | Múltiplos SIGINT não causam panic.                      |
| Bulk com cancelamento          | Context cancel interrompe rate limiting imediatamente.  |
| Ticker stop                    | Rate limit não vaza ticker (defer ticker.Stop()).        |

---

## 7. Checklist de Hardening

### Resiliência
- [ ] Timeout de 10s para Kafka SyncProducer configurado.
- [ ] Timeout de 5s para conexão TCP configurado.
- [ ] Graceful shutdown com context cancel e tratamento de sinais.
- [ ] Falha de um evento não interrompe bulk (prossegue com próximos).
- [ ] Arquivos > 10MB são rejeitados.

### Concorrência
- [ ] Nenhuma goroutine explícita além do signal handler.
- [ ] `defer syncProducer.Close()` no main.
- [ ] `defer ticker.Stop()` no Publish.
- [ ] Testes passam com `-race`.
- [ ] Context cancel interrompe rate limiting.

### Observabilidade
- [ ] Saída clara: stdout para sucesso, stderr para erro.
- [ ] Código de saída 0 para sucesso, 1 para erro.
- [ ] Formato JSON Lines em `--json-output` é parseável.
- [ ] Mensagens de erro são descritivas mas não expõem detalhes internos.

### Segurança
- [ ] Payload validado com mesmo validator do consumer.
- [ ] Tamanho de payload limitado a 10 KB (pelo validator).
- [ ] Tamanho de arquivo limitado a 10 MB.
- [ ] Nenhum secret é manipulado ou exibido.
- [ ] Erros não expõem stack traces ou configurações sensíveis.

### Produção (uso em dev/QA)
- [ ] Documentado que não é para uso em produção.
- [ ] Configuração recomendada para dev e CI documentada.
- [ ] Zero novas dependências Go.
- [ ] `make build-producer` compila binário único.
