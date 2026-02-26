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
| `internal/orchestrator/`| Deletado                         | Legacy, zero importadores                  |
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
  - Criado `state/migrate.go` com `MigrateLegacyState`
  - `loop/migrate.go` reduzido a delegação

## Fase 3 — Extrair domain e prompts do loop

- [x] **T07. Mover plan loading/validation para `internal/domain/`**
  - `LoadPlan`, `ValidatePlan`, `NewPlanFile` adicionados a `domain/plan.go`
  - `loop/plan.go` reduzido a delegações

- [ ] **T08. Mover tipos cognitivos para `internal/orchestration/pipeline/`**
  - `loop/cognitive_agent.go`: `CognitiveAgent`, `PlanRequest`, `ExecuteRequest`, `ReviewRequest`
  - Inclui `NewCognitiveAgent` e implementação `runtimeCognitiveAgent`

- [ ] **T09. Mover prompts para `internal/orchestration/pipeline/`**
  - `loop/prompts.go`: builders de system prompts (executor, reviewer, planner)
  - Regras de prompt pertencem ao pipeline cognitivo

## Fase 4 — Extrair runner e policies do loop

- [ ] **T10. Mover runner para `internal/orchestration/pipeline/`**
  - `loop/runner.go`, `loop/runner_outcome.go`, `loop/runner_policy.go`
  - ~1200 linhas — motor central do pipeline Plan/Execute/Review
  - Maior tarefa da migração; requer atenção a dependências circulares

- [ ] **T11. Mover output/renderer para `internal/cli/`**
  - `loop/output.go`: `Renderer`, `NewRenderer`, métodos de formatação
  - Renderização de terminal pertence à camada de apresentação

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

- [ ] **T15. Mover runtime composition para `internal/orchestration/pipeline/`**
  - `loop/runtime.go`: `composedRuntime`, `defaultAgents()` — montagem de runtime por modo

- [ ] **T16. Mover runtime_agents bridge para `internal/agents/`**
  - `loop/runtime_agents.go`: `registryRuntime` faz ponte entre `agents.Registry` e `AgentRuntime`

## Fase 6 — Extrair agent specs para providers

- [x] **T17. Mover `agent_claude.go` para `internal/providers/claude/`**
  - Criado `providers/claude/spec.go` com `AgentSpec` exportado implementando `domain.AgentSpec`
  - `loop/agent_claude.go` reduzido a wrapper delegando para `claude.AgentSpec`

- [x] **T18. Mover `agent_codex.go` para `internal/providers/codex/`**
  - Criado `providers/codex/spec.go` com `AgentSpec` exportado implementando `domain.AgentSpec`
  - `loop/agent_codex.go` reduzido a wrapper delegando para `codex.AgentSpec`

## Fase 7 — Limpeza final

- [ ] **T19. Deletar stubs de delegação do loop**
  - `types.go`, `transition.go`, `graph.go`, `parse.go`, `project.go`
  - São apenas aliases/delegações para `domain/`; consumidores passam a importar direto

- [ ] **T20. Deletar `internal/loop/`**
  - Diretório deve estar vazio após T01–T19
  - Confirmar zero referências residuais

- [ ] **T21. Atualizar `internal/cli/` para novos imports**
  - Substituir todos os `loop.X` por imports dos pacotes destino
  - `domain.`, `state.`, `pipeline.`, `app.`, `cli.Renderer`

- [ ] **T22. Atualizar `docs/architecture.md`**
  - Refletir estrutura final sem `loop/`, `orchestrator/`, `paths/`

- [ ] **T23. Absorver `internal/config/` em `internal/app/`** *(opcional)*
  - Config loading é parte do bootstrap de wiring
  - Só necessário para strictness máxima com a RFC

---

## Notas

- **Ordem importa:** cada fase só move código cujos pacotes destino já existem.
- **Backward compatibility:** durante a migração, usar type aliases e delegações temporárias.
- **Teste contínuo:** `go build ./... && go test ./...` após cada tarefa.
- **Providers futuros:** `gemini/` e `ollama/` são marcados como "futuro" na RFC — não fazem parte deste plano.
- **Total:** 23 tarefas (T23 opcional), organizadas em 7 fases.
