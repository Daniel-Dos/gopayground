# 01 — Contexto

## Contexto do Negócio

Este spec define uma **CLI tool** para que desenvolvedores publiquem eventos de
pagamento diretamente no tópico Kafka `payment.events` a partir do terminal.

A ferramenta atende a necessidades operacionais e de desenvolvimento que não
são cobertas pelo produtor externo (API de pagamentos / gateway externo).

### Cenários de Uso

1. **Teste do consumer sem dependência externa** — o desenvolvedor pode publicar
   eventos diretamente no Kafka sem precisar de um sistema produtor rodando.
2. **QA e depuração manual** — cenários específicos podem ser simulados com
   payloads customizados, validando o comportamento do consumer diante de
   eventos válidos, inválidos e casos de borda.
3. **Simulação de cenários** — eventos com `status: failed`, `amount: 0`
   (rejeitado pelo validator), timestamps futuros, UUIDs inválidos, etc.
4. **Teste de carga controlado** — modo bulk (`--count N`) com rate limiting
   (`--rate`) para submeter centenas/milhares de eventos sem sobrecarregar o
   Kafka.

### Fluxo de Uso

```
1. Desenvolvedor invoca CLI com payload direto, flags individuais, pipe stdin,
   ou arquivo JSON.
2. CLI valida os dados reutilizando o validator existente do consumer.
3. Em modo dry-run, exibe o JSON do evento sem publicar.
4. Em modo normal, publica no tópico `payment.events` via sarama SyncProducer.
5. Em caso de sucesso, exibe partição e offset.
6. Em caso de erro, sai com código não-zero e mensagem descritiva.
```

## Sistemas Envolvidos

| Sistema   | Função                                              |
|-----------|-----------------------------------------------------|
| **Kafka** | Tópico `payment.events` onde os eventos são publicados |
| **CLI**   | Ferramenta local que gera e publica eventos         |

## Público-Alvo

- Desenvolvedores do time de pagamentos
- Engenheiros de QA
- Engenheiros de SRE para testes de resiliência

## Não Escopo

- Processamento de eventos (já coberto pelo consumer — spec 0001)
- Interface gráfica (já coberta pelo UI — spec 0002)
- DLQ publishing ou reprocessamento
- Autenticação/autorização contra o Kafka (assume rede confiável em dev)
- Geração de payloads aleatórios complexos (apenas campos básicos + UUID)
- Pipeline CI/CD — ferramenta de uso local

---

## Decisões de Design

- **`flag` stdlib** em vez de `cobra`/`pflag` para zero novas dependências.
- **Separado do consumer** em vez de subcomando para manter o consumer
  enxuto e sem dependências de CLI.
- **Reuso do validator existente** para garantir consistência de validação
  entre produtor (CLI) e consumidor.
- **SyncProducer** do sarama para confirmação síncrona de publicação
  (feedback imediato para o usuário).
