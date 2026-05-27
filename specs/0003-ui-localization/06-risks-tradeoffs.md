# 06 — Riscos e Trade-offs

## Riscos

### R01 — Heartbeat Timeout Falso Positivo

**Probabilidade**: Baixa
**Impacto**: Baixo (indicador "Desconectado" por alguns segundos)

**Descrição**: O timeout de 45s pode disparar um falso positivo se o servidor
atrasar o heartbeat por mais de 45s (ex: GC pesado, pausa de rede, servidor
sobrecarregado).

**Mitigação**:
- O timeout de 45s é 1.5× o intervalo de heartbeat do servidor (30s),
  oferecendo margem para jitter e pausas
- Assim que o próximo evento/heartbeat chegar, o status volta a "🟢 Conectado"
- Navegadores throttlam `setTimeout` em background tabs, mas 45s é tolerante

### R02 — `onopen` Não Disparar em Alguns Navegadores

**Probabilidade**: Média
**Impacto**: Médio (status preso em "Desconectado")

**Descrição**: Em navegadores Chromium, `onopen` pode não disparar de forma
confiável após reconexão automática do EventSource.

**Mitigação**:
- O heartbeat timeout detecta conexão ativa mesmo sem `onopen`
- Se dados chegam (heartbeat ou evento), `resetHeartbeatTimeout()` é chamado
  e o status muda para "🟢 Conectado"
- O watchdog independe de `onopen` para manter o status correto

### R03 — Timeouts Órfãos por Reset Incorreto

**Probabilidade**: Baixa
**Impacto**: Médio (múltiplos timers simultâneos, comportamento imprevisível)

**Descrição**: Se `clearTimeout` não for chamado antes de criar um novo timer,
múltiplos timeouts podem acumular, causando comportamentos inconsistentes.

**Mitigação**:
- `scheduleHeartbeatTimeout()` sempre chama `clearTimeout(heartbeatTimer)`
  antes de criar um novo timer
- A função é chamada exclusivamente por `resetHeartbeatTimeout()`, que também
  chama `scheduleHeartbeatTimeout()` após atualizar o status

### R04 — Background Tab Throttling

**Probabilidade**: Alta
**Impacto**: Muito Baixo (atraso máximo de ~1s na detecção)

**Descrição**: Navegadores modernos throttlam `setTimeout` em abas em background.
Chrome: mínimo 1s após 5min em background. Firefox: mínimo 1s.

**Mitigação**:
- Com timeout de 45s, até mesmo com throttling de 1s, o atraso máximo é
  desprezível (46s vs 45s)
- O heartbeat do servidor a cada 30s mantém o timer resetado, então o
  throttling só afeta o momento da detecção de queda, não a operação normal

### R05 — Status Mapping Inconsistente

**Probabilidade**: Baixa
**Impacto**: Baixo (status exibido em inglês em algum lugar)

**Descrição**: Se um novo status for adicionado no backend (ex: `cancelled`)
mas não estiver no `statusMap`, ele será exibido em inglês.

**Mitigação**:
- `getStatusLabel()` tem fallback: retorna o valor original se não encontrar
  no mapa (graceful degradation)
- Novo status é exibido em inglês até o mapa ser atualizado
- O mapeamento é centralizado em um único local, facilitando manutenção

### R06 — Enhancement Scripts com MutationObserver Podem Causar Vazamento

**Probabilidade**: Muito Baixa
**Impacto**: Baixo (consumo extra de memória)

**Descrição**: Os MutationObservers no enhancement script observam o DOM
continuamente. Se não forem desconectados, podem manter referências que
impedem garbage collection.

**Mitigação**:
- Os observers observam elementos que existem por toda a vida da página
  (não há navegação SPA que os remova)
- O escopo é limitado: `#metrics`, `#feed-container`, `#payments-table tbody`
- Em recarga de página (único ciclo de vida), os observers são destruídos
  naturalmente

---

## Trade-offs

### T01 — Watchdog Baseado em Timeout vs. Eventos Nativos do EventSource

| Opção | Prós | Contras |
|-------|------|---------|
| **Heartbeat timeout (escolhido)** | Independente de comportamento inconsistente do navegador, detecta conexões half-open, funciona em todos os navegadores | Timer pode ter falsos positivos, 45s de latência na detecção |
| `onopen`/`onerror` apenas | Detecção instantânea, zero código extra | Comportamento varia entre navegadores, não detecta half-open TCP, status travado em "Desconectado" |

**Decisão**: Heartbeat timeout. A confiabilidade supera a latência de 45s na
detecção.

### T02 — Mapeamento Centralizado vs. Tradução Dispersa

| Opção | Prós | Contras |
|-------|------|---------|
| **Mapa centralizado (escolhido)** | Único ponto de manutenção, consistência garantida, fácil de estender | Requer que todo código use o mapa (disciplina) |
| Tradução inline em cada chamada | Simples, sem abstração | Duplicação, inconsistências, difícil de manter |

**Decisão**: Mapa centralizado `STATUS_LABELS` + helper `getStatusLabel()`.

### T03 — Emojis Literais vs. Unicode Escapes

| Opção | Prós | Contras |
|-------|------|---------|
| **Emojis literais (escolhido)** | Legível no código-fonte, fácil de editar | Depende de encoding UTF-8 do arquivo (padrão em projetos Go) |
| Escapes Unicode (`\u{1F7E2}`) | Funciona em qualquer encoding | Ilegível, difícil de manter, propenso a erro |

**Decisão**: Emojis literais. O projeto já usa UTF-8 em todos os arquivos.

### T04 — Enhancement Scripts Via MutationObserver vs. Lógica Direta no app.js

| Opção | Prós | Contras |
|-------|------|---------|
| **Script separado com MutationObserver (escolhido)** | Desacoplado, não polui lógica principal, pode ser desativado sem afetar funcionalidade | Complexidade adicional, performance do MutationObserver |
| Lógica inline no app.js | Simples, sem observers extras | Acopla enhancement visual à lógica de negócio, mais difícil de remover |

**Decisão**: Script separado no final do `<body>` do `index.html`. Mantém
`app.js` focado em dados e estado, enquanto o enhancement lida apenas com
apresentação visual.

### T05 — Alterar CSS vs. Apenas HTML/JS

| Opção | Prós | Contras |
|-------|------|---------|
| **Apenas HTML/JS (escolhido)** | Zero risco de quebrar layout existente, sem regressão visual | Enhancement via JS é mais "pesado" que CSS puro |
| CSS adicional | Enhancement visual mais performático | Risco de conflito com classes existentes, mais difícil de reverter |

**Decisão**: Apenas HTML/JS. O enhancement visual é obtido via MutationObserver
que adiciona elementos DOM (ícones, timestamps), sem alterar CSS.

---

## Impacto em Specs Anteriores

| Spec | Impacto |
|------|---------|
| 0001 (kafka consumer) | Nenhum |
| 0002 (payment UI dashboard) | Base para as alterações (os arquivos localizados são da dashboard) |
| 0003 (CLI producer) | Nenhum |
| 0004 (producer UI) | Leve: a localização pt-BR estabelece padrão de idioma que a Producer UI deve seguir |
