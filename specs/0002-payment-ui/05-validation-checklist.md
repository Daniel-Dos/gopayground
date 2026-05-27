# 05 — Checklist de Validação

## Instruções

Este checklist deve ser executado pelo **Senior Engineer** durante a implementação
e validado pelo **Hardening Engineer** antes da liberação.

---

## 1. Testes Unitários por Pacote

### 1.1 Config (`config/`)

- [ ] Valores default para `UI_PORT`, `UI_EVENT_BUS_BUFFER`, `UI_READ_TIMEOUT`, `UI_WRITE_TIMEOUT`.
- [ ] Override via variáveis de ambiente.
- [ ] Timeout default de leitura é 10s.
- [ ] Timeout default de escrita é 30s.

### 1.2 EventBus (`internal/ui/events.go`)

- [ ] `Publish` envia evento para o Redis Pub/Sub ou canal em memória.
- [ ] `Subscribe` retorna canal que recebe eventos publicados.
- [ ] `Unsubscribe` (função retornada) remove subscriber e fecha canal.
- [ ] Subscriber lento não bloqueia `Publish` (descartado com warning).
- [ ] Múltiplos subscribers recebem o mesmo evento.
- [ ] `Close` limpa todos os subscribers e fecha canais.
- [ ] Contexto cancelado interrompe `listenRedis`.
- [ ] Erro de marshaling do evento não causa panic.
- [ ] Erro de unmarshaling no listener não quebra o loop.

### 1.3 Handlers (`internal/ui/handlers.go`)

#### handleListPayments

- [ ] Retorna lista vazia `[]` quando não há chaves no Redis.
- [ ] Retorna payments com status correto do Redis.
- [ ] Filtro `payment_id` (LIKE) funciona.
- [ ] Filtro `status` (exato) funciona.
- [ ] Filtros combinados funcionam.
- [ ] Redis scan com erro → retorna 500 com mensagem de erro.
- [ ] Redis HGetAll com erro → ignora a chave (não quebra a resposta).
- [ ] Timeout de contexto respeitado (5s).

#### handlePaymentHistory

- [ ] Retorna array de histórico para payment_id existente.
- [ ] Retorna array vazio para payment_id inexistente.
- [ ] Retorna 400 se payment_id não informado.
- [ ] Histórico ordenado por timestamp ascendente.
- [ ] DynamoDB query com erro → retorna 500.
- [ ] Unmarshal com erro → ignora item (não quebra a resposta).
- [ ] Timeout de contexto respeitado (10s).
- [ ] Path parameter `{id}` corretamente parseado.

#### handleMetrics

- [ ] Retorna `total_processed` igual ao número de chaves `payment:*` no Redis.
- [ ] `by_status` contém contagem correta por status.
- [ ] `success_rate` calculado como `confirmed / (confirmed + failed + refunded) * 100`.
- [ ] `success_rate` é 0 quando não há payments processados (evita divisão por zero).
- [ ] Redis scan com erro → retorna 200 com métricas parciais.

#### handleHealth

- [ ] Retorna 200 com `{"status":"ok"}` quando Redis responde.
- [ ] Retorna 503 com `{"status":"unhealthy"}` quando Redis falha.
- [ ] Timeout de 2s respeitado.

#### handleSSE

