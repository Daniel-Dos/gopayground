# Onboarding — GoPayground

> Boas-vindas ao time! Este documento foi feito para te ajudar a entender o
> GoPayground do zero. Não se preocupe se alguns termos parecerem novos —
> tudo será explicado ao longo do texto.

---

## Boas-vindas

Você acabou de entrar no time de Payment Platform e vai trabalhar com o
**GoPayground**, um sistema distribuído de processamento de eventos de
pagamento escrito em Go.

Seu primeiro objetivo é entender como o sistema funciona por completo:
como um evento de pagamento "anda" do momento em que é criado até aparecer
na tela de um usuário. Este documento te guia por cada parte.

Não hesite em perguntar se algo não ficar claro. Todo mundo aqui já passou
pela mesma curva de aprendizado.

---

## O que é o GoPayground

O GoPayground é um **ambiente de experimentação prática com sistemas
distribuídos em Go**. O nome "playground" não é à toa: ele foi criado para
simular um sistema real de processamento de pagamentos, mas em um ambiente
controlado que você pode rodar no seu próprio computador com Docker.

Na prática, o sistema:

1. **Recebe** eventos de pagamento (uma compra, um reembolso, etc.)
2. **Valida** se os dados estão corretos
3. **Processa** e **persiste** em dois lugares ao mesmo tempo (Redis e
   DynamoDB)
4. **Notifica** uma interface web em tempo real
5. **Lida com falhas** — retenta operações que falham e isola mensagens
   que não podem ser processadas

Tudo isso usando tecnologias que você vai encontrar em sistemas reais:
Apache Kafka, Redis, DynamoDB e OpenTelemetry.

### Stack tecnológica

| Componente | O que faz | Biblioteca Go |
|---|---|---|
| **Apache Kafka** | Broker de mensagens — o "correio" do sistema. Produtores publicam mensagens, consumidores as leem. | `github.com/IBM/sarama` |
| **Redis** | Cache rápido — guarda status atual dos pagamentos e controla idempotência (evita processar duplicado). | `github.com/redis/go-redis/v9` |
| **DynamoDB** | Banco NoSQL persistente — guarda o histórico completo de cada pagamento. | `github.com/aws/aws-sdk-go-v2` |
| **OpenTelemetry** | Observabilidade — tracing distribuído, métricas e logs estruturados. | `go.opentelemetry.io/otel` |
| **Go 1.26** | Linguagem principal. Tudo é escrito em Go idiomático. | — |

### Os 3 serviços

O GoPayground roda **3 serviços independentes**, cada um em um processo
separado:

| Serviço | Diretório | Porta | Função |
|---|---|---|---|
| **Consumer** | `cmd/consumer` | `8080` | Processa eventos do Kafka, valida, persiste |
| **UI** | `cmd/ui` | `8081` | Interface web — dashboards, SSE, API REST |
| **Producer** | `cmd/producer` | `8082` | Publica eventos no Kafka (CLI ou HTTP) |

---

## Arquitetura em 5 minutos

A arquitetura segue o padrão **Event-Driven** (orientada a eventos). Isso
significa que os serviços não se chamam diretamente — eles se comunicam
através de mensagens no Kafka.

```
 ┌─────────────────────────────────────────────────────────────────────┐
 │                         KAFKA CLUSTER                               │
 │                                                                      │
 │  ┌──────────────────────────────┐    ┌─────────────────────────┐    │
 │  │   payment.events (3 parts)   │    │ payment.events.dlq (1)  │    │
 │  │   topico principal           │    │ Dead Letter Queue       │    │
 │  └────────┬─────────────────────┘    └────────────▲────────────┘    │
 │           │                                       │                 │
 └───────────│───────────────────────────────────────│─────────────────┘
             │                                       │
     ┌───────┴──────────────┐              ┌─────────┴──────────┐
     │    PRODUCER (:8082)  │              │  CONSUMER (:8080)  │
     │  ┌─────────────────┐ │              │  ┌──────────────┐  │
     │  │ CLI (publish)   │ │              │  │ 1. Validate  │  │
     │  │ HTTP (serve)    │─┼──publica─────┼─▶│ 2. Idempot.  │  │
     │  └─────────────────┘ │              │  │ 3. Status    │  │
     └──────────────────────┘              │  │ 4. History   │  │
                                           │  │ 5. Retry/DLQ │  │
                                           │  └───┬────┬────┘  │
                                           └──────│────│───────┘
                                                  │    │
                                      ┌───────────┼────┼───────────┐
                                      │           │    │           │
                                      ▼           ▼    ▼           │
                                ┌──────────┐ ┌──────────┐          │
                                │  REDIS   │ │ DYNAMODB │          │
                                │ - status │ │- history │          │
                                │ - idemp. │ │          │          │
                                └─────┬────┘ └──────────┘          │
                                      │                            │
                          EventBus    │                            │
                      (Redis Pub/Sub) │                            │
                                      ▼                            │
                                ┌──────────┐                       │
                                │  UI      │                       │
                                │ (:8081)  │                       │
                                │ - SSE    │                       │
                                │ - REST   │                       │
                                │ - Dashboard                      │
                                └──────────┘                       │
                                                                   │
 ┌──────────────────────────────────────────────────────────────────┘
 │                        OBSERVABILIDADE
 │   OpenTelemetry Collector → Tracing (Jaeger) → Metrics (Prom)
 │   Logs estruturados (slog + JSON)
 └────────────────────────────────────────────────────────────────────
```

