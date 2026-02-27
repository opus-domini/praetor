# RFC-010: Consolidação Arquitetural - Plano de Execução e Checkpoints

- **RFC:** [`.rfcs/RFC-010-consolidacao-arquitetural-pos-rfc009.md`](/home/hugo/Workspace/opus-domini/praetor/.rfcs/RFC-010-consolidacao-arquitetural-pos-rfc009.md)
- **Data:** 2026-02-26
- **Objetivo:** Consolidar a arquitetura pos-RFC-009, eliminar duplicidades de runtime/contrato e endurecer resiliencia operacional.

## Resultado esperado

1. Um unico contrato de agente para todos os runners (`tmux`, `pty`, `direct`).
2. Loop principal executado via FSM funcional reutilizavel.
3. PTY com estrategia clara de fallback e rastreabilidade.
4. Contexto de workspace com normalizacao estruturada.
5. Snapshot local com integridade, retencao e retomada explicita.

---

## Backlog completo

## Fase 0 - Baseline e seguranca de mudanca

- [x] **T001. Congelar baseline de qualidade**
  - Executar `go test ./...`
  - Registrar tempo total e testes falhos (se houver)
  - Salvar baseline no PR/issue de acompanhamento

- [x] **T002. Mapear pontos de entrada de runtime**
  - Listar todos os chamadores de `BuildAgentRuntime`
  - Listar todos os caminhos `domain.AgentSpec` e `internal/agent.Agent`
  - Confirmar onde existe duplicidade ativa

- [x] **T003. Criar matriz de suporte por runner**
  - Tabela runner x agente x fase (plan/execute/review)
  - Registrar comportamento esperado para `codex`, `claude`, `gemini`, `ollama`

## Fase 1 - Convergencia de contratos (agents)

- [x] **T004. Criar adapter `domain.AgentSpec -> agents.Agent`**
  - Novo adapter para transicao
  - Cobrir parse de output/custo e metadados de capacidade

- [x] **T005. Encapsular selecao de backend em `agents`**
  - Centralizar criacao de agentes em um ponto unico
  - Eliminar decisao distribuida entre pipeline/providers

- [x] **T006. Migrar `BuildAgentRuntime` para caminho unico**
  - Manter interface publica atual
  - Direcionar `tmux`, `pty` e `direct` para o mesmo contrato interno

- [x] **T007. Remover dependencia direta de `internal/providers/catalog.go` no fluxo principal**
  - Migrar dependencias para novo contrato

- [x] **T008. Atualizar testes de contrato de agentes**
  - Contract tests unificados por fase (Plan/Execute/Review)
  - Cobrir REST e CLI com os mesmos asserts

- [x] **T009. Limpeza de duplicidades apos convergencia**
  - Remover caminhos mortos identificados em T002
  - Atualizar imports e docs internas

## Fase 2 - FSM operacional do loop principal

- [x] **T010. Integrar `internal/orchestration/fsm.Run` ao runner**
  - Substituir loop manual por engine generica
  - Preservar semantica atual de cancelamento/erro

- [x] **T011. Quebrar `runIteration` em microestados**
  - Estados alvo: `selectTask`, `executeTask`, `reviewTask`, `applyOutcome`, `persist`
  - Remover blocos condicionais extensos

- [x] **T012. Introduzir limite global de transicoes (`maxTransitions`)**
  - Guard rail adicional anti-loop
  - Mensagem de erro clara e checkpoint correspondente

- [x] **T013. Persistir snapshot por fronteira de estado**
  - Cada transicao importante deve gerar evento + snapshot
  - Garantir ordem consistente de escrita

- [x] **T014. Cobertura de testes da FSM**
  - Sequencia feliz
  - Rejeicao de review
  - Cancelamento por contexto
  - Exaustao de retry
  - Excesso de transicoes

## Fase 3 - PTY hardening e estrategia de execucao

