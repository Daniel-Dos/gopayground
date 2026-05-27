# 06 — Riscos e Tradeoffs

## Riscos

### R1 — UI Bloqueia o Consumer

| Atributo       | Detalhe                                                                 |
|----------------|-------------------------------------------------------------------------|
| **Descrição**  | Se o Event Bus da UI bloqueia ao publicar, o consumer pode ficar travado. |
| **Impacto**    | Consumer para de processar mensagens, acumulando lag no Kafka.            |
| **Probabilidade** | Baixa (buffer + non-blocking publish protegem).                        |
| **Mitigação**  | Event Bus com buffer e política de descarte (drop). Publicação em         |
|                | goroutine separada. Timeout de 100ms para publish no Redis Pub/Sub.      |
| **Residual**   | Eventos podem ser perdidos se buffer encher. Aceito (UI não é crítica).  |

### R2 — Perda de Eventos no Event Bus

| Atributo       | Detalhe                                                                 |
|----------------|-------------------------------------------------------------------------|
| **Descrição**  | Se o buffer do Event Bus encher, eventos são descartados.                |
| **Impacto**    | UI não exibe alguns eventos em tempo real.                               |
| **Probabilidade** | Média (depende do throughput do consumer vs capacidade da UI).          |
| **Mitigação**  | Buffer configurável (default 256). Monitorar warning de drop.            |
| **Residual**   | A lista de pagamentos (via Redis) sempre reflete o estado atual, mesmo   |
|                | que eventos individuais sejam perdidos no feed.                          |

### R3 — Redis SCAN com Muitas Chaves

| Atributo       | Detalhe                                                                 |
|----------------|-------------------------------------------------------------------------|
| **Descrição**  | Se há milhares de chaves `payment:*` no Redis, o SCAN pode ser lento.    |
| **Impacto**    | Lista de pagamentos e métricas demoram para carregar.                    |
| **Probabilidade** | Média (depende do volume de pagamentos e TTL configurado).              |
| **Mitigação**  | SCAN com paginação (100 chaves por vez). Timeout de 5s. Se exceder,      |
|                | retorna resultados parciais. UI mostra loading state.                     |
| **Residual**   | Com TTL de 7 dias e throughput moderado, número de chaves é limitado.   |

### R4 — Conexão SSE Não Fecha

| Atributo       | Detalhe                                                                 |
|----------------|-------------------------------------------------------------------------|
| **Descrição**  | Cliente desconecta sem fechar a conexão SSE (ex: Notebook fechado).      |
| **Impacto**    | Goroutine do SSE handler fica presa, acumulando subscribers no EventBus. |
| **Probabilidade** | Alta (comportamento comum de navegadores).                              |
| **Mitigação**  | `r.Context().Done()` detecta quando o cliente desconecta. Unsubscribe    |
|                | automático ao sair do handler. WriteTimeout de 30s no HTTP server.       |
| **Residual**   | Se WriteTimeout for longo, subscriber preso consome memória.             |

### R5 — Dados Sensíveis na UI

| Atributo       | Detalhe                                                                 |
|----------------|-------------------------------------------------------------------------|
| **Descrição**  | Amount e description de pagamentos são exibidos na UI sem autenticação.  |
| **Impacto**    | Qualquer pessoa com acesso à rede pode ver dados financeiros.            |
| **Probabilidade** | Alta (sem autenticação, dev tool).                                      |
| **Mitigação**  | Documentar que é ferramenta de desenvolvimento, sem autenticação.        |
|                | Recomendar execução apenas em rede local/isolada.                        |
| **Residual**   | Aceito. Não é uma ferramenta de produção.                                |

### R6 — DynamoDB Query com Muitos Itens

| Atributo       | Detalhe                                                                 |
|----------------|-------------------------------------------------------------------------|
| **Descrição**  | Um payment_id com centenas de transições de status pode retornar muitos  |
|                | itens do DynamoDB.                                                        |
| **Impacto**    | Modal de histórico demora para carregar.                                 |
| **Probabilidade** | Baixa (pagamentos têm poucas transições de status).                     |
| **Mitigação**  | Limitar histórico aos últimos 50 eventos. Paginação no DynamoDB Query.   |
| **Residual**   | Aceito. UI mostra loading enquanto carrega.                              |

### R7 — Compatibilidade com Consumer

| Atributo       | Detalhe                                                                 |
|----------------|-------------------------------------------------------------------------|
| **Descrição**  | UI depende dos mesmos dados (Redis, DynamoDB) que o consumer. Mudanças   |
|                | no schema do consumer podem quebrar a UI.                                |
| **Impacto**    | UI renderiza dados incorretamente ou quebra.                             |
| **Probabilidade** | Média (em desenvolvimento ativo).                                       |
| **Mitigação**  | Usar os mesmos models (`internal/models`) para serialização. Testes de   |
|                | regressão entre consumer e UI.                                           |
| **Residual**   | Comunicação entre equipes sobre mudanças de schema.                      |

---

## Tradeoffs

### T1 — SSE vs WebSocket

| Opção                    | Prós                                              | Contras                                             |
|--------------------------|---------------------------------------------------|-----------------------------------------------------|
| **SSE (Server-Sent Events)** | Simples, nativo no browser (EventSource),        | Unidirecional (servidor → cliente), limitação de    |
| (escolhido)              | reconexão automática, HTTP padrão.                | conexões simultâneas por domínio (6–8).              |
| WebSocket                | Bidirecional, sem limite de conexões.             | Mais complexo, requer fallback, reconexão manual.   |

