# 05 — Checklist de Validação

## 1. Rota e Servir Página

| # | Item                                                          | Critério de Aceitação                                       | Status |
|---|---------------------------------------------------------------|-------------------------------------------------------------|--------|
| 1 | Rota `/dashboard` registrada no mux                           | `GET /dashboard` retorna 200 com HTML                       | ⬜     |
| 2 | Arquivo `dashboard.html` existe em `static/`                  | Arquivo presente e embedado via `//go:embed static/*`       | ⬜     |
| 3 | Content-Type correto                                          | `Content-Type: text/html; charset=utf-8`                    | ⬜     |
| 4 | Página 404 se arquivo não encontrado                          | Se `static/dashboard.html` não existir, retorna 404         | ⬜     |
| 5 | Link de navegação em `index.html` para `/dashboard`           | Nav em index.html tem link para Gráficos                    | ⬜     |
| 6 | Link de navegação em `producer.html` para `/dashboard`        | Nav em producer.html tem link para Gráficos                 | ⬜     |
| 7 | Link `/dashboard` aparece como `active` na página             | Classe `nav-link active` aplicada ao link na dashboard      | ⬜     |

## 2. Layout e Responsividade

| # | Item                                                          | Critério de Aceitação                                       | Status |
|---|---------------------------------------------------------------|-------------------------------------------------------------|--------|
| 8 | Grid de 3 colunas em desktop (> 768px)                        | Donut, gauge, counter lado a lado                           | ⬜     |
| 9 | Grid de 1 coluna em mobile (< 768px)                          | Gráficos empilhados verticalmente                           | ⬜     |
| 10 | Bar chart ocupa largura total                                 | `grid-column: 1 / -1`                                       | ⬜     |
| 11 | DLQ section ocupa largura total                               | `grid-column: 1 / -1`                                       | ⬜     |
| 12 | Resize: gráficos redimensionam corretamente                   | Redesenhar ao redimensionar janela                          | ⬜     |
| 13 | Toque/mobile: todos os elementos interativos funcionam        | Botões e links funcionam em touch                           | ⬜     |

## 3. Donut Chart

| # | Item                                                          | Critério de Aceitação                                       | Status |
|---|---------------------------------------------------------------|-------------------------------------------------------------|--------|
| 14 | Renderiza na Canvas API                                       | `<canvas id="donut-canvas">`                                | ⬜     |
| 15 | Exibe 4 segmentos (pending, confirmed, failed, refunded)      | Cada status com seu arco e cor                               | ⬜     |
| 16 | Cores corretas por status                                     | pending=#fbbf24, confirmed=#34d399, failed=#ef4444, refunded=#60a5fa | ⬜ |
| 17 | Percentual exibido no centro                                  | Texto central com total de pagamentos                       | ⬜     |
| 18 | Legenda abaixo com cores e contagens                          | Cada status listado com cor, nome e valor                   | ⬜     |
| 19 | Animação de abertura                                          | Arcos preenchem progressivamente em 800ms                   | ⬜     |
| 20 | Tratamento de dados vazios                                    | Se total = 0, exibe círculo vazio com "Sem dados"           | ⬜     |
| 21 | Acessibilidade                                                | `role="img"` + `aria-label` descritivo                       | ⬜     |

## 4. Gauge Chart

| # | Item                                                          | Critério de Aceitação                                       | Status |
|---|---------------------------------------------------------------|-------------------------------------------------------------|--------|
| 22 | Renderiza na Canvas API                                       | `<canvas id="gauge-canvas">`                                | ⬜     |
| 23 | Formato semi-circular                                         | Arco de π a 2π (180° a 360°)                                | ⬜     |
| 24 | Arco de fundo exibido                                         | Arco cinza/muted de 0 a 100%                                | ⬜     |
| 25 | Arco preenchido proporcional à taxa                           | Ex: 71.4% → ~71.4% do arco preenchido                       | ⬜     |
| 26 | Cor muda conforme threshold                                   | < 70% = vermelho, 70-90% = amarelo, > 90% = verde           | ⬜     |
| 27 | Percentual exibido no centro                                  | Número grande centralizado (ex: "71.4%")                    | ⬜     |
| 28 | Tick marks no arco                                            | Marcadores em 0%, 25%, 50%, 75%, 100%                       | ⬜     |
| 29 | Animação de preenchimento                                     | Arco preenche com ease-out em 800ms                         | ⬜     |
| 30 | Acessibilidade                                                | `role="img"` + `aria-label` com valor atual                  | ⬜     |