- [x] **T015. Definir enum de estrategia de execucao**
  - `structured`, `process`, `pty`
  - Expor estrategia efetiva no resultado da execucao

- [x] **T016. Implementar fallback explicito**
  - Preferencia: `structured -> process -> pty`
  - Condicoes de fallback por capacidade e erro

- [x] **T017. Melhorar portabilidade do backend PTY**
  - Encapsular dependencia de `script`
  - Emitir erro orientado quando recurso nao existir

- [x] **T018. Persistir estrategia de execucao em checkpoint/snapshot**
  - Incluir estrategia no `events.log` e/ou `snapshot.json`
  - Facilitar auditoria pos-falha

- [x] **T019. Testes de comportamento PTY**
  - Comando com TTY obrigatorio
  - Comando sem TTY
  - Falha em modo nao-PTY com fallback para PTY

## Fase 4 - Contexto de workspace estruturado

- [x] **T020. Definir schema leve para `praetor.yaml`**
  - Campos minimos: `version`, `instructions`, `constraints`, `test_commands`
  - Suporte ao manifesto Markdown

- [x] **T021. Implementar normalizacao de contexto**
  - Separar contexto bruto de contexto normalizado para prompts
  - Manter truncamento e prioridade atual de arquivos

- [x] **T022. Injetar contexto estruturado nas 3 fases**
  - Planejamento
  - Execucao
  - Revisao

- [x] **T023. Persistir metadados do manifesto no snapshot**
  - `manifest_path`
  - hash de conteudo
  - indicador de truncamento

- [x] **T024. Testes do fluxo de manifesto**
  - `praetor.yaml` presente
  - fallback para `praetor.md`
  - manifesto ausente
  - manifesto invalido

## Fase 5 - Ciclo de vida de snapshots

- [x] **T025. Adicionar checksum de integridade do snapshot**
  - Calcular hash de `snapshot.json`
  - Persistir em `meta.json`

- [x] **T026. Validar integridade no recovery**
  - Ignorar snapshot corrompido
  - Logar aviso explicito de corrupcao detectada

- [x] **T027. Implementar retencao de runs locais**
  - Politica configuravel (`keep-last-n-runs`)
  - Limpeza automatica segura

- [x] **T028. Criar comando explicito de retomada**
  - `praetor plan resume`
  - Selecionar ultima execucao valida do plano

- [x] **T029. Testes de interrupcao e retomada**
  - SIGINT
  - falha abrupta simulada
  - retomada com estado consistente

## Fase 6 - Finalizacao e governanca

- [x] **T030. Atualizar documentacao tecnica**
  - `docs/architecture.md`
  - `docs/orchestration.md`
  - `docs/providers/README.md`

- [x] **T031. Atualizar ajuda de CLI e exemplos**
  - Incluir `resume` e politicas de retencao
  - Documentar estrategia de execucao/fallback

- [x] **T032. Remover codigo obsoleto**
  - Lista de APIs/pacotes removidos
  - Limpeza por versao

- [x] **T033. Rodada final de regressao**
  - `go test ./...`
  - Smoke test de `plan run`, `plan status`, `plan resume`

- [x] **T034. Encerramento formal da RFC-010**
  - Consolidar evidencias
  - Marcar criterios de aceite como atendidos

---

## Checkpoints de acompanhamento

## CP0 - Baseline estabelecida

- **Quando:** fim da Fase 0
- **Entrada:** T001-T003 concluidas
- **Saida obrigatoria:**
  - baseline de testes registrada
  - mapa de duplicidades runtime/contrato aprovado
  - matriz de suporte publicada

## CP1 - Contrato unico definido

- **Quando:** fim da Fase 1 (T004-T009)
- **Saida obrigatoria:**
  - `BuildAgentRuntime` usando caminho unico de contrato
  - testes de contrato passando para REST/CLI
  - duplicidades principais removidas ou marcadas para remocao

## CP2 - FSM operacional consolidada

