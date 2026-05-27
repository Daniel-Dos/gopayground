# 01 — Contexto

## Contexto do Negócio

O sistema de processamento de pagamentos possui uma **dashboard web** (spec `0002-payment-ui`) que exibe métricas agregadas em formato textual (cards numéricos) e uma tabela de pagamentos com feed de eventos em tempo real via SSE.

Atualmente, as métricas são apresentadas como valores numéricos simples:

- Total processado: `150`
- Por status: `pending: 10`, `confirmed: 100`, `failed: 30`, `refunded: 10`
- Taxa de sucesso: `71.43%`
- DLQ count: `2`

### Problema

Números isolados não comunicam padrões, tendências ou distribuições de forma eficiente. Usuários precisam de **visualizações gráficas** para:

1. **Compreender rapidamente a distribuição** de pagamentos por status (proporção entre confirmados, falhas, pendentes e reembolsos).
2. **Avaliar a saúde do sistema** com um gauge visual de taxa de sucesso.
3. **Identificar tendências temporais** — volume de pagamentos processados por hora nas últimas 24h.
4. **Monitorar eventos em tempo real** com indicadores visuais de atualizações instantâneas.
5. **Detectar anomalias** visualmente (picos de falha, queda na taxa de sucesso, acumulo na DLQ).

Sem gráficos, a análise de saúde do sistema exige que o operador interprete mentalmente os números, o que é mais lento e propenso a erro.

### Solução Proposta

Criar uma **página de dashboard gráfica** servida em `/dashboard` que:

- Consome a API `/api/metrics` existente para obter dados agregados
- Conecta-se ao SSE `/api/events` para atualizações em tempo real
- Renderiza **4 tipos de gráficos** usando Canvas API (sem bibliotecas externas):
  - **Donut chart**: distribuição de status (pending/confirmed/failed/refunded)
  - **Gauge semi-circular**: taxa de sucesso com porcentagem
  - **Bar chart (histograma)**: pagamentos por hora nas últimas 24h
  - **Contador de eventos**: total de eventos processados na sessão atual
- Auto-refresh via polling a cada 10s
- Integração SSE para atualizações instantâneas
- Design responsivo empilhando gráficos verticalmente em telas pequenas
- Mantém o sistema de design dark theme existente

### Público-alvo

- **Desenvolvedores** que monitoram o pipeline de pagamentos em tempo real
- **Operadores** que precisam avaliar rapidamente a saúde do sistema
- **QA** que valida cenários de carga e resiliência
- **Gerentes técnicos** que acompanham métricas de processamento

### Não Escopo

- Exportação de gráficos como imagem
- Gráficos interativos (zoom, tooltip avançado, seleção de intervalo)
- Histórico de métricas persistido no backend (apenas dados atuais)
- Comparação entre períodos (dia anterior vs. atual)
- Alertas automáticos baseados em thresholds
- Autenticação/autorização (mesmo modelo read-only da dashboard existente)
- Suporte a WebGL ou bibliotecas de gráficos terceiras
- Personalização de cores ou layout pelo usuário

---

## Sistemas Envolvidos

| Sistema     | Função                                              | Acesso pela Dashboard Gráfica |
|-------------|-----------------------------------------------------|-------------------------------|
| **Redis**   | Cache de status atual dos pagamentos                | Leitura (via API /api/metrics)|
| **DynamoDB**| Armazenamento permanente do histórico de transações | Não utilizado                 |
| **Consumer**| Origem dos eventos (publica no event bus interno)   | Eventos SSE (feed em tempo real) |
| **API HTTP**| Endpoint `/api/metrics` e `/api/events` (SSE)       | Polling + SSE                 |

---

## Fluxo de Dados

```
Browser (Dashboard Gráfica — /dashboard)
    │
    ├── Polling GET /api/metrics (a cada 10s)
    │       └── Resposta JSON com total_processed, by_status, success_rate, dlq_count
    │
    ├── SSE GET /api/events (conexão persistente)
    │       └── Eventos payment → atualiza contador de sessão
    │
    └── Renderização local via Canvas API
            ├── Donut chart (distribuição por status)
            ├── Gauge semi-circular (taxa de sucesso)
            ├── Bar chart (pagamentos por hora — gerado localmente)
            └── Contador de eventos da sessão
```

---

## Dependências

| Dependência | Tipo       | Justificativa                                        |
|-------------|------------|------------------------------------------------------|
| API `/api/metrics` | Existente | Já implementada em `handlers.go` — retorna JSON agregado |
| SSE `/api/events` | Existente  | Já implementada em `events.go` — stream de eventos   |
| `style.css` | Existente  | Design system com variáveis CSS para dark theme       |
| Canvas API  | Nativa     | Renderização de gráficos sem dependências externas   |
