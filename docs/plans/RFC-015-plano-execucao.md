# RFC-015: Plano de Execução

- **RFC base:** [`.rfcs/RFC-015-evolucao-orquestracao-e-schema-de-plano.md`](../../.rfcs/RFC-015-evolucao-orquestracao-e-schema-de-plano.md)
- **Data:** 2026-02-27
- **Status:** Proposed
- **Escopo:** quebra limpa (sem retrocompatibilidade de schema)

---

## Resultado esperado

1. Schema oficial de plano com agentes/modelos no nível do plano e campos por task rejeitados.
2. `plan create` assistido por agente com entrada em texto/markdown.
3. Observabilidade de runtime conectada ao pipeline real (EventSink wiring fix).
4. Resultado final de run explícito (`success`, `partial`, `failed`, `canceled`) com exit codes.
5. Context budget previsível e auditável por fase.
6. Stall detection ativo no loop de iteração.
7. Backpressure declarativo com gates e evidência verificável.
8. `plan diagnose` operacional com diagnósticos JSONL estruturados.

---

## Fase 0 — Novo Schema e Refatoração de Domínio

- [x] **F0-T01. Redefinir structs de domínio para o novo schema**
  - `Plan`: substituir `Title` por `Name`, adicionar `SchemaVersion`, `Summary`, `Meta`, `Settings`, `Quality`
  - `Task`: remover campos `Executor`, `Reviewer`, `Model`, `Criteria`; adicionar `Acceptance []string`
  - Criar `PlanMeta` com `Source`, `CreatedAt`, `CreatedBy`, `Generator`
  - Criar `PlanGenerator` com `Name`, `Version`, `PromptHash`
  - Criar `PlanSettings` com `Agents` e `ExecutionPolicy`
  - Criar `PlanAgents` com `Planner`, `Executor`, `Reviewer`
  - Criar `PlanAgentConfig` com `Agent`, `Model`
  - Criar `ExecutionPolicy`, `BudgetPolicy`, `StallPolicy`
  - Criar `PlanQuality` com `EvidenceFormat`, `Required []string`, `Optional []string`

- [x] **F0-T02. Atualizar `StateTask` para não carregar agente/modelo**
  - Remover `Executor`, `Reviewer`, `Model` de `StateTask`
  - `StateTask`: remover campo `Criteria` (substituído por `Acceptance` em `Task`)
  - `StateTasksFromPlan` não copia mais campos de agente/modelo — lidos de `plan.Settings.Agents` no runtime

- [x] **F0-T03. Implementar parser two-pass e validador**
  - **Primeiro pass**: decode em `map[string]any`, detectar campos legados e emitir erros orientativos:
    - `tasks[].criteria` → `Field 'criteria' is no longer supported. Use 'acceptance' (array of strings) instead.`
    - `tasks[].executor/reviewer/model` → `Per-task agent fields are no longer supported. Use 'settings.agents' at plan level.`
    - `title` (top-level) → `Field 'title' is no longer supported. Use 'name' instead.`
    - `execution`, `origin` (top-level) → `Use 'settings' and 'meta' blocks instead.`
    - Sugestão de recriação: `Recreate with: praetor plan create ...`
  - **Segundo pass**: decode em structs tipadas com `json.Decoder.DisallowUnknownFields()` para rejeitar campos desconhecidos remanescentes
  - Validar contra JSON Schema v1 (incluindo `additionalProperties: false` em todos os objetos)
  - Validar `schema_version=1` e `name` obrigatórios
  - Validar `tasks[].id` obrigatório e único
  - Validar `tasks[].acceptance` não-vazio
  - Validar `timeout` como formato `time.ParseDuration` do Go (quando presente)
  - Validar DAG sem ciclos e sem auto-dependência
  - Validar `settings.agents.executor.agent` e `settings.agents.reviewer.agent` como obrigatórios

