# 02 — Requisitos

## Requisitos Funcionais

### RF-01 — Formulário de Publicação

O formulário deve conter os seguintes campos:

| Campo       | Tipo     | Obrigatório | Observação                                    |
|-------------|----------|-------------|-----------------------------------------------|
| Payment ID  | texto    | Não         | UUID v4; auto-gerado se vazio                 |
| Status      | select   | Sim         | Opções: `pending`, `confirmed`, `failed`, `refunded` |
| Amount      | número   | Sim         | Deve ser > 0                                  |
| Currency    | texto    | Sim         | ISO 4217, 3 caracteres, uppercase automático  |
| Description | textarea | Não         | Máximo 255 caracteres                         |
| Timestamp   | texto    | Não         | RFC3339, auto-preenchido com hora atual       |

### RF-02 — Botão "Publicar"

- Envia `POST /api/publish` com JSON do `PaymentEvent`
- Desabilita o botão durante o envio para prevenir duplicidade
- Reabilita após resposta (sucesso ou erro)

### RF-03 — Feedback Visual

- **Sucesso**: toast/mensagem verde com Payment ID + partition + offset
- **Erro**: toast/mensagem vermelha com descrição do erro
- O formulário **não** deve ser limpo em caso de erro (preserva dados)

### RF-04 — Preview dos Dados

- Antes de enviar, exibir um bloco de pré-visualização com os dados
  formatados em JSON (read-only)
- Atualizar em tempo real conforme o usuário preenche o formulário
- Mostrar o UUID gerado no campo Payment ID quando o campo está vazio

### RF-05 — Botão "Publicar 10 Aleatórios"

- Chama o mesmo `POST /api/publish` (ou um endpoint específico
  `POST /api/publish/bulk`) que gera 10 eventos aleatórios no backend
- Deve usar a função `GenerateBulkEvents(10)` do pacote `producer`
- Feedback com toast para cada evento publicado (ou resumo)

### RF-06 — Histórico da Sessão (Tabela)

- Tabela com as últimas publicações da sessão atual
- Colunas: Payment ID, Status, Amount, Currency, Timestamp, Resultado
- Ordenado do mais recente para o mais antigo (inserção no topo)
- Máximo 50 eventos na tabela (descarta os mais antigos)
- Persistência apenas em memória (sessionStorage do navegador)

### RF-07 — Navegação

- Adicionar link "Producer" no header da dashboard existente
- Adicionar link "Dashboard" no header da Producer UI
- Adicionar link "Documentação" no header de ambas as páginas, apontando para `/docs/`
- A página `GET /producer` serve o `producer.html`

## Requisitos Não-Funcionais

### RNF-01 — Idioma

- Layout completo em **Português-Brasil** (consistente com o dashboard existente)
- Labels, placeholders, toasts, tooltips em pt-BR

### RNF-02 — Dark Theme

- Mesmo esquema de cores e estilo visual do dashboard existente
- Reutilizar `style.css` já existente (ou estender com classes no mesmo arquivo)

### RNF-03 — Responsivo

- Mobile-first (como o dashboard existente)
- Formulário deve ocupar largura total em mobile e coluna única em desktop
- Tabela de histórico deve ser scrollável horizontalmente em mobile

### RNF-04 — Zero Dependências Novas

- HTML + CSS + JavaScript vanilla
- Sem frameworks, sem bibliotecas externas
- Sem novas dependências Go (reutilizar `sarama`, `validator`, `models` existentes)

### RNF-05 — Validação Frontend

Validar antes de enviar ao servidor:

- `payment_id`: se preenchido, deve ser UUID v4 válido
- `amount`: deve ser número > 0
- `currency`: exatamente 3 letras maiúsculas
- `description`: máximo 255 caracteres
- `timestamp`: se preenchido, deve ser RFC3339 válido
- Exibir mensagens de erro inline abaixo de cada campo

### RNF-06 — Validação Backend

- Reutilizar o `validator.Validator` existente em `internal/validator/validator.go`
- Validar antes de publicar no Kafka
- Retornar 400 com mensagem de erro em caso de payload inválido

### RNF-07 — Performance

- Timeout de 10s para publicação no Kafka
- Payload máximo: 100KB
- Rate limiting: máximo 1 request a cada 200ms por IP (5 req/s)
