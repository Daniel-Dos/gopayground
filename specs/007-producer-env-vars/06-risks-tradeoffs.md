# 06 — Riscos e Trade-offs

## Riscos Identificados

### R01 — Regressão no modo "serve" ao mudar defaults das flags

| Atributo       | Descrição |
|----------------|-----------|
| **Descrição**  | Alterar os defaults de `serveFlags` de valores hardcoded (`"8082"`, `"localhost:9092"`, `"payment.events"`) para `""` pode quebrar scripts ou workflows que dependiam do comportamento anterior (ex: `producer serve` sem config.yaml e sem env vars). |
| **Probabilidade** | Média |
| **Impacto**       | Médio — o producer não consegue iniciar se não houver config.yaml ou env vars |
| **Mitigação**     | O `config.yaml` está presente no container (copiado pelo Dockerfile) e em ambiente local (raiz do projeto). Além disso, o docker-compose define env vars. O risco existe apenas se alguém executar `producer serve` fora do Docker sem config.yaml. Nesse caso, uma mensagem de erro clara será exibida. |
| **Plano B**       | Manter os defaults atuais e usar `flagset.Lookup().Value.String() != flagset.Lookup().DefValue` para detectar se a flag foi alterada. Isso evita o problema, mas adiciona complexidade. |

### R02 — Conflito de porta: consumer usa 8080, producer usa 8082

| Atributo       | Descrição |
|----------------|-----------|
| **Descrição**  | O `config.yaml` define `server.port: 8080` (porta do consumer). Se o producer não receber `--port 8082` ou `SERVER_PORT=8082`, ele tentará iniciar na porta 8080, causando conflito se o consumer estiver rodando. |
| **Probabilidade** | Média (fora do Docker, onde ambos rodam na mesma máquina) |
| **Impacto**       | Alto — falha ao iniciar o servidor HTTP |
| **Mitigação**     | O comando no docker-compose inclui `--port 8082`. Em ambiente local, o config.yaml tem `server.port: 8080`, mas o producer deve ter um default de código para 8082 quando nem flag nem env var são fornecidas. |
| **Solução proposta** | Manter o default da flag `--port` como `"8082"` (em vez de `""`). Dessa forma, se ninguém fornecer flag/env/config, o producer usa 8082. **Trade-off:** Isso fere a ordem de precedência ideal (flag default sempre vence). |

### R03 — OpenTelemetry endpoint indisponível no startup

| Atributo       | Descrição |
|----------------|-----------|
| **Descrição**  | O producer inicializa OTel TracerProvider e MeterProvider com conexão gRPC para o endpoint configurado. Se o otel-collector não estiver disponível, a inicialização pode falhar ou bloquear. |
| **Probabilidade** | Média (depende da ordem de startup no docker-compose) |
| **Impacto**       | Alto — producer falha ao iniciar |
| **Mitigação**     | O `pkg/telemetry` usa `WithInsecure()` e tem timeout de 5s. Além disso, a inicialização do OTel no producer ocorre antes de conectar ao Kafka — se falhar, o producer não inicia. Para ser mais resiliente, poderíamos usar um wrapper que tolera falha do OTel (log de warning em vez de fatal), mas isso foge do padrão do consumer. |
| **Decisão**       | Seguir o mesmo padrão do consumer: se OTel falhar, o producer falha. O docker-compose usa `depends_on` para garantir que o otel-collector inicie antes. |

### R04 — Modo "publish" importa pacotes de config/telemetry indiretamente

| Atributo       | Descrição |
|----------------|-----------|
| **Descrição**  | Embora o modo "publish" não chame `config.NewConfig()` nem `telemetry.Init*`, os pacotes estarão linkados no binário. Isso aumenta ligeiramente o tamanho do binário, mas não tem efeito colateral em runtime. |
| **Probabilidade** | Certa |
| **Impacto**       | Baixo — aumento de ~2MB no binário |
| **Mitigação**     | Nenhuma necessária. O link de pacotes Go no binário é normal e não afeta comportamento. |

### R05 — Config `graceful_shutdown_timeout` não é usada pelo producer

| Atributo       | Descrição |
|----------------|-----------|
| **Descrição**  | O `config.yaml` define `server.graceful_shutdown_timeout: 30s`, mas o producer atualmente usa um valor hardcoded de 15s para o shutdown do HTTP e 15s para o shutdown do OTel. O consumer usa `cfg.Server.GracefulShutdownTimeout`. |
| **Probabilidade** | Baixa |
| **Impacto**       | Baixo — o valor hardcoded (15s) é razoável |
| **Mitigação**     | Poderíamos usar `cfg.Server.GracefulShutdownTimeout` em vez de hardcoded, mas isso aumenta o escopo. **Decisão:** Manter hardcoded por ora. |

---

