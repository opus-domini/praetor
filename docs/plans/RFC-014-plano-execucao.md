# RFC-014: Plano de Execucao

- **RFC base:** [`docs/plans/RFC-014-evolucao-fluxo-orquestracao.md`](/home/hugo/Workspace/opus-domini/praetor/docs/plans/RFC-014-evolucao-fluxo-orquestracao.md)
- **Data:** 2026-02-27
- **Status:** Proposed
- **Escopo:** quebra limpa (sem retrocompatibilidade)

## Resultado esperado

1. `plan create` assistido por agente com entrada em texto/markdown.
2. Plano com schema oficial (`name`, `summary`, `execution`, `quality`, `tasks`) e agentes/modelos no nivel do plano.
3. Observabilidade de runtime ligada por padrao com eventos JSONL.
4. Resultado final de run explicito (`success`, `partial`, `failed`, `canceled`).
5. Stall detection e backpressure declarativo funcionando no loop.
6. `plan diagnose` operacional com diagnosticos estruturados.

---

## Fase 0 - Plan Create Assistido + Novo Schema

- [ ] **F0-T01. Redesenhar UX do comando `plan create`**
  - Aceitar brief em arg (`praetor plan create "texto"`)
  - Aceitar `--from-file <md|txt>`
  - Aceitar `--stdin`

- [ ] **F0-T02. Integrar planner agent no `plan create`**
  - Reusar runtime cognitivo de planejamento
  - Adicionar flags `--planner` e `--model`

- [ ] **F0-T03. Implementar geracao automatica de slug**
  - `slugify(name)`
  - Resolver colisao por sufixo incremental
  - `--slug` como override explicito

- [ ] **F0-T04. Definir parser/validator do novo schema oficial**
  - Campos obrigatorios: `name`, `tasks`, `execution.executor_agent`, `execution.reviewer_agent`
  - Rejeitar campos por task: `executor`, `reviewer`, `model`

- [ ] **F0-T05. Persistencia e saida do `plan create`**
  - Salvar em `plans/<slug>.json`
  - Exibir resumo final (slug, path, total tasks)
  - `--dry-run` para imprimir sem salvar

- [ ] **F0-T06. Fallback sem agente**
  - `--no-agent` gera template minimo valido no novo schema

- [ ] **F0-T07. Testes de `plan create`**
  - Casos: arg, file, stdin, dry-run, no-agent, colisao de slug
  - Falhas: schema invalido vindo do planner

**Criterio de aceite da fase:** usuario cria plano completo sem editar JSON manualmente.

---

## Fase 1 - Foundation de Observabilidade

- [ ] **F1-T01. Ligar `RuntimeDeps.EventSink` no bootstrap do runner**
- [ ] **F1-T02. Garantir emissao de `agent_start`, `agent_complete`, `agent_error`, `agent_fallback`**
- [ ] **F1-T03. Melhorar logger default para registrar sucesso em modo verbose**
- [ ] **F1-T04. Testes de integracao para `events.jsonl` em `tmux`, `direct`, `pty`**

**Criterio de aceite da fase:** trilha de eventos de runtime consistente em todas as estrategias.

---

## Fase 2 - Outcome Final de Run

- [ ] **F2-T01. Introduzir `RunOutcome` no dominio**
- [ ] **F2-T02. Persistir outcome em snapshot/meta**
- [ ] **F2-T03. Atualizar `plan status` com outcome final**
- [ ] **F2-T04. Revisar exit codes (`success=0`, `partial=3`, demais nao-zero)**
- [ ] **F2-T05. Testes de regressao para os quatro outcomes**

**Criterio de aceite da fase:** run parcial nao aparece como sucesso pleno.

---

## Fase 3 - Stall Detection

- [ ] **F3-T01. Implementar fingerprint de saida por task/fase**
- [ ] **F3-T02. Implementar janela deslizante e threshold configuravel**
- [ ] **F3-T03. Integrar stall detector com retry/fallback**
- [ ] **F3-T04. Emitir evento `task_stalled` em diagnosticos**
- [ ] **F3-T05. Testes de estabilidade (evitar falso positivo recorrente)**

**Criterio de aceite da fase:** loops improdutivos sao detectados e encerrados com razao explicita.

