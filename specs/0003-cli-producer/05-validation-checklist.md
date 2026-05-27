# 05 — Checklist de Validação

## Instruções

Este checklist deve ser executado pelo **Senior Engineer** durante a implementação
e validado pelo **Hardening Engineer** antes da liberação.

---

## 1. Testes Unitários

### 1.1 Producer Service (`internal/producer/`)

- [ ] `Publish` com evento válido → Result sem erro, partition/offset preenchidos.
- [ ] `Publish` com evento inválido (amount = 0) → Result com erro de validação.
- [ ] `Publish` com Kafka offline → Result com erro de conexão.
- [ ] `Publish` com rate limit = 10 → eventos publicados em ≈ N/10 segundos.
- [ ] `Publish` com contexto cancelado → publicação interrompida.
- [ ] `Publish` com array vazio → retorna slice vazio.
- [ ] `GenerateEvent` com todos os campos → retorna evento com dados fornecidos.
- [ ] `GenerateEvent` com paymentID vazio → gera UUID v4.
- [ ] `GenerateEvent` com status vazio → default "confirmed".
- [ ] `GenerateEvent` com amount = 0 → default 100.00.
- [ ] `GenerateEvent` com currency vazio → default "BRL".
- [ ] `GenerateEvent` sempre preenche timestamp com RFC3339.
- [ ] `GenerateBulkEvents(100)` retorna 100 eventos.
- [ ] `GenerateBulkEvents` cada evento tem payment_id único (UUID).
- [ ] `GenerateBulkEvents` amounts são sequenciais (10, 20, 30...).

### 1.2 Validação (reuso do validator)

- [ ] Payload com `payment_id` inválido → erro.
- [ ] Payload com `status` inválido → erro.
- [ ] Payload com `amount` = 0 → erro.
- [ ] Payload com `amount` negativo → erro.
- [ ] Payload com `currency` de 2 letras → erro.
- [ ] Payload com `currency` minúsculo → erro.
- [ ] Payload com `timestamp` inválido → erro.
- [ ] Payload com `timestamp` futuro (> 5 min skew) → erro.
- [ ] Payload com `description` > 255 chars → erro.
- [ ] Payload com caracteres de controle → erro.
- [ ] Payload vazio (`{}`) → erro de campos required.
- [ ] Payload válido passa na validação sem erro.

---

## 2. Testes de Integração do CLI

### 2.1 Publicação com flags

- [ ] `producer publish --payment-id "abc-123" --status confirmed --amount 150.00 --currency BRL` publica e exibe partition/offset.
- [ ] `producer publish` (sem flags) gera evento com UUID aleatório e publica.

### 2.2 Modo dry-run

- [ ] `producer publish --dry-run` exibe JSON válido sem publicar.
- [ ] `producer publish --payment-id "abc-123" --dry-run` exibe o JSON com o ID fornecido.
- [ ] `producer publish --dry-run --json-output` produz saída JSON.

### 2.3 Bulk mode

- [ ] `producer publish --count 10` publica 10 eventos.
- [ ] `producer publish --count 5 --rate 2` leva ~2.5 segundos.
- [ ] `producer publish --count 0` (ou < 1) → erro ou trata como 1.

### 2.4 File input

- [ ] `producer publish --file events.json` com array JSON válido publica todos.
- [ ] `producer publish --file events.json` com objeto único publica 1 evento.
- [ ] `producer publish --file invalid.json` → erro não-zero.
- [ ] `producer publish --file empty.json` → erro "no events found".

### 2.5 Stdin pipe

- [ ] `echo '{"payment_id":"...","status":"confirmed"}' | producer publish` publica.
- [ ] `echo '[{...},{...}]' | producer publish` publica array de eventos.

### 2.6 JSON output mode

- [ ] `producer publish --json-output` produz JSON lines por linha.
- [ ] `producer publish --dry-run --json-output` produz JSON válido.
- [ ] Saída JSON é parseável com `jq -s .`.

### 2.7 Error handling

- [ ] `producer publish --amount -10` → erro, exit 1.
- [ ] `producer publish --status invalid` → erro, exit 1.
- [ ] `producer publish --brokers invalid:9092` → erro de conexão, exit 1.
- [ ] `producer publish --payment-id "not-a-uuid"` → erro de validação, exit 1.
- [ ] `producer publish --file /nao/existe.json` → erro de arquivo, exit 1.

### 2.8 Help

- [ ] `producer publish --help` exibe uso e flags.
- [ ] `producer --help` exibe uso.

---

## 3. Testes de Concorrência

- [ ] `go test -race ./...` passa sem data races.
- [ ] Ctrl+C durante bulk interrompe e sai limpo (sem goroutine leaks).
- [ ] `Publish` com rate limit não cria goroutines órfãs.

---

## 4. Testes de Saída

### 4.1 Formato texto (default)

```
✓ Published abc-123 → partition 0, offset 42
```

- [ ] Linha começa com ✓ para sucesso.
- [ ] Linha começa com ✗ ou "Error" para falha.
- [ ] Partição e offset são exibidos.

### 4.2 Formato JSON

```json
{"status":"success","payment_id":"abc-123","partition":0,"offset":42}
{"status":"error","payment_id":"def-456","error":"validation error: ..."}
```

- [ ] JSON é linha-a-linha (JSON Lines).
- [ ] Campo `status` presente.
- [ ] Campo `payment_id` presente.
- [ ] Campo `partition` e `offset` no sucesso.
- [ ] Campo `error` no erro.
- [ ] JSON é válido (`jq` não reclama linha a linha).

---

## 5. Testes de Regressão

- [ ] Todos os testes do consumer (`go test ./internal/...`) continuam passando.
- [ ] Makefile targets funcionam (`make build-producer`, `make run-producer`).
- [ ] `go build ./cmd/producer` compila sem erros.
- [ ] `go vet ./cmd/producer ./internal/producer` não reporta problemas.

---

## 6. Casos de Borda

- [ ] Evento com `description` vazia → aceito.
- [ ] Evento com `amount` = 0.01 → aceito.
- [ ] Evento com `amount` máximo (1e9) → aceito.
- [ ] Bulk com `--count 1` → equivalente a single.
- [ ] Bulk com `--rate 0` → sem rate limiting (máxima velocidade).
- [ ] Stdin vazio (sem pipe) → usa default/flags.
- [ ] Pipe com conteúdo vazio → erro.
- [ ] File com JSON mal formatado → erro descritivo.
- [ ] Payload com campos extras → aceito (tolerância do json.Unmarshal).
- [ ] `--count` muito alto (100000) → não crasha, pode ser lento.
- [ ] Evento com `payment_id` já existente → publicado (idempotência é do consumer).

---

## Resumo para Aprovação

| Área               | Status | Observações |
|--------------------|--------|-------------|
| Unit Tests         | [ ]    |             |
| CLI Integration    | [ ]    |             |
| Concorrência       | [ ]    |             |
| Saída/Formatação   | [ ]    |             |
| Regressão          | [ ]    |             |
| Casos de Borda     | [ ]    |             |

**Assinatura do Hardening Engineer:** ____________________

**Data da validação:** ____________________
