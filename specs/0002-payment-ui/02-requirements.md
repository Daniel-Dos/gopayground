# 02 — Requisitos

## Requisitos Funcionais

| ID     | Descrição                                                                                       | Prioridade |
|--------|-------------------------------------------------------------------------------------------------|------------|
| RF-001 | Servir dashboard web acessível via navegador em porta configurável (default 8081)               | Alta       |
| RF-002 | Exibir feed em tempo real de eventos consumidos via SSE (Server-Sent Events)                    | Alta       |
| RF-003 | Listar todos os pagamentos com status atual em uma tabela, lendo do Redis                       | Alta       |
| RF-004 | Exibir histórico completo de um pagamento específico ao clicar, lendo do DynamoDB               | Alta       |
| RF-005 | Atualizar a lista de pagamentos automaticamente quando novos eventos chegarem (push via SSE)    | Alta       |
| RF-006 | Mostrar métricas agregadas: total processado, taxa de sucesso, contagem de DLQ, total por status| Média      |
| RF-007 | Permitir filtro/pesquisa por `payment_id` e `status` na lista de pagamentos                     | Média      |
| RF-008 | Exibir health check básico do servidor                                                          | Baixa      |

### RF-002 — Feed em Tempo Real (SSE)

O feed SSE deve transmitir um evento JSON para cada pagamento consumido pelo
Kafka. Cada evento deve conter:

```json
{
  "payment_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "status": "confirmed",
  "amount": 150.00,
  "currency": "BRL",
  "description": "Pedido #12345",
  "timestamp": "2026-05-24T10:30:00Z",
  "processed_at": "2026-05-24T10:30:01.123Z",
  "trace_id": "0af7651916cd43dd8448eb211c80319c"
}
```

O navegador deve reconectar automaticamente em caso de queda da conexão
(comportamento nativo do `EventSource`).

### RF-003 — Lista de Pagamentos

A tabela de pagamentos deve exibir:

| Coluna       | Descrição                                      |
|--------------|------------------------------------------------|
| Payment ID   | Identificador UUID do pagamento                |
| Status       | Status atual (com badge colorido)              |
| Atualizado em| Timestamp da última atualização                |
| Ações        | Link/botão para ver histórico completo         |

### RF-004 — Histórico do Pagamento

Ao clicar em um pagamento, um modal ou painel lateral deve exibir o histórico
completo em formato de tabela/hierarquia:

| Coluna     | Descrição                                     |
|------------|-----------------------------------------------|
| Timestamp  | Timestamp original do evento                  |
| Status     | Status naquele momento                         |
| Valor      | Amount + Currency                              |
| Descrição  | Descrição do pagamento                         |
| Processado em| Timestamp de quando o consumer processou     |
| Trace ID   | ID de tracing distribuído (OTel)              |

### RF-006 — Métricas

As métricas devem ser calculadas a partir dos dados do Redis e/ou DynamoDB:

| Métrica              | Cálculo                                       |
|----------------------|-----------------------------------------------|
| Total processado     | Quantidade total de registros no DynamoDB     |
| Por status           | Quantidade de payments agrupados por status   |
| Taxa de sucesso      | `confirmed / (confirmed + failed + refunded)` |
| DLQ count            | Quantidade de mensagens na DLQ (tópico Kafka) |

---

## Requisitos Não Funcionais

| ID       | Descrição                                                          | Métrica        | Alvo            |
|----------|--------------------------------------------------------------------|----------------|-----------------|
| RNF-001  | Porta HTTP configurável via variável de ambiente                   | —              | Default 8081    |
| RNF-002  | Serve HTML/CSS/JS puro (sem SPA framework, sem bundler)            | —              | Vanilla JS      |
| RNF-003  | Backend em Go servindo API REST + SSE + arquivos estáticos         | —              | Go 1.22+        |
| RNF-004  | Binary único com arquivos estáticos embutidos via `embed`           | Tamanho        | < 50 MB         |
| RNF-005  | Não deve interferir no processamento do consumer                   | Impacto        | Zero bloqueio   |
| RNF-006  | Latência de resposta da API < 200ms (p99)                          | ms             | p99 < 200ms     |
| RNF-007  | SSE deve reconectar automaticamente (comportamento nativo)         | —              | Obrigatório     |
| RNF-008  | JS requerido para funcionamento (SSE não funciona sem JS)          | —              | Documentado     |
| RNF-009  | Documentação do projeto embutida no binário via `embed` e servida em `/docs/` | Tamanho | < 5 MB adicional |

---

## Restrições Técnicas

| ID  | Restrição                                              | Justificativa                                    |
|-----|--------------------------------------------------------|--------------------------------------------------|
| R01 | Linguagem: Go 1.22+                                    | Stack definida no projeto                        |
| R02 | Sem novas dependências Go além das já existentes       | Minimalismo, binary único                        |
| R03 | Static files (incluindo documentação) embutidos via `embed` (stdlib) | Single binary, sem dependência de diretório      |
| R04 | Redis client: `github.com/redis/go-redis/v9`            | Já existe no projeto                             |
| R05 | DynamoDB client: `github.com/aws/aws-sdk-go-v2`        | Já existe no projeto                             |
| R06 | UI não deve ter dependências npm ou bundlers            | Simplicidade, sem toolchain extra                |
| R07 | Logging: `log/slog` (stdlib)                            | Consistência com o consumer                      |
| R08 | Testes: `testing` padrão + `github.com/stretchr/testify`| Consistência com o projeto                       |

---

## Histórico de Revisão

| Versão | Data       | Autor     | Descrição           |
|--------|------------|-----------|---------------------|
| 1.0    | 2026-05-24 | Architect | Versão inicial      |

---