### Como os serviços se conectam

- **Producer → Kafka**: o producer publica mensagens no tópico `payment.events`
- **Kafka → Consumer**: o consumer lê do tópico em um **consumer group**
- **Consumer → Redis**: atualiza status do pagamento e marca idempotência
- **Consumer → DynamoDB**: salva o histórico completo
- **Consumer → Redis Pub/Sub**: publica evento no EventBus
- **EventBus → UI → Browser**: a UI recebe do Redis Pub/Sub e envia via
  **SSE** para o navegador
- **UI → Producer → Kafka**: quando você publica um evento pela interface
  web, a UI faz um HTTP POST para o producer, que publica no Kafka

> O Producer é a **única** porta de entrada para o Kafka. Nem a UI nem o
> Consumer publicam diretamente no Kafka (exceto o DLQ, que é uma excessão
> controlada).

---

## Fluxo de um pagamento (passo a passo)

Vamos acompanhar um evento de pagamento do começo ao fim:

### 1. Alguém publica um evento

Um evento de pagamento pode ser publicado de três formas:

**a) Pela interface web (mais comum para testes):**
```
Você → http://localhost:8081 (UI) → POST /api/publish → Producer HTTP (8082) → Kafka
```

**b) Pela CLI do producer:**
```bash
go run ./cmd/producer publish --amount 150.00 --currency BRL --status confirmed
```

**c) Via curl diretamente no producer:**
```bash
curl -X POST http://localhost:8082/publish \
  -H "Content-Type: application/json" \
  -d '{"amount":150,"currency":"BRL","status":"confirmed"}'
```

### 2. O evento chega no Kafka

O producer usa o **Sarama SyncProducer** para publicar a mensagem no
tópico `payment.events`. O Kafka armazena a mensagem em uma das 3
partições do tópico. A partição é escolhida pelo `payment_id` — assim,
todo evento com o mesmo `payment_id` vai para a mesma partição (garantindo
ordenação).

### 3. O Consumer recebe a mensagem

O Consumer escuta o tópico `payment.events` usando o consumer group
`payment-consumer-group`. Quando uma mensagem chega, o Consumer:

**a) Adquire um slot do worker pool**
O Consumer tem um semáforo que limita quantas mensagens podem ser
processadas simultaneamente (padrão: 10 workers). Se todos os slots
estiverem ocupados, a mensagem espera.

**b) Valida o payload**
Usa a biblioteca `go-playground/validator` para conferir se:
- `payment_id` é um UUID v4 válido
- `status` é um dos valores permitidos (pending, confirmed, failed, refunded)
- `amount` é maior que zero
- `currency` tem 3 letras maiúsculas (ISO 4217)

Se a validação falhar → a mensagem vai direto para a **DLQ** (Dead Letter
Queue), sem tentar novamente.

**c) Verifica idempotência**
Antes de processar, o Consumer pergunta ao Redis: "este `payment_id` já
foi processado?". Se sim, a mensagem é ignorada (commit sem ação). Se
não, marca no Redis com `SET NX` e TTL de 24h.

> Isso garante que mesmo que o Kafka entregue a mesma mensagem duas vezes
> (comportamento padrão "at-least-once"), ela só será processada uma vez.

