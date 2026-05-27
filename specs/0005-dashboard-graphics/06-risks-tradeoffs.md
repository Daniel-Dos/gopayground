# 06 — Riscos e Tradeoffs

## Riscos

### R01 — Dados do Bar Chart são Sintéticos

**Risco:** O bar chart de pagamentos por hora (24h) usa dados gerados localmente no navegador a partir das métricas agregadas atuais, não de um histórico real. Isso pode dar uma **falsa impressão de precisão** ao usuário.

**Impacto:** Médio. Operadores podem confiar nos dados do gráfico e tomar decisões baseadas em informações sintéticas.

**Mitigação:**
- Adicionar nota visual no gráfico: "Distribuição estimada baseada nos dados atuais"
- Documentar que é uma visualização demonstrativa
- Planejar endpoint `/api/metrics/historical` em spec futura

### R02 — Performance de Canvas em Dispositivos com Pouca Memória

**Risco:** Dispositivos mobile antigos ou com GPU limitada podem ter dificuldade para renderizar os canvases, especialmente durante animações e redimensionamento.

**Impacto:** Baixo. Canvas 2D é bem suportado até em hardware modesto. O número de elementos desenhados é pequeno.

**Mitigação:**
- Animações opcionais via `prefers-reduced-motion`
- Cleanup adequado de contextos
- Evitar redesenho desnecessário (debounce no resize)

### R03 — Timeout de Polling vs. SSE

**Risco:** Se o polling de 10s e o SSE dispararem refresh simultaneamente, pode haver **duas requisições concorrentes** para `/api/metrics`.

**Impacto:** Baixo. A API é idempotente e o frontend apenas sobrescreve os mesmos dados.

**Mitigação:**
- Usar uma flag `isRefreshing` para evitar chamadas simultâneas
- Abortar requisição anterior se nova for disparada

### R04 — Canvas com devicePixelRatio em Displays com Alto DPI

**Risco:** Em dispositivos com `devicePixelRatio` > 2 (ex: MacBooks, iPhones), o canvas pode consumir 4x-9x mais memória. Com 3 canvases simultâneos, o impacto pode ser perceptível.

**Impacto:** Baixo. 3 canvases de ~400x300px em 3x consomem ~4MB de buffer, ainda aceitável.

**Mitigação:**
- Limitar `devicePixelRatio` a 2 em dispositivos com pouca memória (detectável via `navigator.deviceMemory`)
- Manter tamanhos de canvas modestos

### R05 — Falta de Dados Históricos Reais

**Risco:** O bar chart pode ficar sempre vazio se não houver dados suficientes para gerar distribuição horária significativa.

**Impacto:** Médio. Gráfico vazio não agrega valor. Pode causar confusão.

**Mitigação:**
- Exibir mensagem clara "Aguardando dados históricos..." quando `total_processed` for 0
- Se total > 0 mas pequeno, gerar distribuição com proporção real
- Em versão futura, substituir por endpoint real de histórico

### R06 — Quebra de Layout com Cards de Altura Variável

**Risco:** O donut chart, gauge e counter têm alturas intrínsecas diferentes, o que pode quebrar o grid de 3 colunas se não forem equalizados.

**Impacto:** Baixo. Visualmente perceptível mas não funcional.

**Mitigação:**
- Definir altura mínima fixa para os chart cards no grid
- Usar `align-items: stretch` no grid container

### R07 — Experiência Degradada sem JavaScript

**Risco:** Usuários com JavaScript desabilitado não verão os gráficos.

**Impacto:** Muito baixo. A dashboard inteira depende de JS (como as outras páginas do projeto).

**Mitigação:**
- Adicionar `<noscript>` com mensagem informativa
- Manter consistência com outras páginas (todas requerem JS)

### R08 — Fuga de Memória por Acumulação de Eventos SSE

**Risco:** Em sessões longas, o navegador pode acumular muitos eventos na fila do EventSource, especialmente se o processamento for mais lento que a chegada de eventos.

**Impacto:** Baixo. O contador da sessão é apenas um número. Não há acúmulo de objetos.

**Mitigação:**
- Contador é inteiro simples — não acumula memória
- O callback SSE é leve e rápido

---

## Tradeoffs