- [x] **F0-T04. Atualizar resolução de agente no pipeline**
  - `resolveExecutorWithRouting`: remover fallback para `task.Executor` — ler de `RunnerOptions` apenas
  - `resolveReviewer`: remover fallback para `task.Reviewer` — ler de `RunnerOptions` apenas
  - Adicionar `ExecutorModel` e `ReviewerModel` em `RunnerOptions`
  - Mapear `settings.execution_policy` para `RunnerOptions` (`max_total_iterations`, `max_retries_per_task`, `timeout`, budget, stall)
  - Implementar precedência (agente + modelo + política): CLI flag > `plan.Settings` > config default
  - Sequência de merge no bootstrap: LoadPlan → extrair settings → merge com CLI flags (CLI vence)
  - Expor `--executor-model` e `--reviewer-model` no `plan run`
  - Passar `plan.Settings` para `RunnerOptions` no bootstrap

- [x] **F0-T05. Atualizar path de `plan run --objective`**
  - `CognitiveAgent.Plan()` produz plano no schema v1
  - Preencher `meta` e `settings` automaticamente a partir de `RunnerOptions`
  - Validação leniente do output do planner (sem `DisallowUnknownFields`)

- [x] **F0-T06. Publicar JSON Schema v1 do plano**
  - Criar artefato versionado em `docs/schemas/plan.v1.schema.json`
  - Referenciar schema no `docs/orchestration.md`
  - Garantir alinhamento entre schema documentado e validação em runtime

- [x] **F0-T07. Atualizar `NewPlanFile` (skeleton)**
  - Gerar plano no novo formato com `schema_version`, `name`, `meta`, `settings`, `tasks`
  - Sem campos por task

- [x] **F0-T08. Atualizar `planner.system.tmpl`**
  - Descrever novo schema com `meta`, `settings.agents`, `settings.execution_policy`
  - Remover menção a executor/reviewer por task
  - Exigir `tasks[].acceptance` no output do planner

- [x] **F0-T09. Testes da Fase 0**
  - Parser aceita novo schema completo e mínimo
  - Parser exige `schema_version=1`
  - Two-pass: campos legados produzem mensagens orientativas ricas (não genéricas)
    - `criteria` → mensagem sobre `acceptance`
    - `tasks[].executor` → mensagem sobre `settings.agents`
    - `title` → mensagem sobre `name`
  - Parser rejeita campos desconhecidos via `DisallowUnknownFields` (segundo pass)
  - `timeout` inválido (ex: `"abc"`) falha com erro de formato
  - `tasks[].id` obrigatório e único
  - `tasks[].acceptance` obrigatório e não-vazio
  - DAG inválido (ciclo/autodependência) falha na validação
  - `StateTasksFromPlan` sem campos de agente
  - Resolução de agente respeita precedência CLI > plan > default
  - `NewPlanFile` gera schema válido
  - `plan run --objective` produz plano v1 com `meta` e `settings`
  - Regressão: pipeline completo funciona com novo schema (smoke test)

**Critério de aceite:** pipeline inteiro opera com o novo schema; campos por task rejeitados com erro orientativo.

**Docs:** atualizar `docs/orchestration.md` com novo schema e regras de precedência.

---

## Fase 1 — `plan create` Assistido e UX

- [x] **F1-T01. Implementar input resolver**
  - Aceitar brief como argumento posicional (`praetor plan create "texto"`)
  - Aceitar `--from-file <path>` (lê conteúdo do arquivo)
  - Aceitar `--stdin` (lê de stdin)
  - Validar: exatamente uma fonte de input
  - Erro claro se nenhuma ou múltiplas fontes

- [x] **F1-T02. Implementar slug generator**
  - `slugify(name)`: lowercase, remover acentos, substituir espaços por hifens, remover caracteres inválidos
  - Resolver colisão: `slug`, `slug-2`, `slug-3`, ...
  - `--slug <slug>` como override explícito (bypass de geração)
  - Validar: `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`