## 5. Bar Chart

| # | Item                                                          | Critério de Aceitação                                       | Status |
|---|---------------------------------------------------------------|-------------------------------------------------------------|--------|
| 31 | Renderiza na Canvas API                                       | `<canvas id="bar-canvas">`                                  | ⬜     |
| 32 | 24 barras visíveis                                            | Uma barra para cada hora das últimas 24h                    | ⬜     |
| 33 | Rótulos eixo X a cada 3h                                      | Labels: 0:00, 3:00, 6:00, 9:00, 12:00, 15:00, 18:00, 21:00| ⬜     |
| 34 | Rótulo eixo Y com valores                                     | Valores numéricos no eixo Y                                 | ⬜     |
| 35 | Gradiente accent nas barras                                   | `#7c5cfc` → `#a78bfa`                                       | ⬜     |
| 36 | Linhas de grade horizontais sutis                             | Gridlines em `#2d3140`                                      | ⬜     |
| 37 | Altura máxima respeita o canvas                               | Barras escalonadas para caber no canvas                     | ⬜     |
| 38 | Dados gerados consistentes                                    | Distribuição minimamente realista (horário comercial > noturno) | ⬜ |
| 39 | Tratamento de dados vazios                                    | Se total = 0, exibe placeholder "Aguardando dados..."       | ⬜     |
| 40 | Acessibilidade                                                | `role="img"` + `aria-label` descritivo                       | ⬜     |

## 6. Session Counter

| # | Item                                                          | Critério de Aceitação                                       | Status |
|---|---------------------------------------------------------------|-------------------------------------------------------------|--------|
| 41 | Exibe contador de eventos                                     | Número grande com label "eventos desde que abriu"           | ⬜     |
| 42 | Incrementa a cada evento SSE                                  | Cada evento `payment` incrementa o contador                 | ⬜     |
| 43 | Animação de pulo ao incrementar                               | Scale 1 → 1.15 → 1 em 150ms                                | ⬜     |
| 44 | Persiste na sessão                                            | Não zera com refresh de métricas (apenas com reload)        | ⬜     |

## 7. DLQ Counter

| # | Item                                                          | Critério de Aceitação                                       | Status |
|---|---------------------------------------------------------------|-------------------------------------------------------------|--------|
| 45 | Exibe valor de `dlq_count` da API                             | Número com label "mensagens não processadas"                | ⬜     |
| 46 | Atualiza a cada polling                                       | Valor reflete o último dado da API                          | ⬜     |
| 47 | Destaque visual se DLQ > 0                                    | Estilo warning (amarelo) quando > 0                         | ⬜     |

## 8. Polling e Atualização

| # | Item                                                          | Critério de Aceitação                                       | Status |
|---|---------------------------------------------------------------|-------------------------------------------------------------|--------|
| 48 | Polling a cada 10s                                            | `setInterval(fetchMetrics, 10000)`                          | ⬜     |
| 49 | Timeout de 5s por requisição                                  | `AbortController` com timeout de 5000ms                     | ⬜     |
| 50 | Atualização visual imediata                                   | Gráficos redesenhados após cada resposta                    | ⬜     |
| 51 | Não acumula requisições                                       | Se resposta demorar > 10s, próxima requisição não espera    | ⬜     |
| 52 | Refresh antecipado via SSE                                    | Evento SSE dispara refresh imediato das métricas            | ⬜     |

## 9. SSE

| # | Item                                                          | Critério de Aceitação                                       | Status |
|---|---------------------------------------------------------------|-------------------------------------------------------------|--------|
| 53 | Conecta em `/api/events`                                      | `new EventSource('/api/events')`                            | ⬜     |
| 54 | Escuta evento `payment`                                       | Incrementa contador e refresha métricas                     | ⬜     |
| 55 | Escuta evento `heartbeat`                                     | Marca como conectado                                        | ⬜     |
| 56 | Reconexão automática                                          | EventSource reconecta em caso de falha                      | ⬜     |
| 57 | Timeout de heartbeat (45s)                                    | Se sem heartbeat por 45s, marca como desconectado           | ⬜     |
| 58 | Indicador visual de conexão                                   | "🟢 Conectado" / "🔴 Desconectado"                          | ⬜     |

## 10. Estados de Loading e Erro