**d) Processa com retry**
Tenta executar duas operações:
1. Atualizar o status no Redis (chave `payment:<id>`, TTL 7 dias)
2. Salvar o histórico no DynamoDB (tabela `payment_history`)

Se alguma falhar (ex: Redis fora do ar), o retry tenta novamente com
backoff exponencial: 100ms → 300ms → 900ms (3 tentativas no total).

**e) Se o retry esgotar → DLQ**
Após 3 tentativas falhas, a mensagem vai para o tópico `payment.events.dlq`
com headers que preservam o erro original, o número de tentativas e o
`trace_id` para debugging.

**f) Publica no EventBus**
Se tudo der certo, o Consumer publica o evento no canal `payment:events`
do Redis Pub/Sub. A UI está escutando esse canal.

### 4. A UI recebe e mostra

A UI mantém uma conexão **SSE** (Server-Sent Events) com cada navegador
conectado. Quando um evento chega pelo Redis Pub/Sub:

1. O `EventBus` da UI recebe a mensagem do Redis
2. Distribui para todos os subscribers (cada aba do navegador)
3. O handler SSE escreve no HTTP response: `event: payment\ndata: {...}\n\n`
4. O JavaScript do navegador escuta `eventSource.addEventListener('payment', ...)`
   e atualiza a tela em tempo real

### 5. Você consulta

Além do feed em tempo real, a UI oferece endpoints REST para consultar:
- `GET /api/payments` — lista pagamentos no Redis (com filtros)
- `GET /api/payments/{id}/history` — histórico completo no DynamoDB
- `GET /api/metrics` — métricas agregadas (total, por status, taxa de sucesso)

---

## Primeiros passos

### Pré-requisitos