---

## Fase 4 - Backpressure Declarativo

- [ ] **F4-T01. Suportar bloco `quality` no plano**
- [ ] **F4-T02. Injetar gates requeridos no prompt do executor**
- [ ] **F4-T03. Definir formato de evidencia estruturada de gates**
- [ ] **F4-T04. Evoluir parser/reviewer para validar evidencia**
- [ ] **F4-T05. Garantir comportamento padrao quando `quality` ausente**
- [ ] **F4-T06. Testes de aprovacao/reprovacao por gate**

**Criterio de aceite da fase:** tarefa so conclui quando evidencias obrigatorias estiverem satisfeitas.

---

## Fase 5 - Context Budget Manager

- [ ] **F5-T01. Criar `ContextBudgetManager` por fase (`plan`, `execute`, `review`)**
- [ ] **F5-T02. Integrar truncamento/sumarizacao de `diff`, `output`, `feedback`**
- [ ] **F5-T03. Persistir metricas (`prompt_bytes`, `estimated_tokens`, truncamentos)**
- [ ] **F5-T04. Expor configuracao por CLI e config**
- [ ] **F5-T05. Testes com prompts volumosos**

**Criterio de aceite da fase:** consumo de contexto previsivel e auditavel.

---

## Fase 6 - Diagnose CLI

- [ ] **F6-T01. Versionar schema de diagnosticos JSONL**
- [ ] **F6-T02. Implementar `praetor plan diagnose <slug>`**
- [ ] **F6-T03. Adicionar consultas padrao (erro, fallback, stall, custo)**
- [ ] **F6-T04. Formatos de saida (`json` e `table`)**
- [ ] **F6-T05. Documentar playbook de troubleshooting**

**Criterio de aceite da fase:** investigacao pos-falha sem depender de leitura manual extensa de logs brutos.

---

## Fase 7 - Workflow Event-Driven (Experimental)

- [ ] **F7-T01. Prototipar bloco `workflow.events` no plano**
- [ ] **F7-T02. Implementar roteador de trigger -> role**
- [ ] **F7-T03. Integrar com snapshot/checkpoint e diagnosticos**
- [ ] **F7-T04. Feature flag experimental**
- [ ] **F7-T05. Testes de fallback para DAG padrao**

**Criterio de aceite da fase:** fluxo event-driven opt-in sem degradar o caminho principal.

---

## Fase 8 - Documentacao Final e Publicacao

- [ ] **F8-T01. Atualizar documentacao tecnica em `docs/`**
  - Revisar `docs/orchestration.md` com novo fluxo (`plan create` assistido, outcome final, diagnose)
  - Revisar `docs/architecture.md` com diagramas e componentes novos (`ContextBudgetManager`, stall detector, diagnosticos)
  - Ajustar exemplos de plano para o schema oficial

- [ ] **F8-T02. Atualizar `README.md`**
  - Novo fluxo de criacao de plano (texto/markdown)
  - Exemplo de plano no novo schema
  - Comandos novos/atualizados (`plan diagnose`, flags de `plan create`)

- [ ] **F8-T03. Revisao de consistencia documental**
  - Garantir que `README.md` e `docs/` nao citem campos por task (`executor`, `reviewer`, `model`)
  - Garantir alinhamento entre exemplos e CLI real

**Criterio de aceite da fase:** `docs/` e `README.md` refletem integralmente o comportamento final entregue.

---

## Checkpoints

- [ ] **CP0:** Fase 0 concluida (novo `plan create` + novo schema)
- [ ] **CP1:** Observabilidade runtime consolidada
- [ ] **CP2:** Outcome final explicitado e testado
- [ ] **CP3:** Stall detection ativo em producao de teste
- [ ] **CP4:** Backpressure declarativo validando gates
- [ ] **CP5:** Context budget manager com metricas
- [ ] **CP6:** `plan diagnose` operacional
- [ ] **CP7:** Workflow event-driven experimental validado
- [ ] **CP8:** documentacao final atualizada (`docs/` + `README.md`)

---

## Validacao por fase

Executar ao fim de cada fase:

1. `go test ./...`
2. smoke de `praetor plan create` e `praetor plan run`
3. validacao de artefatos em `runtime/<run-id>/`
