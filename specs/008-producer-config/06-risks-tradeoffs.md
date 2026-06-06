# 06 — Riscos e Trade-offs

## Riscos Identificados

| ID | Risco | Probabilidade | Impacto | Mitigação |
|----|-------|---------------|---------|-----------|
| R-001 | Scripts ou deploy pipelines que passam `--port 8082` continuam funcionando (risco baixo) | Baixa | Baixo | Compatibilidade retroativa garantida — flag continua existindo |
| R-002 | Alguém definir `producer.port: 0` no config.yaml acidentalmente | Baixa | Médio | Fallback `8082` cobre este caso; logging warning pode ser adicionado |
| R-003 | Confusão entre `ProducerConfig` (config.go) e `ProducerConfig` (kafka/provider.go) | Média | Baixo | Nomes em pacotes diferentes; `config.ProducerConfig` vs `kafka.ProducerConfig` |
| R-004 | Variável `PRODUCER_PORT` conflitar com outra ferramenta no ambiente | Baixa | Baixo | Nome específico com prefixo `PRODUCER_` reduz chance; Viper mapeia apenas `producer.port` |
| R-005 | Docker compose sem `--port` explícito pode confundir operadores que esperavam ver a flag | Média | Baixo | Documentação atualizada explica que a porta vem do config.yaml |

## Trade-offs

### Decisão 1: Novo struct `ProducerConfig` vs reaproveitar `ServerConfig`

| Opção | Prós | Contras |
|-------|------|---------|
| **Novo `ProducerConfig`** (escolhido) | Semântica clara; isolamento de responsabilidades; permite crescer com novos campos do producer | Mais um tipo no pacote `config` |
| Reaproveitar `ServerConfig` | Menos código; reuso do campo `Port` | Semanticamente incorreto (ServerConfig é do consumer); impossível ter configs divergentes no futuro (ex: graceful_shutdown diferente) |

### Decisão 2: Fallback 8082 vs zero = erro

| Opção | Prós | Contras |
|-------|------|---------|
| **Fallback para 8082** (escolhido) | Tolerante; comportamento previsível; compatibilidade retroativa | Porta errada pode passar despercebida se config.yaml estiver mal formatado |
| Retornar erro se `cfg.Producer.Port == 0` | Detecta config mal formada cedo | Quebra compatibilidade; exigiria config.yaml sempre presente com producer.port definido |

### Decisão 3: Manter flag `--port` vs remover

| Opção | Prós | Contras |
|-------|------|---------|
| **Manter flag** (escolhido) | Compatibilidade retroativa total; override explícito útil para debug | Duas fontes de verdade (flag e YAML) — mas ordem de precedência clara |
| Remover flag | Simplifica o código; força uso de config.yaml/env vars | Quebra scripts existentes; reduz flexibilidade para override rápido |

### Decisão 4: Adicionar warning log se cfg.Producer.Port == 0

| Opção | Prós | Contras |
|-------|------|---------|
| Não adicionar warning (inicial) | Menos código; fallback silencioso | Operador pode não perceber que config.yaml não foi carregado |
| Adicionar warning log | Visibilidade sobre fallback sendo usado | Ruído em logs se consciente |

> **Recomendação:** Adicionar `slog.Warn("producer port not configured, using default 8082")` no
> fallback como boa prática de observabilidade. Decisão deixada para o implementador.