- **Go 1.26+** — [instalar](https://go.dev/dl/)
- **Docker + Docker Compose** — [instalar](https://docs.docker.com/compose/install/)
- **Git** — para clonar o repositório

### Setup rápido

```bash
# 1. Clone o repositório
git clone https://github.com/Daniel-Dos/gopayground.git
cd gopayground

# 2. Inicie a infraestrutura (Kafka, Redis, DynamoDB, OTel)
make docker-up
# Equivalente a: docker compose up -d

# 3. Aguarde os serviços subirem (cerca de 30s)
docker compose ps

# 4. Em um terminal, inicie o Consumer
make run

# 5. Em outro terminal, inicie a UI
make run-ui

# 6. Em outro terminal, inicie o Producer em modo servidor HTTP
make run-producer-serve

# 7. Abra o navegador em http://localhost:8081
```

### Comandos essenciais (Makefile)

| Comando | O que faz |
|---|---|
| `make test` | Roda todos os testes com race detector |
| `make lint` | Executa golangci-lint |
| `make build` | Compila o consumer |
| `make build-ui` | Compila a UI (copia docs para embed) |
| `make build-producer` | Compila o producer |
| `make run` | Sobe o consumer localmente |
| `make run-ui` | Sobe a UI localmente |
| `make run-producer` | Executa o producer no modo CLI (publish) |
| `make run-producer-serve` | Sobe o producer como servidor HTTP |
| `make docker-up` | Sobe todos os serviços Docker (infra + apps) |
| `make docker-down` | Derruba todos os serviços Docker |

### Publicando seu primeiro evento

Com tudo rodando, abra o navegador em `http://localhost:8081` e:

1. Acesse a página **Producer** (`/producer`)
2. Preencha valor, moeda e clique em "Publish"
3. Veja o evento aparecer em tempo real no dashboard

Ou via terminal:

```bash
# Publicar um evento via curl na UI
curl -X POST http://localhost:8081/api/publish \
  -H "Content-Type: application/json" \
  -d '{"amount": 250.00, "currency": "BRL", "status": "confirmed", "description": "Pedido #67890"}'

# Publicar 10 eventos aleatórios
curl -X POST http://localhost:8081/api/publish/bulk \
  -H "Content-Type: application/json" \
  -d '{"count": 10}'

# Ver métricas
curl http://localhost:8081/api/metrics

# Ver o stream SSE (deixe rodando)
curl -N http://localhost:8081/api/events
```

### Encerrando

```bash
# Ctrl+C em cada terminal para parar os serviços Go
# Depois, pare a infraestrutura:
make docker-down
```

---

## Estrutura do código

```
.
├── cmd/                        # Entrypoints (main de cada serviço)
│   ├── consumer/main.go        #   Consumer: inicia Kafka + workers
│   ├── producer/main.go        #   Producer: CLI + HTTP server
│   └── ui/main.go              #   UI: servidor web
│
├── internal/                   # Código interno (não importável externamente)
│   ├── config/config.go        #   Config via Viper (YAML + env vars)
│   ├── consumer/handler.go     #   Lógica central de processamento
│   ├── models/payment.go       #   PaymentEvent, PaymentStatus, PaymentHistory
│   ├── validator/validator.go  #   Validação de payload
│   ├── idempotency/redis.go    #   Idempotência via Redis SET NX
│   ├── status/redis.go         #   Status updater (Redis HSET)
│   ├── history/dynamodb.go     #   Histórico no DynamoDB
│   ├── retry/handler.go        #   Retry com backoff exponencial
│   ├── dlq/producer.go         #   Dead Letter Queue (publica no Kafka)
│   ├── events/                 #   EventPublisher (Redis Pub/Sub)
│   │   ├── publisher.go        #     Interface Publisher
│   │   └── redis_publisher.go  #     Implementação Redis
│   ├── kafka/provider.go       #   Configuração Sarama (consumer + producer)
│   ├── provider/               #   Clients centralizados
│   │   ├── redis.go            #     Factory do Redis client
│   │   └── dynamodb.go         #     Factory do DynamoDB client
│   ├── producer/producer.go    #   Lógica de publicação reusada
│   └── ui/                     #   Servidor web
│       ├── server.go           #     Setup do HTTP server + middlewares
│       ├── handlers.go         #     Handlers REST + SSE
│       ├── events.go           #     EventBus (Redis Pub/Sub subscriber)
│       └── static/             #     Frontend (HTML, CSS, JS embutidos)
│           ├── index.html      #       Dashboard principal
│           ├── dashboard.html  #       Dashboard gráfico
│           ├── producer.html   #       Página de publicação manual
│           ├── app.js          #       JavaScript principal
│           └── styles.css      #       Estilos
│
├── pkg/                        # Pacotes potencialmente reutilizáveis
│   ├── telemetry/telemetry.go  #   OpenTelemetry (tracer + meter provider)
│   └── model/user.go           #   Modelo User (não utilizado atualmente)
│
├── specs/                      # Especificações SDD das features
│   ├── 0001-kafka-payment-consumer/  # Spec do Consumer
│   ├── 0002-payment-ui/              # Spec da UI
│   └── 0003-cli-producer/            # Spec do Producer
│
├── adrs/                       # Architecture Decision Records (13 ADRs)
│   ├── ADR-001-kafka-message-broker.md
│   ├── ADR-009-worker-pool-concurrency.md
│   └── ... (veja lista completa no README)
│
├── docs/                       # Documentação
│   ├── architecture.md         #   Arquitetura detalhada
│   ├── onboarding.md           #   Este arquivo
│   ├── ui.md                   #   Documentação da UI
│   ├── producer.md             #   Documentação do Producer
│   ├── observability.md        #   Stack OTel
│   ├── setup.md                #   Guia de setup
│   ├── features/               #   Docs das features
│   └── diagrams/               #   Diagramas (JPG + .excalidraw)
│
├── config.yaml                 # Configuração central
├── docker-compose.yml          # Stack completa
├── Makefile                    # Comandos de build/test/run
├── go.mod / go.sum             # Dependências Go
└── AGENTS.md                   # Instruções para agentes de IA
```

### Tour guiado pelos diretórios

**`cmd/`** — Cada subdiretório tem um `main.go` que é o ponto de entrada
de um binário independente. É aqui que as dependências são "coladas":
config, Redis, DynamoDB, Kafka, etc.

**`internal/`** — Código do domínio, organizado por responsabilidade. A
convenção Go é que pacotes em `internal/` só podem ser importados por
código dentro do módulo — ninguém de fora consegue importar.

**`specs/`** — Especificações no formato SDD (Spec Driven Development).
Cada feature tem uma pasta com 7 arquivos: contexto, requisitos, design,
plano de implementação, checklist de validação, riscos e hardening.

**`adrs/`** — Decision Records. Cada ADR documenta uma decisão
arquitetural importante: por que escolhemos Kafka em vez de RabbitMQ,
por que usamos SSE em vez de WebSocket, etc.

**`docs/`** — Documentação em Markdown. Complementa o README com
detalhes sobre arquitetura, setup, UI e producer.

---

## Como contribuir (fluxo SDD)

O GoPayground segue **Spec Driven Development (SDD)**. Toda feature ou
mudança significativa segue um fluxo rigoroso em etapas, cada uma
executada por um papel específico.

### Ciclo de vida de uma feature

```
1. Architect
   ↓  Cria a spec em /specs/<id-feature>/
2. Senior Engineer
   ↓  Implementa o código seguindo a spec
3. AI Engineering (se aplicável)
   ↓  Implementa pipelines de IA
4. Hardening Engineer
   ↓  Valida resiliência, concorrência, segurança
5. Reviewer
   ↓  Revisa aderência à spec e qualidade
6. Technical Writer
   ↓  Documenta o que foi construído
7. Docker Specialist
   ↓  Cria/mantém Dockerfiles e docker-compose
8. GitHub Specialist
   ↓  Configura CI/CD, PRs, releases
9. Frontend Engineer (se aplicável)
   ↓  Interfaces web
10. Dashboard & Report Specialist (se aplicável)
    ↓  Relatórios HTML, métricas
```

### Estrutura de uma spec

Cada feature tem uma pasta numerada em `specs/`:

```text
specs/0003-cli-producer/
├── 01-context.md           # Contexto de negócio e problema
├── 02-requirements.md      # Requisitos funcionais e não-funcionais
├── 03-design.md            # Design da solução
├── 04-implementation-plan.md  # Plano de implementação
├── 05-validation-checklist.md # Checklist de validação
├── 06-risks-tradeoffs.md   # Riscos e tradeoffs
└── 07-hardening.md         # Considerações de segurança
```

### Como criar uma nova feature

1. Crie uma pasta em `specs/` com o próximo número disponível
2. Escreva os arquivos de spec (começando pelo contexto)
3. Suba um PR com a spec para revisão do Architect
4. Após aprovação, a implementação segue o fluxo SDD

### Convenções de código

- **Idiomático Go**: interfaces pequenas, composição, erros com
  `fmt.Errorf("%w", err)`
- **Context.Context**: obrigatório em operações bloqueantes (Redis,
  DynamoDB, Kafka)
- **Testes table-driven**: testes com subtests (`t.Run`) e cenários em
  tabela
- **Race detector**: todos os testes rodam com `-race`
- **Logs estruturados**: usar `log/slog` com atributos, não `fmt.Println`
- **Simplicidade primeiro**: preferir solução simples sobre complexa

---

## Glossário

### Kafka

- **Topic (tópico)**: um canal nomeado onde mensagens são publicadas. O
  GoPayground tem dois tópicos: `payment.events` (principal) e
  `payment.events.dlq` (Dead Letter Queue).
- **Partition (partição)**: uma divisão do tópico. Mensagens com a mesma
  chave vão para a mesma partição, garantindo ordenação. O tópico
  `payment.events` tem 3 partições.
- **Consumer Group**: um grupo de consumidores que divide o trabalho de
  ler um tópico. Cada partição é atribuída a um consumidor do grupo.
  O grupo do GoPayground é `payment-consumer-group`.
- **Offset**: um número sequencial que identifica a posição de uma
  mensagem dentro de uma partição. O Kafka mantém qual offset cada
  consumer group já processou.
- **At-least-once**: garantia de entrega — a mensagem é entregue pelo
  menos uma vez, mas pode ser entregue mais de uma vez. Por isso
  precisamos de idempotência.
- **SyncProducer**: produtor que espera a confirmação do Kafka antes de
  retornar. Mais lento mas mais seguro que o AsyncProducer.

### DLQ (Dead Letter Queue)

- **DLQ**: um tópico separado para mensagens que não puderam ser
  processadas. No GoPayground, vai para a DLQ: (1) payload inválido
  (falha imediata) ou (2) retry exaustado (falha após 3 tentativas).
- **Headers DLQ**: metadados preservados na mensagem da DLQ —
  `original_topic`, `original_partition`, `original_offset`,
  `last_error`, `trace_id`.

### SSE (Server-Sent Events)

- **SSE**: tecnologia que permite ao servidor enviar eventos para o
  navegador em tempo real, usando uma conexão HTTP contínua. Diferente
  de WebSocket, SSE é unidirecional (servidor → cliente).
- **EventSource**: API JavaScript nativa dos navegadores para consumir
  SSE. Reconecta automaticamente em caso de queda.
- **Heartbeat**: mensagem enviada a cada 30s para manter a conexão viva.

### Idempotência

- **Idempotência**: propriedade que garante que processar a mesma
  mensagem duas vezes produz o mesmo resultado que processar uma vez.
  No GoPayground, usamos Redis com `SET NX` (set if not exists) e TTL
  de 24h.
- **SET NX**: comando Redis que só define uma chave se ela não existir.
  Perfeito para controle de concorrência.

### Redis

- **Pub/Sub**: padrão publish/subscribe do Redis. Um publisher envia
  mensagem para um canal, e todos os subscribers desse canal recebem.
- **HSET**: comando Redis para definir campos em um hash. Usado para
  guardar status do pagamento: `payment:<id>` com campos `status` e
  `updated_at`.
- **TTL (Time To Live)**: tempo de vida de uma chave no Redis. Após
  expirar, a chave é automaticamente removida.

### DynamoDB

- **Tabela**: `payment_history`. Cada registro é um evento de pagamento
  completo.
- **Query**: operação que busca itens pela chave primária
  (`payment_id`).

### Observabilidade

- **OpenTelemetry (OTel)**: framework de observabilidade que unifica
  tracing, métricas e logs.
- **Span**: uma unidade de trabalho no tracing distribuído. Cada
  mensagem processada gera um span.
- **OTLP**: protocolo usado para exportar dados OTel para o Collector.
- **Trace ID**: identificador único que correlaciona todos os spans de
  uma mesma operação.

### Infraestrutura

- **Docker Compose**: orquestração local. Define todos os serviços
  (Kafka, Redis, DynamoDB, OTel Collector, Consumer, Producer, UI) em
  um único arquivo YAML.
- **Floci**: substituto local do DynamoDB (compatível com a API AWS).
- **OTel Collector**: serviço que recebe dados de telemetria, processa
  (batch, filtro) e exporta para sistemas como Jaeger e Prometheus.

---

## Onde buscar ajuda

| O que você precisa | Onde encontrar |
|---|---|
| Como o sistema funciona | `docs/architecture.md` — arquitetura detalhada |
| Configuração e setup | `docs/setup.md` — guia de configuração |
| Documentação da UI | `docs/ui.md` — endpoints, páginas, exemplos curl |
| Documentação do Producer | `docs/producer.md` — CLI e HTTP |
| Stack de observabilidade | `docs/observability.md` — OTel, logs, métricas |
| Decisões arquiteturais | `adrs/` — 13 ADRs documentando cada escolha |
| Specs de features | `specs/` — especificações completas |
| Diagramas visuais | `docs/diagrams/` — arquivos .excalidraw e JPG |
| README principal | `README.md` — visão geral, comandos, estrutura |
| Código-fonte | `internal/` — cada pacote tem sua responsabilidade |
| Este documento | `docs/onboarding.md` — visão geral para novos membros |

### ADRs recomendados para começar

Se você quer entender o *porquê* das decisões, leia estes ADRs primeiro:

| ADR | Título | O que explica |
|---|---|---|
| 001 | Apache Kafka como Message Broker | Por que Kafka? |
| 002 | Redis para Idempotência e Status | Por que Redis para cache? |
| 003 | DynamoDB para Histórico | Por que DynamoDB? |
| 004 | Retry Síncrono com Backoff | Como lidamos com falhas |
| 005 | SSE para Streaming | Como funciona o tempo real |
| 009 | Worker Pool com Semáforo | Controle de concorrência |
| 010 | DLQ Estratégia | O que acontece com falhas |
| 013 | UI publica via Producer HTTP | Por que a UI não publica direto no Kafka |

### Dicas finais

1. **Leia os ADRs** antes de sugerir mudanças arquiteturais — provavelmente
   a decisão já foi discutida e documentada.
2. **Sempre rode os testes** antes de abrir um PR: `make test`.
3. **Use o race detector** — o GoPayground é um sistema concorrente e race
   conditions podem acontecer.
4. **Não tenha medo de perguntar** — todo mundo no time já ficou confuso
   com Kafka no começo.
5. **Acompanhe o fluxo completo**: publique um evento pela UI, veja no
   dashboard, consulte no Redis e no DynamoDB. Entender o caminho completo
   é a melhor forma de aprender.

---

> Bem-vindo ao time! 🎉