- [x] **F1-T03. Integrar planner agent no `plan create`**
  - Reusar `CognitiveAgent.Plan()` existente
  - Preencher `PlanRequest` com brief, `--planner-model`, `--planner`
  - **Validação leniente** do output: sem `DisallowUnknownFields`, campos extras ignorados. Validar apenas campos obrigatórios e estrutura.
  - Normalizar output antes de persistir: decode em struct `Plan` tipada e re-serializar (descarta campos extras do planner)
  - Se válido: prosseguir para persistência (arquivo final será estrito ao ser carregado por `LoadPlan`)
  - Se inválido: log do output completo em `logs/`, erro orientativo, sugerir `--dry-run` ou `--no-agent`

- [x] **F1-T04. Implementar persistência e saída**
  - Preencher bloco `meta` automaticamente (`source`, `created_at`, `created_by`, `generator`)
  - `meta.created_by`: `$USER` env var, fallback para `git config user.name`
  - Preencher `settings.agents.planner` com agente/model efetivo do planejamento
  - Salvar em `plans/<slug>.json` via `WriteJSONFile` (atômico)
  - Exibir: slug, path absoluto, total tasks, nome do plano

- [x] **F1-T05. Implementar flags auxiliares**
  - `--dry-run`: exibir JSON formatado em stdout sem salvar
  - `--no-agent`: gerar template mínimo no novo schema (1 task placeholder)
  - `--planner <agent>`: override do agente de planejamento
  - `--planner-model <model>`: override de modelo do planner
  - `--force`: sobrescrever plano existente
    - Se state file ativo: emitir warning com contagem de tasks done/failed
    - Sem `--force` e plano existente: erro

- [x] **F1-T06. Testes da Fase 1**
  - Entrada via arg, file, stdin
  - `--dry-run` imprime JSON válido sem criar arquivo
  - `--no-agent` gera template mínimo válido
  - Colisão de slug gera sufixo incremental
  - Planner falhando: output logado, erro claro
  - `--force` sobrescreve plano existente
  - `--force` com state ativo: warning exibido
  - Input inválido (múltiplas fontes): erro claro

**Critério de aceite:** usuário cria plano completo sem editar JSON manualmente; falhas do planner são comunicadas claramente.

**Docs:** atualizar help do comando e `README.md` com novos exemplos de `plan create`.

---

## Fase 2 — Observabilidade de Runtime

- [x] **F2-T01. Corrigir wiring de `RuntimeDeps` no `bootstrapRun`**
  - Criar `eventSink` antes da construção do runtime
  - Substituir `BuildAgentRuntime(normalized)` por `BuildAgentRuntimeWithDeps(normalized, deps)`
  - `deps.EventSink = eventSink` e depois atribuir `run.eventSink = eventSink`

- [x] **F2-T02. Verificar emissão de eventos**
  - `agent_start`: emitido antes de cada chamada ao agente
  - `agent_complete`: emitido após retorno com sucesso
  - `agent_error`: emitido após erro
  - `agent_fallback`: emitido quando `FallbackRuntime` aciona agente alternativo

- [x] **F2-T03. Expandir logging para sucesso em verbose**
  - Logger registra resultado positivo (não só erros)
  - Controlado por flag `--verbose` existente

- [x] **F2-T04. Testes de integração**
  - Cenário com sucesso: `events.jsonl` contém `agent_start` + `agent_complete`
  - Cenário com erro: `events.jsonl` contém `agent_start` + `agent_error`
  - Cenário com fallback: `events.jsonl` contém `agent_fallback`
  - Verificar conteúdo dos campos (`run_id`, `task_id`, `timestamp`)

**Critério de aceite:** `events.jsonl` contém eventos reais do middleware em todas as estratégias de runtime.

**Docs:** documentar schema de eventos em `docs/observability.md`.

---

## Fase 3 — `RunOutcome` e Exit Codes

- [x] **F3-T01. Introduzir `RunOutcome` no domínio**
  - Tipo `RunOutcome string` com constantes `RunSuccess`, `RunPartial`, `RunFailed`, `RunCanceled`
  - Adicionar campo `Outcome RunOutcome` a `RunnerStats`