- **Quando:** fim da Fase 2 (T010-T014)
- **Saida obrigatoria:**
  - loop principal rodando via `fsm.Run`
  - `maxTransitions` ativo
  - cobertura de testes dos microestados

## CP3 - PTY robusto com fallback rastreavel

- **Quando:** fim da Fase 3 (T015-T019)
- **Saida obrigatoria:**
  - estrategia de execucao registrada por tarefa
  - fallback funcional validado em teste
  - comportamento PTY documentado

## CP4 - Contexto de workspace governado

- **Quando:** fim da Fase 4 (T020-T024)
- **Saida obrigatoria:**
  - schema de `praetor.yaml` ativo
  - injecao normalizada nas fases cognitivas
  - metadados de manifesto persistidos em snapshot

## CP5 - Snapshot lifecycle fechado

- **Quando:** fim da Fase 5 (T025-T029)
- **Saida obrigatoria:**
  - checksum de snapshot validado no recovery
  - retencao local funcionando
  - comando `plan resume` operante

## CP6 - RFC encerrada

- **Quando:** fim da Fase 6 (T030-T034)
- **Saida obrigatoria:**
  - docs atualizadas
  - regressao final verde
  - RFC-010 marcada como implementada

## Status atual (2026-02-26)

Checkpoint: CP0  
Status: completed  
Concluido: T001, T002, T003  
Pendencias: nenhuma  
Riscos: nenhum bloqueador ativo  
Decisao: avancar fase

Checkpoint: CP1  
Status: completed  
Concluido: T004, T005, T006, T007, T008, T009  
Pendencias: nenhuma  
Riscos: nenhum bloqueador ativo  
Decisao: avancar fase

Checkpoint: CP2  
Status: completed  
Concluido: T010, T011, T012, T013, T014  
Pendencias: nenhuma  
Riscos: nenhum bloqueador ativo  
Decisao: avancar fase

Checkpoint: CP3  
Status: completed  
Concluido: T015, T016, T017, T018, T019  
Pendencias: nenhuma  
Riscos: nenhum bloqueador ativo  
Decisao: avancar fase

Checkpoint: CP4  
Status: completed  
Concluido: T020, T021, T022, T023, T024  
Pendencias: nenhuma  
Riscos: nenhum bloqueador ativo  
Decisao: avancar fase

Checkpoint: CP5  
Status: completed  
Concluido: T025, T026, T027, T028, T029  
Pendencias: nenhuma  
Riscos: nenhum bloqueador ativo  
Decisao: avancar fase

Checkpoint: CP6  
Status: completed  
Concluido: T030, T031, T032, T033, T034  
Pendencias: nenhuma  
Riscos: nenhum bloqueador ativo  
Decisao: encerrar RFC-010

---

## Cadencia de acompanhamento

1. **Ritmo:** checkpoint formal a cada fim de fase.
2. **Reuniao curta diaria:** progresso por tarefa (Txxx), bloqueios e risco.
3. **Relatorio por checkpoint:** status (`on-track`, `at-risk`, `blocked`), evidencias e proxima fase.

## Template de status por checkpoint

```text
Checkpoint: CPx
Status: on-track | at-risk | blocked
Concluido: Txxx, Tyyy, Tzzz
Pendencias: Taaa, Tbbb
Riscos: <lista curta>
Decisao: avancar fase | manter fase | replanejar
```

---

## Dependencias criticas

1. T004-T009 antes de T010 (FSM depende de contrato estabilizado).
2. T010-T014 antes de T025-T029 (snapshot lifecycle depende de fronteiras FSM).
3. T015-T019 pode ocorrer em paralelo parcial com T020-T024.
4. T033-T034 so apos conclusao de todas as fases anteriores.

---

## Criterios de aceite (rastreados no plano)

1. Contrato unico de agente em todos os runners.
2. Loop FSM funcional com guard rails.
3. PTY com fallback rastreavel e previsivel.
4. Contexto local estruturado e auditavel.
5. Recovery deterministico apos interrupcao.
