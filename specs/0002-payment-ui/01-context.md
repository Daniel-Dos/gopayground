# 01 — Contexto

## Contexto do Negócio

Este spec define uma **interface web de desenvolvimento (dev UI)** para o
**Kafka Payment Consumer** (spec `0001-kafka-payment-consumer`). O consumer
processa eventos de pagamento e armazena:

- **Status atual** no Redis (chave `payment:<payment_id>` com HSET contendo
  `payment_id`, `status`, `updated_at`).
- **Histórico completo** no DynamoDB (tabela `payment_history` com `payment_id`,
  `status`, `amount`, `currency`, `description`, `timestamp`, `processed_at`,
  `trace_id`).

Atualmente, o único modo de visualizar esses dados é acessar Redis e DynamoDB
diretamente via CLI ou bibliotecas. Não há uma interface gráfica que permita
acompanhar o processamento em tempo real ou investigar o estado de pagamentos
individuais.

### Problema

Desenvolvedores e operadores precisam de uma ferramenta visual para:

1. **Monitorar em tempo real** os eventos sendo consumidos do Kafka.
2. **Consultar o status atual** de qualquer pagamento no Redis.
3. **Navegar pelo histórico completo** de um pagamento no DynamoDB.
4. **Visualizar métricas agregadas** (total processado, taxa de sucesso, DLQ).

Sem essa interface, a depuração de problemas em desenvolvimento e staging é
lenta e propensa a erro, exigindo comandos manuais em múltiplos terminais.

### Solução Proposta

Uma **aplicação web leve** escrita em Go que:

- Serve uma dashboard HTML/CSS/JS em uma porta configurável (default `8081`).
- Expõe endpoints HTTP para consultar dados do Redis e DynamoDB.
- Transmite eventos em tempo real via **Server-Sent Events (SSE)**.
- É independente do consumer — roda como um processo separado que **lê** os
  dados já persistidos, sem interferir no pipeline de processamento.

### Público-alvo

- **Desenvolvedores** que trabalham no sistema de pagamentos e precisam
  depurar o comportamento do consumer.
- **Operadores** que monitoram o sistema em staging e produção (leitura apenas).
- **QA** que valida o fluxo de pagamentos durante testes.

### Não Escopo

- Autenticação/autorização (ferramenta de desenvolvimento, sem requisito de
  segurança).
- Edição/remoção de dados (read-only).
- Reprocessamento manual de mensagens da DLQ.
- Notificações em tempo real para usuários finais.
- Suporte a múltiplos usuários simultâneos (apenas 1 desenvolvedor por
  dashboard).
- Persistência de estado da UI (não há banco de dados próprio).

---

## Sistemas Envolvidos

| Sistema     | Função                                              | Acesso pela UI |
|-------------|-----------------------------------------------------|----------------|
| **Redis**   | Cache de status atual dos pagamentos                | Leitura        |
| **DynamoDB**| Armazenamento permanente do histórico de transações | Leitura        |
| **Consumer**| Origem dos eventos (publica no event bus interno)   | Eventos SSE    |

---

## Fluxo de Dados

```
Consumer Kafka
    │
    ├── Persiste status no Redis ──► UI lê GET /api/payments
    ├── Persiste histórico no DynamoDB ──► UI lê GET /api/payments/:id/history
    └── Publica evento no Event Bus ──► UI transmite via SSE (GET /api/events)
```

---