### T01 — Dados Sintéticos vs. Endpoint Real de Histórico

| Opção                          | Prós                                        | Contras                                    |
|--------------------------------|---------------------------------------------|--------------------------------------------|
| ✅ **Dados sintéticos (atual)** | Sem dependência de nova API, implementação imediata | Dados não refletem realidade histórica     |
| Endpoint real de histórico     | Dados precisos, análise real                | Requer nova API, armazenamento de série temporal, maior complexidade |

**Decisão:** Implementar com dados sintéticos agora. Planejar endpoint real como melhoria futura.

### T02 — Canvas API vs. SVG

| Característica                 | Canvas API                     | SVG                            |
|--------------------------------|--------------------------------|--------------------------------|
| Renderização                   | Pixel-based (raster)           | Vector-based                   |
| Animação                       | `requestAnimationFrame`        | CSS animations/SMIL            |
| Acessibilidade                 | `role="img"` + `aria-label`    | Elementos DOM acessíveis       |
| Performance (muchos elementos) | Melhor                         | Pior                           |
| Interatividade                 | Hit detection manual           | Eventos DOM nativos            |
| Nitidez em retina              | `devicePixelRatio` manual      | Automático (vetorial)          |

**Decisão:** Canvas API. Performance superior para animações, controle pixel-level sobre gradientes e arcos, e alinhamento com requisito de gráficos dinâmicos. SVG seria mais simples para acessibilidade, mas Canvas oferece mais controle visual.

### T03 — Polling 10s vs. Apenas SSE

| Opção                          | Prós                                        | Contras                                    |
|--------------------------------|---------------------------------------------|--------------------------------------------|
| ✅ **Polling 10s + SSE**        | Redundância, dados sempre atuais, fallback  | Duas fontes de atualização, mais requisições |
| Apenas SSE                     | Menos requisições, tempo real puro          | Perda de dados se SSE falha, sem heartbeat de dados |

**Decisão:** Manter ambos. SSE para instantaneidade, polling como fallback e sincronização periódica.

### T04 — Gráfico 3D vs. 2D

| Opção                          | Prós                                        | Contras                                    |
|--------------------------------|---------------------------------------------|--------------------------------------------|
| ✅ **Canvas 2D**                | Simples, rápido, consiste com requisitos    | Menos "impressionante" visualmente         |
| WebGL/3D                       | Visualmente impactante                      | Complexidade alta, maior consumo de GPU    |

**Decisão:** Canvas 2D. Atende aos requisitos, mais simples de implementar e manter.

### T05 — Estilos Inline vs. no style.css

| Opção                          | Prós                                        | Contras                                    |
|--------------------------------|---------------------------------------------|--------------------------------------------|
| ✅ **CSS no dashboard.html**    | Isolamento, não polui CSS compartilhado     | Duplicação se outros usarem (improvável)   |
| Adicionar ao `style.css`       | Consistência centralizada                   | Arquivo já grande (1547 linhas)            |

**Decisão:** Incluir estilos específicos da dashboard gráfica em bloco `<style>` no `<head>` do `dashboard.html`, reutilizando apenas as variáveis CSS do `style.css`.

### T06 — Donut vs. Pie Chart

| Característica                 | Donut Chart                    | Pie Chart                      |
|--------------------------------|--------------------------------|--------------------------------|
| Legibilidade de proporções     | Similar                         | Similar                        |
| Espaço central para total      | ✅ Sim                          | ❌ Não                         |
| Estética moderna               | ✅ Sim                          | ❌ Clássico                     |

**Decisão:** Donut. O espaço central permite exibir o total de pagamentos, adicionando informação sem ocupar espaço extra.

---

## Dívida Técnica Identificada

| Item                           | Descrição                                      | Prioridade | Quando Pagar                     |
|--------------------------------|------------------------------------------------|------------|----------------------------------|
| Endpoint histórico real        | `/api/metrics/historical` para dados precisos  | Média      | Próxima iteração de métricas     |
| Testes automatizados de canvas | Testar renderização via headless browser       | Baixa      | Quando houver pipeline de UI tests |
| Animações com `prefers-reduced-motion` | Suporte a preferência de movimento reduzido | Baixa | Após implementação inicial |