- [x] **F3-T02. Computar outcome em `runnerStateFinalize`**
  - Todas as tasks `done` → `success`
  - Sem tasks ativas e `failed > 0` → `partial`
  - Erro fatal de pipeline (context canceled, panic recovery) → `canceled` ou `failed`
  - Signal/timeout → `canceled`

- [x] **F3-T03. Persistir outcome**
  - Campo `outcome` em `snapshot.json` (última persistência)
  - Campo `outcome` em `RunnerStats` retornado ao CLI

- [x] **F3-T04. Atualizar `plan status`**
  - Exibir outcome quando disponível (lido do snapshot ou state)
  - Formato: `Outcome: success ✓` / `Outcome: partial (2 failed)` / etc.

- [x] **F3-T05. Implementar tabela de exit codes**
  - `0` → `success`
  - `1` → `failed`
  - `2` → `canceled`
  - `3` → `partial`
  - Atualizar `plan run` para retornar exit code baseado em outcome

- [x] **F3-T06. Testes de regressão**
  - Run com todas as tasks passando → exit 0, outcome `success`
  - Run com task falhada e task concluída → exit 3, outcome `partial`
  - Run com erro fatal → exit 1, outcome `failed`
  - Run cancelado via context → exit 2, outcome `canceled`

**Critério de aceite:** run parcial não é confundido com sucesso pleno; exit codes determinísticos.

**Docs:** documentar tabela de exit codes em `docs/orchestration.md`.

---

## Fase 4 — Context Budget Manager

- [x] **F4-T01. Criar `ContextBudgetManager`**
  - Struct com limites por fase (`execute`, `review`)
  - Defaults: execute=120k chars, review=80k chars
  - Heurística de tokens: `len(prompt) / 4`
  - Métodos: `Budget(phase)`, `Check(phase, prompt)`, `Truncate(phase, sections)`

- [x] **F4-T02. Integrar no `prompts.go`**
  - Substituir `TruncateOutput(output, 300)` por chamada ao budget manager
  - Truncamento por prioridade:
    1. Nunca truncar: task description, acceptance
    2. Primeiro: executor output (últimas N linhas calculadas pelo budget)
    3. Segundo: git diff (últimas N linhas)
    4. Terceiro: feedback de iterações anteriores

- [x] **F4-T03. Persistir métricas por iteração**
  - `performance.jsonl` em `runtime/<run-id>/diagnostics/`
  - Campos: `iteration`, `phase`, `prompt_chars`, `estimated_tokens`, `sections_truncated[]`

- [x] **F4-T04. Expor configuração**
  - Flags: `--budget-execute <chars>`, `--budget-review <chars>`
  - Plano: `settings.execution_policy.budget.execute`, `settings.execution_policy.budget.review`
  - Config file: `budget.execute`, `budget.review`
  - `--verbose`: mostra budget usado vs disponível por iteração

- [x] **F4-T05. Testes com prompts volumosos**
  - Diff de 50k linhas: truncado sem perder task info
  - Output de 100k chars: truncado respeitando prioridade
  - Sem budget configurado: defaults aplicados sem erro
  - Métricas registradas corretamente em `performance.jsonl`

**Critério de aceite:** tamanho de prompt previsível e auditável; truncamento hardcoded removido.

**Docs:** documentar configuração de budget em `docs/orchestration.md`.

---

## Fase 5 — Stall Detection

- [x] **F5-T01. Implementar normalização e fingerprint**
  - Normalizar output: remover timestamps (`\d{4}-\d{2}-\d{2}T...`), UUIDs, paths absolutos
  - Lowercase + colapsar whitespace
  - SHA-256 do resultado normalizado

- [x] **F5-T02. Implementar janela deslizante**
  - Struct `StallDetector` com mapa `taskID+phase → []fingerprint`
  - Janela de tamanho `N` (default: 3)
  - Similaridade: `count(identical) / len(window)`
  - Threshold configurável (default: 0.67)