| # | Item                                                          | Critério de Aceitação                                       | Status |
|---|---------------------------------------------------------------|-------------------------------------------------------------|--------|
| 59 | Loading overlay visível durante primeira carga                | Overlay "Carregando métricas..." antes dos dados            | ⬜     |
| 60 | Loading overlay some após dados carregados                    | Fade out do overlay                                         | ⬜     |
| 61 | Mensagem de erro se API falhar                                | "API de métricas indisponível" no lugar dos gráficos        | ⬜     |
| 62 | Últimos dados válidos mantidos em caso de erro                | Gráficos não somem — mantém último estado                   | ⬜     |
| 63 | Tentativa contínua mesmo após erro                            | Próximo ciclo de polling continua tentando                  | ⬜     |
| 64 | Fallback se SSE falhar                                        | Polling mantém dados atualizados mesmo sem SSE              | ⬜     |

## 11. Acessibilidade

| # | Item                                                          | Critério de Aceitação                                       | Status |
|---|---------------------------------------------------------------|-------------------------------------------------------------|--------|
| 65 | Navegação por teclado                                         | Todos os links e botões acessíveis via Tab                  | ⬜     |
| 66 | Canvas com `role="img"` e `aria-label`                        | 3 canvas com atributos de acessibilidade                    | ⬜     |
| 67 | Status da conexão com `aria-live="polite"`                    | Leitores de tela anunciam mudanças de conexão               | ⬜     |
| 68 | Contraste de cores WCAG AA                                    | Texto sobre fundo escuro atende contraste mínimo            | ⬜     |
| 69 | Foco visível                                                  | Elementos focáveis têm outline visível                      | ⬜     |

## 12. Performance

| # | Item                                                          | Critério de Aceitação                                       | Status |
|---|---------------------------------------------------------------|-------------------------------------------------------------|--------|
| 70 | Renderização < 50ms por gráfico                               | Cada função de desenho executa em < 50ms                    | ⬜     |
| 71 | Animações < 800ms                                             | Todas as animações completam em até 800ms                   | ⬜     |
| 72 | Memória < 10MB                                                | Uso de memória em runtime não excede 10MB                   | ⬜     |
| 73 | Retina-ready                                                  | Gráficos nítidos em displays HiDPI (`devicePixelRatio`)     | ⬜     |
| 74 | Zero dependências externas                                    | Nenhum CDN ou biblioteca JS carregada                       | ⬜     |
| 75 | JavaScript < 50KB                                             | Tamanho do `dashboard.js` (bruto) < 50KB                    | ⬜     |

## 13. Consistência Visual

| # | Item                                                          | Critério de Aceitação                                       | Status |
|---|---------------------------------------------------------------|-------------------------------------------------------------|--------|
| 76 | Usa variáveis CSS do design system                            | Nenhum valor de cor hardcoded fora das variáveis            | ⬜     |
| 77 | Header consistente com outras páginas                         | Mesmo layout, mesmas classes, mesma altura                  | ⬜     |
| 78 | Fontes consistentes                                           | Plus Jakarta Sans para títulos, DM Sans para corpo          | ⬜     |
| 79 | Border-radius consistente                                     | `--radius-lg` para cards, `--radius` para elementos        | ⬜     |
| 80 | Sombras consistentes                                          | `--shadow-md` para cards, `--shadow-accent` para destaque   | ⬜     |

## 14. Cross-browser

| # | Item                                                          | Critério de Aceitação                                       | Status |
|---|---------------------------------------------------------------|-------------------------------------------------------------|--------|
| 81 | Chrome (últimas 2 versões)                                    | Testado e funcional                                         | ⬜     |
| 82 | Firefox (últimas 2 versões)                                   | Testado e funcional                                         | ⬜     |
| 83 | Safari (últimas 2 versões)                                    | Testado e funcional                                         | ⬜     |
| 84 | Edge (últimas 2 versões)                                      | Testado e funcional                                         | ⬜     |

---

## Checklist de Revisão

| # | Item de Revisão                                              | Responsável | Status |
|---|--------------------------------------------------------------|-------------|--------|
| R1 | Spec aprovada pelo Architect                                 | Architect   | ⬜     |
| R2 | Código segue a spec                                          | Senior Eng  | ⬜     |
| R3 | Hardening validado                                           | Hardening   | ⬜     |
| R4 | Testes de integração passam                                  | Senior Eng  | ⬜     |
| R5 | Revisão de código concluída                                  | Reviewer    | ⬜     |
| R6 | Documentação atualizada                                      | Tech Writer | ⬜     |