- [ ] Headers corretos: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`.
- [ ] Evento `payment` transmitido quando evento é publicado.
- [ ] Heartbeat enviado a cada 30s.
- [ ] Conexão termina quando cliente desconecta (context cancelado).
- [ ] Flusher é verificado (retorna 500 se não suportar streaming).

### 1.4 Server (`internal/ui/server.go`)

- [ ] Root path `/` serve arquivo HTML.
- [ ] API paths roteiam para handlers corretos.
- [ ] Path desconhecido retorna 404.
- [ ] Middleware de logging não interfere na resposta.
- [ ] Middleware de recovery captura panic e retorna 500.
- [ ] `Start` retorna erro se porta já está em uso.
- [ ] `Shutdown` finaliza conexões dentro do timeout.
- [ ] `Shutdown` chama `eventBus.Close()`.

---

## 2. Testes de Frontend (Manual/Navegador)

### 2.1 Dashboard

- [ ] Página carrega sem erros no console.
- [ ] Título "Payment Consumer Dashboard" visível.
- [ ] Status da conexão exibe "🟢 Connected" após carregar.

### 2.2 Feed em Tempo Real

- [ ] Feed SSE exibe novos eventos automaticamente.
- [ ] Eventos aparecem no topo da lista (mais recente primeiro).
- [ ] Feed suporta scroll quando muitos eventos.
- [ ] Ao perder conexão, status muda para "🔴 Disconnected".
- [ ] Ao reconectar, status volta para "🟢 Connected".
- [ ] Reconexão não duplica eventos (idempotência na lista).

### 2.3 Tabela de Pagamentos

- [ ] Dados carregam da API e exibem na tabela.
- [ ] Badges de status com cores corretas:
  - pending: amarelo
  - confirmed: verde
  - failed: vermelho
  - refunded: azul
- [ ] Colunas: Payment ID, Status, Updated At, Actions.
- [ ] Ao receber novo evento SSE, a linha correspondente é atualizada.
- [ ] Se payment_id não existe na tabela, nova linha é adicionada.

### 2.4 Histórico (Modal)

- [ ] Clicar em "View History" abre modal.
- [ ] Título do modal contém payment_id correto.
- [ ] Tabela de histórico exibe todas as transições de status.
- [ ] Colunas: Timestamp, Status, Amount, Currency, Description, Processed At, Trace ID.
- [ ] Fechar modal (X ou clique fora) funciona.
- [ ] Abrir histórico de outro payment_id atualiza o conteúdo.

### 2.5 Métricas

- [ ] Total processed exibe número correto.
- [ ] By status exibe contagem por status.
- [ ] Success rate exibe porcentagem formatada.
- [ ] DLQ count exibe valor (ou 0 se não há DLQ).
- [ ] Métricas atualizam após novos eventos SSE.

### 2.6 Filtros

- [ ] Filtro por payment_id filtra a tabela.
- [ ] Filtro por status filtra a tabela.
- [ ] Limpar filtros restaura lista completa.
- [ ] Filtros não afetam o feed SSE.
- [ ] Requisição de filtro tem timeout (não trava a UI).

### 2.7 Responsividade

- [ ] Layout funciona em viewport >= 320px (mobile).
- [ ] Tabela de pagamentos tem scroll horizontal em mobile.
- [ ] Modal ocupa largura total em mobile.
- [ ] Fonte e padding adaptáveis.

---

## 3. Testes de Integração

### 3.1 API Completa

- [ ] `GET /` → 200, Content-Type `text/html`.
- [ ] `GET /api/payments` → 200, JSON array.
- [ ] `GET /api/payments/{id}/history` → 200, JSON array (ou 404).
- [ ] `GET /api/metrics` → 200, JSON objeto.
- [ ] `GET /api/events` → 200, Content-Type `text/event-stream`.
- [ ] `GET /healthz` → 200, JSON `{"status":"ok"}`.
- [ ] `GET /nonexistent` → 404.

### 3.2 Redis

- [ ] Inserir dados no Redis manualmente → UI reflete.
- [ ] Remover dados do Redis → UI reflete (lista vazia).
- [ ] Redis indisponível → API retorna 500 nos endpoints relevantes.
- [ ] Redis reinicia → UI se recupera (próxima requisição).

### 3.3 DynamoDB

- [ ] Inserir histórico no DynamoDB → modal exibe dados.
- [ ] DynamoDB indisponível → endpoint de histórico retorna 500.
- [ ] Tabela DynamoDB vazia → endpoint de histórico retorna array vazio.

### 3.4 SSE + Redis Pub/Sub

- [ ] Publicar evento no Redis via `PUBLISH payment:events '{"payment_id":"..."}'` → SSE recebe.
- [ ] Payload mal formatado no Redis → ignorado (não quebra SSE).
- [ ] Múltiplos clients SSE recebem o mesmo evento.

---

## 4. Testes de Concorrência

### 4.1 Race Detector

- [ ] Todos os testes passam com `-race` flag.
- [ ] Nenhum data race no Event Bus (subscribers map com RWMutex).
- [ ] Nenhum data race nos handlers (request-scoped data).

### 4.2 Conexões SSE Simultâneas

- [ ] 10+ clients SSE conectados simultaneamente recebem eventos.
- [ ] Desconexão de um client não afeta os demais.
- [ ] Cliente lento não acumula eventos indefinidamente.

### 4.3 Event Bus

- [ ] Publicação concorrente de eventos (10 goroutines) não causa panic.
- [ ] Subscribe/Unsubscribe concorrente não causa race condition.
- [ ] Buffer do EventBus não estoura em pico de eventos.

---

## 5. Testes de Graceful Shutdown

### 5.1 Sinal SIGTERM

- [ ] Servidor HTTP para de aceitar novas conexões.
- [ ] SSE connections em andamento são finalizadas.
- [ ] EventBus é fechado (subscribers notificados).
- [ ] Log de shutdown é emitido.
- [ ] Servidor finaliza dentro do timeout de 15s.

### 5.2 Timeout

- [ ] Se shutdown excede 15s, processo termina.
- [ ] Conexões SSE lentas não impedem shutdown.

---

## 6. Testes de Falha

### 6.1 Falha do Redis

- [ ] `GET /api/payments` → 500 com mensagem de erro.
- [ ] `GET /api/metrics` → 200 com métricas parciais (ou zero).
- [ ] `GET /healthz` → 503.
- [ ] UI mostra aviso de erro (não quebra a página).

### 6.2 Falha do DynamoDB

- [ ] `GET /api/payments/{id}/history` → 500 com mensagem de erro.
- [ ] UI mostra aviso de erro no modal.

### 6.3 Falha do Kafka (DLQ count indisponível)

- [ ] `GET /api/metrics` → `dlq_count` retorna 0 (não quebra).

### 6.4 Payload Inesperado

- [ ] Evento SSE com campo faltando → renderização parcial (não quebra).
- [ ] Resposta da API com formato inesperado → UI mostra erro amigável.

---

## 7. Testes de Performance

### 7.1 Throughput de Eventos

- [ ] 100 eventos SSE/segundo → feed atualiza sem atraso perceptível.
- [ ] 1000 eventos SSE/segundo → eventos são descartados (buffer cheio) sem
      bloquear o publicador.

### 7.2 Latência

- [ ] API `/api/payments` com 1000 chaves no Redis < 500ms.
- [ ] API `/api/metrics` com 1000 chaves no Redis < 500ms.
- [ ] API `/api/payments/{id}/history` com 100 itens no DynamoDB < 200ms.

### 7.3 Consumo de Recursos

- [ ] Memória < 50 MB em operação normal.
- [ ] Número de goroutines estável (não cresce com SSE connections).
- [ ] CPU < 10% em idle.

---

## 8. Testes de Segurança

### 8.1 Path Traversal

- [ ] `GET /../etc/passwd` → 404 (embed.FS protege).
- [ ] `GET /static/../../../etc/passwd` → 404.

### 8.2 Documentação

- [ ] `GET /docs/` serve a página inicial da documentação
- [ ] `GET /docs/index.html` serve `index.html` corretamente
- [ ] `GET /docs/setup.html` serve o guia de setup
- [ ] `GET /docs/architecture.html` serve o documento de arquitetura
- [ ] `GET /docs/features/payment-consumer.html` serve documentação de features
- [ ] `GET /docs/diagrams/architecture-overview.jpg` serve imagem de diagrama
- [ ] Path traversal em `/docs/../../../etc/passwd` retorna 404 (protegido por embed.FS)
- [ ] Documentação é servida sem headers de cache agressivos (ou com cache curto)
- [ ] Links de navegação na UI principal incluem link para `/docs/`
- [ ] Links de navegação na documentação incluem link de volta para a UI

### 8.4 Injeção

- [ ] `payment_id` com caracteres especiais é tratado como string (não query injection).
- [ ] XSS via `payment_id` no HTML é prevenido (textContent vs innerHTML).

### 8.5 Informação

- [ ] Erros de API não expõem detalhes internos (stack trace).
- [ ] Headers de resposta não expõem versão do servidor.

---

## 9. Regressão

- [ ] Todos os testes unitários passam.
- [ ] Todos os testes de integração passam.
- [ ] Nenhum teste do consumer (spec 0001) quebrado.
- [ ] Cobertura de código > 70% no pacote `internal/ui`.

---

## Resumo para Aprovação

| Área               | Status | Observações |
|--------------------|--------|-------------|
| Unit Tests         | [ ]    |             |
| Frontend (Manual)  | [ ]    |             |
| Integration Tests  | [ ]    |             |
| Concorrência       | [ ]    |             |
| Graceful Shutdown  | [ ]    |             |
| Falhas             | [ ]    |             |
| Performance        | [ ]    |             |
| Segurança          | [ ]    |             |
| Regressão          | [ ]    |             |

**Assinatura do Hardening Engineer:** ____________________

**Data da validação:** ____________________

---
