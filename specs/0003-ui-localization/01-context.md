# 01 — Contexto

## Identificador

`0003-ui-localization`

## Data

2026-05-25

## Escopo

Localização para português-brasileiro (pt-BR) dos arquivos estáticos da interface web (`internal/ui/static/`) e correção do indicador de status de conexão SSE que permanecia travado em "Desconectado".

## Contexto do Negócio

A UI de monitoramento de pagamentos (spec `0002-payment-ui`) foi construída originalmente em inglês. O time de desenvolvimento opera em português (Brasil), e todos os envolvidos no projeto (desenvolvedores, operadores, QA) leem e escrevem em pt-BR.

Além disso, o indicador de status de conexão SSE (`#connection-status`) nunca mudava de "Desconectado", mesmo quando a conexão estava ativa e recebendo eventos. Isso ocorria porque a lógica de detecção de conexão dependia exclusivamente dos eventos `EventSource.onopen` e `EventSource.onerror`, cujo comportamento varia entre navegadores e condições de rede:

- Alguns navegadores (especialmente Chromium) não disparam `onopen` de forma confiável após uma reconexão automática
- Quedas silenciosas de conexão (half-open TCP, timeout de proxy) não são detectadas pelo `EventSource`
- O `onerror` dispara imediatamente, mas o `onopen` subsequente pode não disparar, deixando o status travado em "Desconectado"

## Problema

1. **Interface em inglês**: Todas as labels, headings, placeholders e mensagens da UI estão em inglês, dificultando a leitura e operação pelo time brasileiro.
2. **Indicador de conexão quebrado**: O status de conexão SSE nunca mudava de "Desconectado", tornando o indicador inútil para diagnosticar problemas de conectividade.
3. **Código fonte com escapes Unicode**: O uso de `\u{1F7E2}` em vez de emojis literais (`🟢`) reduz a legibilidade do código.

## Solução Proposta

1. **Localização pt-BR**: Traduzir todos os textos estáticos do `index.html` e `app.js` para português-brasileiro. As respostas da API (valores de status `pending`, `confirmed`, etc.) permanecem em inglês — apenas a exibição no frontend é localizada.
2. **Heartbeat timeout pattern**: Substituir a detecção baseada em `onopen`/`onerror` por um watchdog baseado em timeout de 45s. Se nenhum dado (heartbeat ou evento de pagamento) chegar em 45s, a conexão é considerada morta.
3. **Status value mapping**: Mapear valores internos de status para exibição em pt-BR:
   - `pending` → Pendente
   - `confirmed` → Confirmado
   - `failed` → Falhou
   - `refunded` → Reembolsado
4. **Emojis literais**: Substituir escapes Unicode por caracteres emoji literais para legibilidade.

## Público-alvo

- **Desenvolvedores** que operam a dashboard em ambiente de desenvolvimento
- **Operadores** que monitoram o sistema em staging
- **QA** que valida o fluxo de pagamentos durante testes

## Não Escopo

- Internacionalização multi-idioma (i18n)
- Tradução das respostas da API (endpoints REST)
- Modificação no backend Go para suporte a localização
- Substituição do mecanismo SSE por WebSocket
- Redesign do indicador de conexão (apenas correção da lógica de detecção)
- Adição de botão de reconexão manual

---

## Sistemas Envolvidos

| Sistema     | Função                                           | Impacto |
|-------------|--------------------------------------------------|---------|
| **Browser** | Executa o frontend (index.html, app.js, style.css) | Alterações nos arquivos estáticos |
| **Go Server**| Serve os arquivos estáticos via embed + SSE      | Nenhuma alteração necessária |
| **SSE**     | Stream de eventos em tempo real                  | Nenhuma alteração no backend |
