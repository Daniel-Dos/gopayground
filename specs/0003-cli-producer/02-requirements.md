# 02 — Requisitos

## Requisitos Funcionais

| ID     | Descrição                                                                          | Prioridade |
|--------|------------------------------------------------------------------------------------|------------|
| RF-001 | Aceitar JSON payload como argumento via `--payload` ou pipe do stdin.              | Alta       |
| RF-002 | Gerar evento mínimo válido com UUID aleatório se nenhum dado for fornecido.        | Alta       |
| RF-003 | Aceitar campos individuais como flags: `--payment-id`, `--status`, `--amount`,     | Média      |
|        | `--currency`, `--description`.                                                     |            |
| RF-004 | Publicar no tópico `payment.events` (configurável via `--topic`).                  | Alta       |
| RF-005 | Suportar dry-run mode (`--dry-run`) que exibe o evento sem publicar.               | Alta       |
| RF-006 | Suportar bulk mode (`--count N`) para publicar N eventos com dados sequenciais.    | Alta       |
| RF-007 | Suportar rate limiting (`--rate N`) no bulk mode (eventos por segundo).            | Média      |
| RF-008 | Confirmar publicação exibindo partição e offset no Kafka.                          | Alta       |
| RF-009 | Sair com código não-zero em caso de falha (validação, Kafka, I/O).                 | Alta       |
| RF-010 | Suportar leitura de eventos de um arquivo JSON (`--file`).                         | Média      |
| RF-011 | Suportar saída em JSON para scripting (`--json-output`).                           | Média      |
| RF-012 | Validar payload usando o mesmo validator do consumer (`internal/validator`).        | Alta       |
| RF-013 | Gerar timestamp RFC3339 automaticamente se não fornecido.                          | Alta       |

### Schema do Payload

O payload gerado/publicado segue exatamente o mesmo schema do consumer:

```json
{
  "payment_id":   "a1b2c3d4-e5f6-7890-abcd-ef1234567890",   // UUID v4
  "status":       "confirmed",                                 // pending|confirmed|failed|refunded
  "amount":       150.00,                                      // float64 > 0
  "currency":     "BRL",                                       // ISO 4217
  "description":  "Pagamento de teste",                        // opcional, max 255 chars
  "timestamp":    "2026-05-24T10:30:00Z"                       // RFC3339
}
```

### Regras de Validação

Mesmas regras do consumer (RF-012):

- `payment_id`: UUID v4 válido.
- `status`: `pending`, `confirmed`, `failed`, `refunded`.
- `amount`: float64, > 0, finito.
- `currency`: ISO 4217, 3 letras maiúsculas.
- `timestamp`: RFC3339 válido, não futuro (> 5 min skew).
- `description`: string opcional, max 255 chars, apenas ASCII imprimível.

---

## Requisitos Não Funcionais

| ID      | Descrição                                                | Métrica      | Alvo             |
|---------|----------------------------------------------------------|--------------|------------------|
| RNF-001 | Nenhuma nova dependência Go além das já existentes.      | go.mod       | Sem novas deps   |
| RNF-002 | Binário único (`producer`) sem dependência externa.      | build        | Single binary    |
| RNF-003 | Modo JSON output (`--json-output`) produz JSON parseável.| saída        | JSON por linha   |
| RNF-004 | Código de saída zero em sucesso, não-zero em falha.      | exit code    | 0 ou 1           |
| RNF-005 | Bulk mode com `--rate` deve respeitar a taxa configurada.| eventos/s    | ±5% do alvo      |
| RNF-006 | Tempo máximo de publicação individual: 10s.              | timeout      | 10s              |

---

## Restrições Técnicas

| ID  | Restrição                                            | Justificativa                                |
|-----|------------------------------------------------------|----------------------------------------------|
| R01 | CLI em Go, mesmo module (`exemplo.com/teste`).       | Stack definida no projeto                    |
| R02 | Kafka client: `github.com/IBM/sarama` (existente).   | Já em uso pelo consumer                      |
| R03 | Validação: `internal/validator` (reusar).            | Consistência de regras com o consumer        |
| R04 | Models: `internal/models` (reusar `PaymentEvent`).   | Mesma struct de domínio                      |
| R05 | Flag parsing: `flag` stdlib.                         | Zero dependências novas                      |
| R06 | UUID: `github.com/google/uuid` (já em go.mod).       | Já disponível no projeto                     |
| R07 | Testes: `testing` + `github.com/stretchr/testify`.   | Padrão do projeto                            |

---

## Histórico de Revisão

| Versão | Data       | Autor     | Descrição           |
|--------|------------|-----------|---------------------|
| 1.0    | 2026-05-24 | Architect | Versão inicial      |
