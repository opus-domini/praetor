# RFC-009: Reestruturação de Diretórios — Plano de Execução

- **RFC:** `.rfcs/RFC-009-convergencia-arquitetural-plan-execute-fsm.md` §5.1
- **Data:** 2026-02-26
- **Objetivo:** Dissolver `internal/loop/` e alinhar o codebase estritamente com a estrutura alvo da RFC-009.

## Estrutura alvo (RFC-009 §5.1)

```text
cmd/praetor/                      # entrypoint apenas
internal/
  app/                            # bootstrap de dependências e wiring
  domain/                         # tipos de domínio puros
  orchestration/
    fsm/                          # máquina de estados funcional
    pipeline/                     # regras Plan/Execute/Review + runner
  agents/                         # interface central Agent + registry
  providers/
    claude/                       # adapter Claude CLI
    codex/                        # adapter Codex CLI
    gemini/                       # futuro
    ollama/                       # futuro
  runtime/
    process/                      # exec não interativo
    pty/                          # integração interativa (TTY real)
    tmux/                         # sessões tmux para observabilidade
  workspace/                      # detecção de root, manifesto local
  state/                          # snapshots, checkpoints, lock, recovery
  cli/                            # Cobra command wiring (infraestrutura)
```

## Pacotes a eliminar

| Pacote atual            | Destino                          | Motivo                                    |
|-------------------------|----------------------------------|-------------------------------------------|
| `internal/loop/`        | Dissolvido em 7+ pacotes         | Monólito que concentra domínio + runtime   |
| `internal/orchestrator/`| Deletado                         | Zero importadores                          |
| `internal/paths/`       | Absorvido em `state/`            | RFC não lista como pacote separado         |

---

## Fase 1 — Remoções e fundações

- [x] **T01. Remover `internal/orchestrator/`**
  - Contract types (Request, Result, Provider) movidos para `internal/providers/catalog.go`
  - Adapters `claude/adapter.go` e `codex/adapter.go` atualizados para importar `providers`
  - Engine + Registry deletados (zero importadores)
  - Pacote removido integralmente

- [x] **T02. Absorver `internal/paths/` em `internal/state/`**
  - Funções XDG movidas para `state/xdg.go`, testes para `state/xdg_test.go`
  - `state/roots.go`, `loop/project.go`, `loop/migrate.go`, `config/config.go` atualizados
  - `internal/paths/` deletado

- [x] **T03. Criar `internal/app/`**
  - Criado `app/app.go` com `ResolveStateRoot`, `ResolveCacheRoot`, `ResolveProjectRoot`
  - Wiring completo será extraído conforme loop for dissolvido

## Fase 2 — Extrair Store do loop para state

- [x] **T04. Mover Store para `internal/state/`**
  - Criados `state/store.go`, `store_lock.go`, `store_metrics.go`, `store_state.go`, `store_retry.go`, `store_plan.go`
  - Helpers exportados em `domain/plan.go`: `CanonicalTaskID`, `AutoTaskFingerprint`, `StateTasksFromPlan`, `WriteJSONFile`, `SanitizePathToken`, `PlanChecksum`
  - `loop/store.go` reduzido a alias `Store = state.Store`
  - Originais deletados do loop

- [x] **T05. Mover snapshot para `internal/state/`**
  - Criado `state/local_snapshot.go` com `LocalSnapshot`, `LocalSnapshotStore`, `LoadLatestLocalSnapshot`
  - `loop/snapshot.go` reduzido a aliases

- [x] **T06. Mover migração para `internal/state/`**
  - Criado `state/migrate.go` com migração de estado
  - `loop/migrate.go` reduzido a delegação

## Fase 3 — Extrair domain e prompts do loop

- [x] **T07. Mover plan loading/validation para `internal/domain/`**
  - `LoadPlan`, `ValidatePlan`, `NewPlanFile` adicionados a `domain/plan.go`
  - `loop/plan.go` reduzido a delegações

- [x] **T08. Mover tipos cognitivos para `internal/orchestration/pipeline/`**
  - Criado `pipeline/cognitive.go` com `CognitiveAgent`, `PlanRequest`, `ExecuteRequest`, `ReviewRequest`, `NewCognitiveAgent`, `ExtractJSONObject`
  - `loop/cognitive_agent.go` reduzido a aliases e delegação

- [x] **T09. Mover prompts para `internal/orchestration/pipeline/`**
  - Criado `pipeline/prompts.go` com `BuildExecutorSystemPrompt`, `BuildExecutorTaskPrompt`, `BuildReviewerSystemPrompt`, `BuildReviewerTaskPrompt`, `TruncateOutput`
  - `loop/prompts.go` reduzido a delegações