## Trade-offs

### T01 — Defaults vazios vs defaults hardcoded nas flags

| Opção | Prós | Contras |
|-------|------|---------|
| **Defaults vazios (`""`)** | Ordem de precedência correta; env vars e config.yaml funcionam como fonte primária | Quebra scripts que dependem dos defaults hardcoded sem config.yaml |
| **Defaults hardcoded** | Compatibilidade retroativa total | Ordem de precedência incorreta (flag default vence config/env); env vars só funcionam se não houver flag default |
| **Detecção via `flagset`** | Ordem de precedência correta + compatibilidade | Complexidade adicional; mais código para manter |

**Decisão:** Adotar **defaults vazios** para `--brokers` e `--topic`, mas
manter `--port` com default `"8082"` para evitar conflito com consumer.
O risco de falta de config.yaml em ambientes locais é mitigado pela
presença do arquivo e por mensagens de erro claras do Viper.

**Voto final:** Usar defaults vazios para `brokers` e `topic`, e `"8082"`
para `port`. Documentar que `config.yaml` é obrigatório para o modo "serve"
a menos que env vars sejam fornecidas.

### T02 — OTel failure: fatal vs warning

| Opção | Prós | Contras |
|-------|------|---------|
| **Fatal (exit 1)** | Consistência com consumer; falha rápida e visível | Menos resiliente; producer não inicia sem OTel |
| **Warning (continua)** | Mais resiliente; producer funciona sem OTel | Comportamento diferente do consumer; perda de observabilidade silenciosa |

**Decisão:** Seguir o consumer — **fatal**. O docker-compose garante que
o otel-collector esteja disponível. Em ambientes locais sem OTel, o
desenvolvedor pode configurar `OTEL_ENDPOINT` para um collector local
ou tolerar a falha ajustando a config.

### T03 — Producer com estrutura `serveFlags.apply()` vs fusão direta

| Abordagem | Prós | Contras |
|-----------|------|---------|
| **`apply()` separado** | Código explícito, fácil de testar, fácil de modificar | Mais linhas de código |
| **Fusão direta (if/else no runServe)** | Menos código, mais direto | Mistura lógica de config com lógica de inicialização |

**Decisão:** Usar `apply()` separado para manter `runServe` mais limpo
e permitir teste unitário da lógica de mesclagem.

### T04 — Porta do producer: config.yaml (8080) vs flag default (8082)

| Cenário | Porta resultante | Problema |
|---------|------------------|----------|
| `producer serve` (sem config, sem env) | 8080 (vindo do config.yaml) | Conflito com consumer |
| `producer serve --port 8082` | 8082 | OK, mas exige flag explícita |
| `SERVER_PORT=8082 producer serve` e config sem `server.port` | 8082 | OK, mas exige env var |

**Decisão:** Manter a flag `--port` com default `"8082"` para garantir
que o producer sempre use 8082, independente do config.yaml. Isso é uma
exceção à regra de "flag default sobrescreve env", mas é a escolha mais
segura para evitar conflito.

---

## Decisões Arquiteturais (ADR-007)

### ADR-007-001: Producer usa `internal/config` para carregar configurações

**Contexto:** O producer precisa ler configurações de config.yaml e env vars.
O pacote `internal/config` já existe e é usado pelo consumer.

**Decisão:** Reusar `internal/config.NewConfig()` no modo "serve".

**Consequências:**
- Producer ganha suporte a 12-factor sem nova dependência
- Alguns campos do Config (Redis, DynamoDB, Worker) não são usados, mas
  isso não causa problemas
- O producer depende do arquivo `config.yaml` na raiz (ou paths relativos)

### ADR-007-002: Producer inicializa OpenTelemetry no modo "serve"

**Contexto:** O producer não envia traces nem métricas para o collector.

**Decisão:** Inicializar TracerProvider e MeterProvider usando
`pkg/telemetry`, seguindo o mesmo padrão do consumer.

**Consequências:**
- Producer exporta traces e métricas OTLP para o endpoint configurado
- A inicialização segue os mesmos parâmetros (timeout, batch, resource)
- Em caso de falha, o producer não inicia (comportamento consistente)

### ADR-007-003: Modo "publish" permanece sem config e sem OTel

**Contexto:** O modo "publish" é uma CLI transient para desenvolvedores.

**Decisão:** Não alterar o modo "publish". Ele continua usando apenas
flags da stdlib.

**Consequências:**
- Binário único contém ambos os modos
- Modo "publish" não depende de config.yaml nem de OTel
- Tamanho do binário aumenta marginalmente (pacotes linkados)

---

## Histórico de Revisão

| Versão | Data       | Autor     | Descrição           |
|--------|------------|-----------|---------------------|
| 1.0    | 2026-06-06 | Architect | Versão inicial      |
