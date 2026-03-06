# Operations Runbook

Operational flow for local execution quality using `praetor plan eval` (plan level) and `praetor eval` (project level).

## Daily routine

```bash
praetor doctor
praetor eval
```

## What is evaluated

`praetor plan eval` inspects one local run end-to-end:

- final task status (`done` / `failed`)
- required gate evidence (`tests`, `lint`, `standards`, or plan-defined gates)
- parser/contract failures (`executor_parse_error`, `reviewer_parse_error`)
- stalls and retries per task
- cost and duration per task/run

`praetor eval` aggregates the latest plan runs for the project and produces a project verdict (`pass|warn|fail`).

## User journey (step by step)

### 1. Execute the plan

```bash
praetor plan run my-plan
```

Expected:

- Tasks are executed with executor/reviewer orchestration.
- Runtime artifacts are written under the local project state (`runtime/<run-id>/...`).

### 2. Evaluate this plan (latest run)

```bash
praetor plan eval my-plan
```

Expected:

- Header with plan slug, run id, outcome, and final verdict.
- Summary with acceptance rate, gate failures/missing evidence, parse errors, stalls, retries, and cost.
- Task table with one row per task and explicit failure reasons.

### 3. Evaluate a specific run id

```bash
praetor plan eval my-plan --run-id run-123 --format json
```

Expected:

- Full machine-readable report for that exact run.
- Useful for forensic comparison between two different runs of the same plan.

### 4. Aggregate project health

```bash
praetor eval
```

Expected:

- Project verdict with counts of plans in `pass|warn|fail`.
- Global acceptance rate and cost/retry indicators.
- Table listing each plan verdict and key risk signals.

### 5. Limit analysis window

```bash
praetor eval --window 72h --format json
```

Expected:

- Only recent plan runs are considered.
- Output is suitable for dashboards/scripts.

## Fast triage sequence

```bash
# 1) Identify problematic plan(s)
praetor eval

# 2) Drill into a specific plan
praetor plan eval <slug>

# 3) Inspect diagnostic stream for root cause
praetor plan diagnose <slug> --query errors --format json
praetor plan diagnose <slug> --query stalls
```

## Common failure signatures

- `required gate ... status=FAIL`
  - Action: run the same gate command in the task workdir and fix before rerun.
- `missing required gate ...`
  - Action: ensure gate mapping is configured and host gate execution is enabled.
- `parse error event(s)`
  - Action: inspect executor/reviewer output contract and retry policy.
- `task stalled`
  - Action: inspect feedback loop, fallback behavior, prompt budget truncation, and cost budget events.

## Operational rule

Use `praetor plan eval` as release-quality evidence for one plan, and `praetor eval` as the local project control panel before merge/deploy decisions.