- [x] **F5-T03. Expor configuração de stall via schema e CLI**
  - `settings.execution_policy.stall_detection.enabled`, `window`, `threshold`
  - `enabled=false` por default no primeiro ciclo de rollout
  - Flags: `--stall-enabled`, `--stall-window`, `--stall-threshold`
  - Precedência: CLI > schema > config default

- [x] **F5-T04. Integrar no iteration machine**
  - Após `iterationStateExecuteTask`: alimentar detector com output do executor
  - Após `iterationStateReviewTask`: alimentar detector com output do reviewer
  - Se stall detectado: injetar informação no outcome para escalonamento

- [x] **F5-T05. Implementar escalonamento**
  - Nível 1: tentar fallback de agente (se `FallbackPolicy` disponível)
  - Nível 2: comprimir contexto via `ContextBudgetManager` (budget temporário reduzido)
  - Nível 3: marcar task como `failed` com razão `stalled`

- [x] **F5-T06. Emitir evento `task_stalled`**
  - Via `EventSink`: `event_type: "task_stalled"`, `data: {task_id, phase, similarity, window_size, action}`

- [x] **F5-T07. Testes de estabilidade**
  - 3 outputs idênticos → stall detectado
  - 3 outputs diferentes → sem stall
  - 2 idênticos + 1 diferente (threshold 0.67) → stall detectado
  - Outputs com timestamps diferentes mas conteúdo igual → stall detectado (normalização funciona)
  - Fallback resolve stall → task continua

**Critério de aceite:** loops repetitivos detectados e encerrados com razão `stalled`; falsos positivos controlados por threshold.

**Docs:** documentar configuração de stall detection.

---

## Fase 6 — Backpressure Declarativo

- [x] **F6-T01. Definir formato de evidência**
  - Bloco `GATES:` no output do executor:
    ```
    GATES:
    - tests: PASS (42 tests, 0 failures)
    - lint: PASS (0 issues)
    ```
  - Parser: regex por linha `^- (\w+): (PASS|FAIL)(.*)$`

- [x] **F6-T02. Implementar parser de evidência**
  - `ParseGateEvidence(output string) map[string]GateResult`
  - `GateResult{Name, Status, Detail}`
  - Retorna mapa vazio se bloco `GATES:` ausente

- [x] **F6-T03. Injetar gates no prompt do executor**
  - Se `plan.Quality.Required` não vazio: adicionar seção ao prompt
  - Incluir `plan.Quality.EvidenceFormat` na instrução de saída
  - Template: listar gates com instrução de formato
  - Se `plan.Quality` ausente: não injetar nada (compatibilidade)

- [x] **F6-T04. Evoluir reviewer para validar gates**
  - Se plan tem `quality.required`: reviewer verifica evidência
  - Gate required sem evidência → review FAIL com motivo `missing gate: <name>`
  - Gate required com FAIL → review FAIL com motivo `gate failed: <name>`
  - Todos os gates OK → não interfere na decisão normal do reviewer

- [x] **F6-T05. Garantir compatibilidade**
  - Plano sem `quality`: fluxo idêntico ao atual
  - Plano com `quality.optional` apenas: logar mas não bloquear

- [x] **F6-T06. Testes de aprovação/reprovação**
  - Gates required todos PASS → review segue normalmente
  - Gate required ausente → review FAIL automático
  - Gate required FAIL → review FAIL automático
  - Gate optional FAIL → logado, não bloqueia
  - Sem quality block → sem mudança de comportamento

**Critério de aceite:** gates obrigatórios bloqueiam conclusão sem evidência; planos sem `quality` inalterados.

**Docs:** documentar formato de evidência e exemplos de `quality`.

---

## Fase 7 — Diagnose CLI e Schema de Diagnósticos

