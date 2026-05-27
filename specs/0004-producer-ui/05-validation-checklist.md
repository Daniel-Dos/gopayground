# 05 — Checklist de Validação

## Build e CI

- [ ] `go build ./cmd/ui/` compila sem erros
- [ ] `go vet ./internal/ui/` não reporta problemas
- [ ] `go test ./internal/ui/` passa (incluindo novos testes)
- [ ] `go vet ./...` limpo
- [ ] Docker build: `docker build -f Dockerfile.ui -t payment-ui:test .` passa

## Backend — Handler HandlePublish

- [ ] POST `/api/publish` com payload válido retorna 200 + JSON com `status: "success"`
- [ ] POST `/api/publish` com JSON mal formado retorna 400
- [ ] POST `/api/publish` com amount <= 0 retorna 400
- [ ] POST `/api/publish` com currency de 2 letras retorna 400
- [ ] POST `/api/publish` com description > 255 chars retorna 400
- [ ] POST `/api/publish` com Payment ID vazio gera UUID automaticamente
- [ ] POST `/api/publish` com Timestamp vazio preenche hora atual
- [ ] POST `/api/publish` retorna `partition` e `offset` do Kafka
- [ ] POST `/api/publish` publica também no EventBus (Redis Pub/Sub)
- [ ] POST `/api/publish` com Kafka indisponível retorna 502
- [ ] POST `/api/publish` com payload > 100KB retorna 413
- [ ] POST `/api/publish` respeita rate limit (429 se excedido)

## Backend — Handler HandlePublishBulk

- [ ] POST `/api/publish/bulk` com count=10 retorna 200 com 10 resultados
- [ ] POST `/api/publish/bulk` sem body usa default de 10
- [ ] POST `/api/publish/bulk` com count > 50 retorna 400
- [ ] POST `/api/publish/bulk` com count < 1 retorna 400

## Backend — Server e Roteamento

- [ ] `GET /producer` serve `producer.html`
- [ ] `GET /` (root) continua servindo `index.html`
- [ ] `POST /api/publish` registrada no mux
- [ ] `POST /api/publish/bulk` registrada no mux
- [ ] Navegação entre Dashboard e Producer funciona via links
- [ ] Servidor não falha se Kafka estiver indisponível (apenas log.Warn)

## Frontend — Formulário

- [ ] Todos os campos renderizam corretamente
- [ ] Campo Status tem 4 opções: pending, confirmed, failed, refunded
- [ ] Campo Payment ID aceita UUID v4 válido
- [ ] Campo Amount só aceita números > 0
- [ ] Campo Currency força uppercase e máximo 3 chars
- [ ] Campo Description tem maxlength 255
- [ ] Campo Timestamp é auto-preenchido com hora atual

## Frontend — Validação

- [ ] Payment ID inválido (não-UUID) mostra erro inline
- [ ] Amount vazio ou <= 0 mostra erro inline
- [ ] Currency diferente de 3 letras mostra erro inline
- [ ] Description > 255 chars mostra erro inline
- [ ] Timestamp inválido (não-RFC3339) mostra erro inline
- [ ] Botão "Publicar" desabilitado enquanto há erros de validação
- [ ] Botão "Publicar" desabilitado durante envio (prevenção duplicidade)
- [ ] Botão "Publicar" reabilitado após resposta

## Frontend — Preview

- [ ] Preview JSON atualiza em tempo real conforme formulário muda
- [ ] Preview mostra UUID gerado quando Payment ID está vazio
- [ ] Preview mostra timestamp atualizado

## Frontend — Feedback

- [ ] Toast verde de sucesso aparece após publicação bem-sucedida
- [ ] Toast vermelho de erro aparece após falha
- [ ] Toast mostra Payment ID + detalhes relevantes
- [ ] Toast desaparece após 5 segundos

## Frontend — Histórico da Sessão

- [ ] Evento publicado aparece na tabela imediatamente
- [ ] Tabela ordenada do mais recente para o mais antigo
- [ ] Máximo 50 linhas (mais antigos descartados)
- [ ] Colunas: ID, Status, Valor, Moeda, Timestamp, Resultado

## Frontend — Bulk

- [ ] Botão "Publicar 10 Aleatórios" funciona
- [ ] 10 eventos aparecem na tabela após sucesso
- [ ] Toast com resumo (ex: "10 eventos publicados com sucesso")

## Frontend — Responsivo

- [ ] Formulário ocupa largura total em mobile
- [ ] Tabela tem scroll horizontal em mobile
- [ ] Fonte e espaçamento adequados em telas pequenas
- [ ] Botões empilhados verticalmente em mobile

## Frontend — Navegação

- [ ] Link "Dashboard" na Producer UI volta para `/`
- [ ] Link "Producer" no Dashboard leva para `/producer`
- [ ] Link "Documentação" em ambas as páginas leva para `/docs/`
- [ ] Link ativo destacado visualmente
- [ ] Documentação carrega corretamente em `/docs/`

## Frontend — Dark Theme

- [ ] Esquema de cores consistente com dashboard existente
- [ ] Fundo escuro, texto claro
- [ ] Inputs e selects estilizados com o mesmo padrão

## Docker

- [ ] `docker-compose up` sobe sem erros
- [ ] Serviço `payment-ui` tem env vars `KAFKA_BROKERS` e `KAFKA_TOPIC`
- [ ] Producer UI acessível em `http://localhost:8081/producer`
- [ ] Evento publicado via UI aparece na dashboard em tempo real
