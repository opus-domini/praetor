package commands

const planCreateContent = `# Plan Create

Create a structured execution plan for the given objective using praetor.

## Instructions

1. Analyze the objective and understand the codebase structure
2. Break down the work into atomic, dependency-aware tasks
3. Each task must have clear acceptance criteria
4. Present the plan for human approval before any execution

## Allowed Tools

Read, Glob, Grep, Bash(git log), Bash(git diff), Bash(make test)

## Output Format

Return a JSON plan matching the praetor plan schema:

` + "```json" + `
{
  "name": "plan-name",
  "cognitive": {
    "assumptions": [],
    "open_questions": [],
    "failure_modes": []
  },
  "settings": {
    "agents": {
      "executor": {"agent": "codex"},
      "reviewer": {"agent": "claude"}
    }
  },
  "tasks": [
    {
      "id": "TASK-001",
      "title": "Task title",
      "description": "What to do",
      "acceptance": ["Criteria 1"]
    }
  ]
}
` + "```" + `

Save the plan with: praetor plan create "<brief>"
`

const planRunContent = `# Plan Run

Execute an existing praetor plan.

## Instructions

1. List available plans: praetor plan list
2. Check plan status: praetor plan status <slug>
3. Run the plan: praetor plan run <slug>
4. Monitor progress and report the outcome

## Allowed Tools

Bash(praetor plan list), Bash(praetor plan status *), Bash(praetor plan run *), Bash(praetor plan diagnose *)
`

const reviewTaskContent = `# Review Task

Review the output of a praetor task execution.

## Instructions

1. Read the executor output and git diff
2. Verify acceptance criteria are met
3. Check for quality gate compliance (tests, lint)
4. Verify changes follow project conventions (AGENTS.md)
5. Return a clear PASS or FAIL verdict with reasoning

## Allowed Tools

Read, Glob, Grep, Bash(make test), Bash(make lint), Bash(git diff)
`

const doctorContent = `# Doctor

Check the health and availability of all AI agent providers.

## Instructions

Run the praetor doctor command and report results:

praetor doctor

## Allowed Tools

Bash(praetor doctor)
`

const diagnoseContent = `# Diagnose

Inspect and debug a praetor plan run.

## Instructions

1. Identify the plan: praetor plan list
2. Check current status: praetor plan status <slug>
3. Run diagnostics: praetor plan diagnose <slug> --query all
4. Analyze errors, stalls, fallbacks, and costs
5. Suggest fixes for any issues found

## Allowed Tools

Bash(praetor plan list), Bash(praetor plan status *), Bash(praetor plan diagnose *), Read
`
