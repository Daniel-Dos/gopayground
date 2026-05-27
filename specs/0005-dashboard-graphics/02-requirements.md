# 02 — Requisitos

## Requisitos Funcionais (RF)

### RF01 — Página de Dashboard Gráfica
O sistema deve servir uma página HTML em `GET /dashboard` que exibe gráficos e métricas visuais do processamento de pagamentos.

### RF02 — Donut Chart de Distribuição por Status
A página deve renderizar um gráfico **donut** (rosquinha) usando Canvas API que exibe a distribuição de pagamentos por status:
- `pending` (amarelo, `#fbbf24`)
- `confirmed` (verde, `#34d399`)
- `failed` (vermelho, `#ef4444`)
- `refunded` (azul, `#60a5fa`)
- Deve exibir o percentual e a contagem absoluta de cada segmento
- Deve animar a abertura (animação de preenchimento circular ao carregar)
- Deve incluir uma legenda abaixo ou ao lado do gráfico

### RF03 — Gauge Semi-Circular de Taxa de Sucesso
A página deve renderizar um **gauge** (medidor) semi-circular usando Canvas API que exibe a taxa de sucesso:
- Escala de 0% a 100%
- Arco preenchido proporcional à taxa atual
- Valor percentual exibido no centro do gauge
- Cores por faixa: vermelho (< 70%), amarelo (70-90%), verde (> 90%)
- Animação de preenchimento ao carregar

### RF04 — Bar Chart de Pagamentos por Hora
A página deve renderizar um **gráfico de barras** (histograma) usando Canvas API que mostra pagamentos processados por hora:
- 24 barras representando as últimas 24 horas
- Altura da barra proporcional ao volume de pagamentos naquela hora
- Rótulos do eixo X com horários (ex: "14:00", "15:00")
- Rótulo do eixo Y com contagem
- Dados gerados/transformados localmente a partir dos dados de métrica disponíveis
- Cor das barras usando o gradiente accent (`#7c5cfc` a `#a78bfa`)

### RF05 — Contador de Eventos da Sessão
A página deve exibir um contador em tempo real de eventos processados durante a sessão atual:
- Incrementa a cada evento recebido via SSE
- Exibe o total com animação de transição numérica
- Estilo visual destacado (card com gradiente accent)

### RF06 — Indicador de Conexão SSE
A página deve exibir o status da conexão SSE (conectado/desconectado) usando o mesmo padrão visual da dashboard existente.

### RF07 — Polling Automático a cada 10s
A página deve executar polling de `GET /api/metrics` a cada 10 segundos para atualizar todos os gráficos.

### RF08 — Atualização Instantânea via SSE
Quando um evento SSE do tipo `payment` é recebido, a página deve:
- Incrementar o contador de eventos da sessão
- Acionar refresh das métricas (polling antecipado)
- Exibir um flash visual indicando atualização

### RF09 — Layout Responsivo
Em viewports menores que 768px, os gráficos devem empilhar-se verticalmente (um abaixo do outro) em vez de lado a lado.

### RF10 — Estado de Carregamento
Enquanto as métricas não são carregadas, a página deve exibir um estado de carregamento (skeleton/spinner) para cada gráfico.

### RF11 — Estado de Erro
Se a API `/api/metrics` falhar, a página deve:
- Exibir uma mensagem de erro amigável no lugar dos gráficos
- Continuar tentando no próximo ciclo de polling
- Não quebrar a experiência geral

### RF12 — Acessibilidade via Teclado
- Todos os gráficos devem ter `role="img"` e `aria-label` descrevendo o conteúdo
- O foco do teclado deve ser gerenciado adequadamente
- Contraste de cores deve seguir WCAG AA mínimo

### RF13 — Navegação Integrada
O header da nova página deve incluir links de navegação para as outras páginas (`/` Dashboard original, `/producer` Producer UI) e destacar a página atual com classe `active`.

---

## Requisitos Não Funcionais (RNF)

### RNF01 — Performance de Renderização
Cada gráfico Canvas deve renderizar em menos de **50ms** (tempo de desenho). A animação de abertura não deve exceder **800ms**.

### RNF02 — Sem Dependências Externas
Zero bibliotecas de terceiros para gráficos. Todo o desenho deve usar **Canvas API** nativa do navegador. CSS e HTML vanilla.

### RNF03 — Consumo de Memória
O JavaScript da página não deve exceder **50KB** (minificado). O uso de memória em tempo de execução não deve exceder **10MB** para dados de gráficos.

### RNF04 — Disponibilidade
A página deve funcionar mesmo que a API de métricas esteja temporariamente indisponível (graceful degradation).

### RNF05 — Aderência ao Design System
Deve usar exclusivamente as variáveis CSS definidas em `style.css` para cores, sombras, bordas e tipografia. Nenhum valor de cor hardcoded fora do sistema de design.

### RNF06 — Compatibilidade
Deve funcionar nos 2 últimos major releases de Chrome, Firefox, Safari e Edge.

### RNF07 — Tempo de Resposta
O polling de métricas não deve exceder **5s** de timeout. Se a API não responder em 5s, a requisição deve ser abortada e tratada como falha.

### RNF08 — Consistência Visual
O tamanho e espaçamento dos gráficos deve seguir a grade do design system existente (múltiplos de 8px para padding, border-radius consistente).

---

## Mensagens e Rótulos (pt-BR)

| Contexto                   | Texto                               |
|----------------------------|-------------------------------------|
| Título da página           | Dashboard de Métricas               |
| Título donut chart         | Distribuição por Status             |
| Título gauge               | Taxa de Sucesso                     |
| Título bar chart           | Pagamentos por Hora (24h)           |
| Título contador eventos    | Eventos na Sessão                   |
| Status conectado           | 🟢 Conectado                        |
| Status desconectado        | 🔴 Desconectado                     |
| Estado carregando          | Carregando métricas...              |
| Estado erro                | API de métricas indisponível        |
| Rótulo DLQ                 | DLQ                                 |
| Rótulo total               | Total                               |
| Navegação Dashboard        | 📋 Dashboard                        |
| Navegação Producer         | 📝 Producer                         |
| Navegação Dashboard Gráf.  | 📊 Gráficos                         |
| Badge de tempo real        | Tempo Real                          |
