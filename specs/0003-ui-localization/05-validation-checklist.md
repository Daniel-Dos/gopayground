# 05 — Checklist de Validação

## Build e CI

| # | Item | Critério de Aceitação | Status |
|---|------|----------------------|--------|
| 1 | `go build ./cmd/ui/` compila sem erros | Build bem-sucedido | ⬜ |
| 2 | `go vet ./internal/ui/` sem warnings | Nenhum problema reportado | ⬜ |
| 3 | Arquivos estáticos embutidos via `//go:embed static/*` | Embed não quebrado | ⬜ |

## Localização pt-BR do index.html

| # | Item | Critério de Aceitação | Status |
|---|------|----------------------|--------|
| 4 | `<html lang="pt-BR">` | Atributo `lang` alterado | ⬜ |
| 5 | Título da página | `<title>` exibe "Monitor de Pagamentos" | ⬜ |
| 6 | Cabeçalho principal | `<h1>` exibe "Monitor de Pagamentos" | ⬜ |
| 7 | Subtítulo | "Processamento de Pagamentos em Tempo Real" | ⬜ |
| 8 | Label do filtro ID | "ID do Pagamento" | ⬜ |
| 9 | Placeholder do filtro ID | "Buscar por ID..." | ⬜ |
| 10 | Label do filtro status | "Situação" | ⬜ |
| 11 | Options do select de status | "Todos", "Pendente", "Confirmado", "Falhou", "Reembolsado" | ⬜ |
| 12 | Título do feed | "Feed de Eventos ao Vivo" | ⬜ |
| 13 | Badge de tempo real | "Tempo Real" | ⬜ |
| 14 | Título da tabela | "Pagamentos" | ⬜ |
| 15 | Headers da tabela | "ID do Pagamento", "Situação", "Atualizado Em", "Ações" | ⬜ |
| 16 | Título do modal | "Histórico do Pagamento" | ⬜ |
| 17 | Headers do modal | "Data/Hora", "Situação", "Valor", "Moeda", "Descrição", "Processado Em", "ID de Rastreamento" | ⬜ |
| 18 | Status inicial de conexão | `🔴 Desconectado` | ⬜ |
| 19 | Aria-label do botão fechar | "Fechar" | ⬜ |

## Localização pt-BR do app.js

| # | Item | Critério de Aceitação | Status |
|---|------|----------------------|--------|
| 20 | Status conectado | "🟢 Conectado" (emoji literal) | ⬜ |
| 21 | Status desconectado | "🔴 Desconectado" (emoji literal) | ⬜ |
| 22 | Texto "nenhum pagamento" | "Nenhum pagamento encontrado." | ⬜ |
| 23 | Texto "sem histórico" | "Nenhum registro de histórico encontrado." | ⬜ |
| 24 | Texto "carregando" | "Carregando..." | ⬜ |
| 25 | Botão "Ver Histórico" | "Ver Histórico" | ⬜ |
| 26 | Erro de histórico | "Falha ao carregar histórico" | ⬜ |
| 27 | Label "Total Processado" em métricas | "Total Processado" | ⬜ |
| 28 | Label "Taxa de Sucesso" em métricas | "Taxa de Sucesso" | ⬜ |
| 29 | Label "DLQ Contagem" em métricas | "DLQ Contagem" | ⬜ |

## Status Value Mapping

| # | Item | Critério de Aceitação | Status |
|---|------|----------------------|--------|
| 30 | Mapeamento `pending` | "Pendente" no badge | ⬜ |
| 31 | Mapeamento `confirmed` | "Confirmado" no badge | ⬜ |
| 32 | Mapeamento `failed` | "Falhou" no badge | ⬜ |
| 33 | Mapeamento `refunded` | "Reembolsado" no badge | ⬜ |
| 34 | Labels de métricas por status | "Pendentes", "Confirmados", "Falhas", "Reembolsados" | ⬜ |
| 35 | Feed de eventos usa statusMap | Badge de status no feed localizado | ⬜ |
| 36 | Tabela de pagamentos usa statusMap | Badge de status na tabela localizado | ⬜ |
| 37 | Tabela de histórico usa statusMap | Badge de status no histórico localizado | ⬜ |

## Heartbeat Timeout Pattern