- [x] **F7-T01. Definir schema versionado JSONL**
  - Schema v1:
    ```json
    {
      "schema_version": 1,
      "event_type": "string",
      "timestamp": "RFC3339",
      "run_id": "string",
      "task_id": "string",
      "phase": "execute|review|plan",
      "data": {}
    }
    ```
  - Event types: `agent_start`, `agent_complete`, `agent_error`, `agent_fallback`, `task_stalled`, `budget_warning`, `gate_result`, `state_transition`
  - Apenas 2 arquivos: `events.jsonl` (todos os eventos) e `performance.jsonl` (métricas de budget)
  - Erros, transições e fallbacks são filtrados por `event_type` via `plan diagnose --query`
  - Retrofit de eventos existentes para o schema

- [x] **F7-T02. Implementar `praetor plan diagnose <slug>`**
  - Carregar `events.jsonl` e `performance.jsonl` do último run (ou `--run-id`)
  - Filtrar e formatar por query

- [x] **F7-T03. Implementar queries padrão**
  - `--query errors`: eventos com `event_type` contendo `error`
  - `--query stalls`: eventos `task_stalled`
  - `--query fallbacks`: eventos `agent_fallback`
  - `--query costs`: métricas de `performance.jsonl` (tokens, chars)
  - `--query all`: todos os eventos ordenados por timestamp

- [x] **F7-T04. Implementar formatos de saída**
  - `--format table` (default): tabela legível com colunas relevantes por query
  - `--format json`: JSONL raw filtrado

- [x] **F7-T05. Testes com dados sintéticos**
  - Diagnose de run com erros → lista erros corretamente
  - Diagnose de run com stall → mostra stalls
  - Diagnose de run sem problemas → mensagem limpa
  - `--run-id` específico → filtra corretamente
  - Formatos table e json → output correto

**Critério de aceite:** investigação pós-falha orientada por diagnóstico estruturado.

**Docs:** documentar playbook de troubleshooting em `docs/troubleshooting.md`.

---

## Checkpoints

- [x] **CP0:** Fase 0 concluída — novo schema ativo, two-pass parser, campos por task rejeitados, `--objective` produz v1, pipeline funcional
- [x] **CP1:** Fase 1 concluída — `plan create` assistido operacional
- [x] **CP2:** Fase 2 concluída — EventSink conectado, `events.jsonl` com dados reais
- [x] **CP3:** Fase 3 concluída — `RunOutcome` explícito, exit codes implementados
- [x] **CP4:** Fase 4 concluída — context budget ativo, truncamento hardcoded removido
- [x] **CP5:** Fase 5 concluída — stall detection ativo com escalonamento
- [x] **CP6:** Fase 6 concluída — backpressure declarativo validando gates
- [x] **CP7:** Fase 7 concluída — `plan diagnose` operacional

---

## Validação por fase

Executar ao fim de cada fase:

1. `go test ./...` — todos os testes passando
2. Smoke test de `praetor plan create` e `praetor plan run` com cenários relevantes
3. Validação de artefatos em `runtime/<run-id>/` (quando aplicável)
4. **Atualização incremental de docs** — cada fase atualiza a documentação relevante
5. Revisão de consistência: CLI help, `docs/`, `README.md` alinhados com o código

---

## Dependências entre fases

```
F0 (schema) ──→ F1 (plan create)
      │
      ├──→ F2 (observability)
      │          │
      │          ├──→ F3 (outcome)
      │          │
      │          ├──→ F4 (budget) ──→ F5 (stall) ──→ F7 (diagnose)
      │          │
      │          └──→ F6 (backpressure) ──→ F7 (diagnose)
      │
      └──→ F1 (plan create)
```

- F0 é pré-requisito de F1 (plan create precisa do novo schema)
- F0 é pré-requisito de F2 (testes de observabilidade usam planos no novo schema)
- F2 é pré-requisito de F5 e F7 (stall e diagnose dependem de EventSink conectado)
- F4 é pré-requisito de F5 (stall usa budget manager para compressão)
- F3 pode ser feita em paralelo com F4-F6 (outcome é independente do budget/stall/backpressure)
- F1 e F2 são independentes entre si (podem ser executadas em paralelo após F0)
- F7 depende de F5 e F6 (precisa dos eventos de stall e gates para queries)
