# 01 — Contexto

## Contexto do Negócio

O sistema de processamento de pagamentos consome eventos do Kafka através do
**consumer** (spec `0001-kafka-payment-consumer`) e oferece uma **dashboard
web** (spec `0002-payment-ui`) para monitorar os eventos processados em tempo
real, consultar status no Redis, histórico no DynamoDB e métricas agregadas.

No entanto, a **publicação** de eventos no Kafka ainda é feita exclusivamente
via CLI:

```bash
go run ./cmd/producer publish --status confirmed --amount 150.00
```

Isso é funcional, mas impõe uma barreira de entrada para:
- Desenvolvedores que querem testar cenários rapidamente sem abrir terminal
- QA e operadores que precisam disparar eventos manualmente durante validação
- Demonstrações e apresentações do sistema

### Problema

Atualmente, não há uma interface gráfica para **publicar** eventos de pagamento
no Kafka. O fluxo de desenvolvimento exige que o usuário:

1. Abra um terminal
2. Lembre dos parâmetros do CLI (`--payment-id`, `--status`, `--amount`, etc.)
3. Execute o comando manualmente
4. Alterne para o navegador para ver o resultado na dashboard

### Solução Proposta

Adicionar uma **Producer UI** como uma nova página web dentro do serviço `ui`
(porta 8081), permitindo que qualquer usuário publique pagamentos no Kafka
diretamente do navegador.

A Producer UI será servida pelo mesmo servidor HTTP Go que já serve a
dashboard (`cmd/ui`), usando o mesmo `embed.FS` para arquivos estáticos.

### Público-alvo

- **Desenvolvedores** que precisam testar o fluxo de pagamentos rapidamente
- **QA** que executa cenários de teste manuais
- **Operadores** que precisam simular eventos em ambientes de staging
- **Apresentações/demos** do sistema

### Não Escopo

- Substituição completa do CLI (o CLI continua existindo para scripts e automação)
- Publicação em lote com alta taxa de throughput (para isso, usar o CLI com `--count` e `--rate`)
- Edição/remoção de eventos já publicados
- Agendamento de eventos futuros
- Publicação em múltiplos tópicos simultaneamente

---

## Sistemas Envolvidos

| Sistema     | Função                                        | Acesso pela Producer UI |
|-------------|-----------------------------------------------|-------------------------|
| **Kafka**   | Destino dos eventos publicados                | Escrita (via API)       |
| **Redis**   | Publicação no EventBus para feed SSE em tempo real | Escrita            |
| **Consumer**| Consome os eventos e persiste (status + histórico) | Indireto (via Kafka) |

---

## Fluxo de Dados (novo)

```
Browser (Producer UI)
    │
    │ POST /api/publish  (JSON PaymentEvent)
    ▼
Go HTTP Server (:8081)
    │
    ├── Valida payload (reusa validator existente)
    ├── Publica no Kafka (tópico payment.events)
    ├── Publica no Redis Pub/Sub (canal payment:events)
    │       └── EventBus distribui para SSE clients
    └── Retorna 200 + resultado (partition, offset)
```

---