| # | Item | Critério de Aceitação | Status |
|---|------|----------------------|--------|
| 38 | Timer de 45s implementado | `HEARTBEAT_TIMEOUT_MS = 45000` | ⬜ |
| 39 | `clearTimeout` antes de reset | Timer anterior cancelado em `scheduleHeartbeatTimeout()` | ⬜ |
| 40 | `onopen` reseta timeout | Conexão estabelecida → timer resetado | ⬜ |
| 41 | `heartbeat` reseta timeout | Heartbeat recebido → timer resetado | ⬜ |
| 42 | `payment` reseta timeout | Evento recebido → timer resetado | ⬜ |
| 43 | `onerror` não altera status | Apenas loga, não muda indicador | ⬜ |
| 44 | Timeout expirado → desconectado | 45s sem dados → "🔴 Desconectado" | ⬜ |
| 45 | Reconexão → conectado | Evento chega após timeout → "🟢 Conectado" | ⬜ |
| 46 | Timer inicial na carga | `resetHeartbeatTimeout()` chamado na inicialização | ⬜ |

## Emojis Literais

| # | Item | Critério de Aceitação | Status |
|---|------|----------------------|--------|
| 47 | Nenhum escape `\u{...}` em app.js | Apenas emojis literais usados | ⬜ |
| 48 | Emoji 🟢 literal | Caractere Unicode direto | ⬜ |
| 49 | Emoji 🔴 literal | Caractere Unicode direto | ⬜ |

## Enhancement Scripts (index.html)

| # | Item | Critério de Aceitação | Status |
|---|------|----------------------|--------|
| 50 | MutationObserver para ícones | Ícone adicionado a cada card de métrica baseado no título | ⬜ |
| 51 | Mapa de ícones definido | 7 entradas: Total, Confirmados, Falhas, Pendentes, Reembolsados, Taxa, DLQ | ⬜ |
| 52 | MutationObserver para timestamp | Timestamp extraído do texto do feed para elemento separado | ⬜ |
| 53 | MutationObserver para contagem | Contagem de linhas da tabela exibida no badge | ⬜ |

## Testes de Comportamento

| # | Item | Critério de Aceitação | Status |
|---|------|----------------------|--------|
| 54 | Conexão SSE estabelecida | Indicador mostra "🟢 Conectado" | ⬜ |
| 55 | Desconexão de rede | Após 45s, indicador mostra "🔴 Desconectado" | ⬜ |
| 56 | Reconexão automática | Indicador volta a "🟢 Conectado" | ⬜ |
| 57 | Status badge no feed | Exibe em pt-BR: Pendente, Confirmado, etc. | ⬜ |
| 58 | Status badge na tabela | Exibe em pt-BR | ⬜ |
| 59 | Status badge no histórico | Exibe em pt-BR | ⬜ |
| 60 | Filtro por status | Options em pt-BR, mas value em inglês (funcional) | ⬜ |
| 61 | Navegação inalterada | Links e funcionalidades não quebrados | ⬜ |
| 62 | Métricas exibidas corretamente | Labels e valores corretos | ⬜ |
| 63 | Modal de histórico funciona | Abre, carrega dados, fecha | ⬜ |

## Regressão

| # | Item | Critério de Aceitação | Status |
|---|------|----------------------|--------|
| 64 | API `/api/payments` funcional | Tabela carrega pagamentos | ⬜ |
| 65 | API `/api/metrics` funcional | Métricas carregam | ⬜ |
| 66 | SSE `/api/events` funcional | Feed em tempo real funciona | ⬜ |
| 67 | CSS não alterado | Nenhuma modificação em `style.css` | ⬜ |
| 68 | Nenhum arquivo Go modificado | `git diff` mostra apenas `index.html` e `app.js` | ⬜ |

## Resumo para Aprovação

| Área | Status | Observações |
|------|--------|-------------|
| Localização index.html | [ ] | |
| Localização app.js | [ ] | |
| Status value mapping | [ ] | |
| Heartbeat timeout | [ ] | |
| Emojis literais | [ ] | |
| Enhancement scripts | [ ] | |
| Regressão (backend inalterado) | [ ] | |
| Testes de comportamento | [ ] | |

**Assinatura do Hardening Engineer:** ____________________

**Data da validação:** ____________________