**Decisão**: SSE. A comunicação é unidirecional (servidor → cliente). O
navegador reconecta automaticamente. Simplicidade é prioridade.

### T2 — Event Bus: Redis Pub/Sub vs Canal em Memória

| Opção                    | Prós                                              | Contras                                             |
|--------------------------|---------------------------------------------------|-----------------------------------------------------|
| **Redis Pub/Sub**        | Consumer e UI em processos separados sem          | Latência adicional de rede, Redis como dependência  |
| (escolhido)              | acoplamento. Escalável para múltiplas UIs.        | de runtime.                                         |
| Canal em Memória         | Zero latência, sem dependência externa.           | Consumer e UI devem ser mesmo processo.             |

**Decisão**: Redis Pub/Sub como padrão, com fallback para canal em memória
quando consumer e UI rodam no mesmo processo (detectado por flag de config).
A interface (`EventBus`) abstrai a implementação.

### T3 — Embedded vs Served Static Files

| Opção                    | Prós                                              | Contras                                             |
|--------------------------|---------------------------------------------------|-----------------------------------------------------|
| **Embedded (embed.FS)**  | Single binary, sem dependência de diretório,      | Tamanho do binário aumenta. Difícil hot-reload.     |
| (escolhido)              | deploy simples.                                   |                                                     |
| Served de diretório externo | Hot-reload durante desenvolvimento.             | Requer diretório de assets no deploy.               |

**Decisão**: embed.FS para produção. Durante desenvolvimento, usar flag
`-dev` para servir de diretório externo (hot-reload CSS/JS).

### T4 — Polling vs Push

| Opção                    | Prós                                              | Contras                                             |
|--------------------------|---------------------------------------------------|-----------------------------------------------------|
| **Push (SSE)**           | Tempo real, sem polling overhead, menor latência. | Conexão permanente, mais estado no servidor.        |
| Polling (HTTP)           | Simples, sem conexão permanente.                  | Latência de até N segundos, maior carga no servidor.|

**Decisão**: Push via SSE. Tempo real é requisito. Polling desperdiçaria
recursos para o cenário de feed contínuo.

### T5 — Vanilla JS vs SPA Framework

| Opção                    | Prós                                              | Contras                                             |
|--------------------------|---------------------------------------------------|-----------------------------------------------------|
| **Vanilla JS**           | Zero dependências, sem build step, carregamento   | Mais código manual, menos produtividade para UIs    |
| (escolhido)              | instantâneo.                                      | complexas.                                          |
| React/Vue/Svelte         | Componentização, estado reativo, ecossistema.     | Build step, dependências npm, complexidade.         |

**Decisão**: Vanilla JS. A UI é simples (poucos componentes, sem roteamento
complexo). O custo de adicionar um bundler e framework não se justifica.

### T6 — DLQ Count via Kafka vs Ignorar

| Opção                    | Prós                                              | Contras                                             |
|--------------------------|---------------------------------------------------|-----------------------------------------------------|
| **Ignorar DLQ count**    | Simplicidade, sem dependência de Kafka na UI.     | Métrica de DLQ não disponível.                      |
| Consultar Kafka          | DLQ count acurado.                                | Complexidade (consumidor admin client), falha se    |
|                          |                                                   | Kafka está indisponível.                             |

**Decisão**: DLQ count será obtido via consulta ao offset mais recente do
tópico DLQ usando `kafka.Client` (admin). Se falhar, retorna 0 com log de
warning. A UI não deve depender de Kafka para funcionar.

---

## Decisões Arquiteturais (ADRs)

### ADR-001: SSE para Streaming em Tempo Real

**Contexto**: Necessário transmitir eventos do servidor para o navegador em
tempo real, sem polling.

**Decisão**: Server-Sent Events (SSE).

**Justificativa**: Unidirecional (suficiente), reconexão automática nativa,
implementação simples com `http.Flusher` e `text/event-stream`.

### ADR-002: Redis Pub/Sub como Event Bus

**Contexto**: Consumer e UI precisam compartilhar eventos sem acoplamento
direto de processo.

**Decisão**: Redis Pub/Sub (canal `payment:events`).

**Justificativa**: Redis já é dependência do consumer. Pub/Sub é leve e
permite que UI e consumer rodem em processos separados. Alternativa de
canal em memória disponível para desenvolvimento.

### ADR-003: Sem Autenticação

**Contexto**: Ferramenta de desenvolvimento/debug.

**Decisão**: Sem autenticação. Acesso livre na porta configurada.

**Justificativa**: Simplifica desenvolvimento. Para produção, recomenda-se
não expor a porta publicamente ou adicionar proxy reverso com autenticação.

### ADR-004: embed.FS para Assets e Documentação

**Contexto**: Assets estáticos (HTML, CSS, JS) e documentação do projeto
(`.html`, imagens, diagramas) precisam ser servidos sem dependência de
sistema de arquivos externo.

**Decisão**: `embed.FS` para embutir assets e documentação no binário. A
documentação é copiada de `/docs/` para `internal/ui/static/docs/` durante
o build (via `Makefile` ou `Dockerfile`) antes da compilação, sendo
automaticamente incluída pelo `//go:embed static/*`.

**Justificativa**: Single binary, deploy simples, sem dependência de
diretório externo. A documentação se beneficia do mesmo mecanismo de `embed`
já existente, sem necessidade de nova diretiva ou código adicional. A
cópia pré-build é intencional para manter a fonte da documentação em `/docs/`
(separada dos assets da UI) e evitar misturar responsabilidades.

---