## Fase 4 — Extrair runner e policies do loop

- [x] **T10. Mover runner para `internal/orchestration/pipeline/`**
  - Criados `pipeline/runner.go`, `pipeline/runner_outcome.go`, `pipeline/runner_policy.go`
  - ~1200 linhas movidas com todas as referências atualizadas para `domain.`, `localstate.`, `workspace.`
  - Testes movidos para `pipeline/runner_test.go` e `pipeline/runtime_composed_test.go`
  - `loop/runner.go` reduzido a alias `Runner = pipeline.Runner`

- [x] **T11. Mover output/renderer para `internal/cli/`**
  - Criada interface `domain.RenderSink` desacoplando pipeline do renderer concreto
  - Criado `cli/render.go` com `Renderer` implementando `domain.RenderSink`
  - `loop/output.go` reduzido a alias `RenderSink = domain.RenderSink`
  - `Runner.Run` alterado para aceitar `domain.RenderSink` em vez de `io.Writer`

## Fase 5 — Extrair runtimes do loop

- [x] **T12. Consolidar `runtime_direct.go` em `internal/runtime/process/`**
  - `process/runner.go` atualizado para usar `domain.CommandSpec` e `domain.ProcessResult`
  - `loop/runtime_direct.go` reduzido a wrapper delegando para `process.Runner`

- [x] **T13. Absorver `runtime_pty.go` em `internal/runtime/pty/`**
  - Criado `pty/runner.go` com `Runner` implementando `domain.ProcessRunner`
  - `loop/runtime_pty.go` reduzido a wrapper

- [x] **T14. Criar `internal/runtime/tmux/` e mover `runtime_tmux.go`**
  - Criado `runtime/tmux/runner.go` com `Runner` + `NewRunner()` + `SessionManager`
  - `loop/runtime_tmux.go` reduzido a wrapper com delegates

- [x] **T15. Mover runtime composition para `internal/orchestration/pipeline/`**
  - Criado `pipeline/runtime_composed.go` com `composedRuntime`, `defaultAgents()`, `BuildAgentRuntime`
  - `loop/runtime.go` esvaziado (sem chamadores restantes no loop)

- [x] **T16. Mover runtime_agents bridge para `internal/agent/runtime/`**
  - Criado `agent/runtime/registry_runtime.go` com `RegistryRuntime`, `NewRegistryRuntime`
  - `loop/runtime_agents.go` esvaziado (sem chamadores restantes no loop)

## Fase 6 — Extrair agent specs para providers

- [x] **T17. Mover `agent_claude.go` para `internal/providers/claude/`**
  - Criado `providers/claude/spec.go` com `AgentSpec` exportado implementando `domain.AgentSpec`
  - `loop/agent_claude.go` reduzido a wrapper delegando para `claude.AgentSpec`

- [x] **T18. Mover `agent_codex.go` para `internal/providers/codex/`**
  - Criado `providers/codex/spec.go` com `AgentSpec` exportado implementando `domain.AgentSpec`
  - `loop/agent_codex.go` reduzido a wrapper delegando para `codex.AgentSpec`

## Fase 7 — Limpeza final

- [x] **T19. Deletar stubs de delegação do loop**
  - Todos os stubs (types.go, transition.go, graph.go, parse.go, project.go, etc.) removidos com o diretório

- [x] **T20. Deletar `internal/loop/`**
  - Diretório removido integralmente — zero referências residuais
  - Build e testes passam limpo sem loop

- [x] **T21. Atualizar `internal/cli/` para novos imports**
  - `cli/run.go`: `loop` → `domain`, `pipeline`, `workspace`
  - `cli/plan.go`: `loop` → `domain`, `state`

- [x] **T22. Atualizar `docs/architecture.md`**
  - Package tree atualizado: removidos `loop/`, `orchestrator/`, `paths/`; adicionados `app/`, `runtime/tmux/`
  - Descrições de `pipeline/`, `domain/`, `providers/` atualizadas
  - `loop.Runner` → `pipeline.Runner`

- [ ] **T23. Absorver `internal/config/` em `internal/app/`** *(opcional)*
  - Config loading é parte do bootstrap de wiring
  - Só necessário para strictness máxima com a RFC

---

## Notas

- **Ordem importa:** cada fase só move código cujos pacotes destino já existem.
- **Transição:** durante a migração, usar type aliases e delegações temporárias.
- **Teste contínuo:** `go build ./... && go test ./...` após cada tarefa.
- **Providers futuros:** `gemini/` e `ollama/` são marcados como "futuro" na RFC — não fazem parte deste plano.
- **Total:** 23 tarefas (T23 opcional), organizadas em 7 fases.
