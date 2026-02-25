<div align="center">
    <img src="docs/assets/images/logo.png" alt="Logo Praetor" width="500"/>
    <hr />
    <p>Lead. Delegate. Dominate.</p>
</div>

`praetor` is a Go CLI for orchestrating multiple AI agent providers from a single execution surface.

The initial scope is provider orchestration for:

- Claude Code
- Codex

## Repository layout

```text
.
├── cmd/
│   └── praetor/                 # CLI entrypoint
├── internal/
│   ├── orchestrator/            # Provider contracts and dispatch engine
│   └── providers/
│       ├── claude/              # Claude SDK port + adapter
│       └── codex/               # Codex SDK port + adapter
├── docs/                        # Project documentation
└── .github/                     # CI/CD and repository automation
```

This layout follows idiomatic Go CLI conventions:

- one binary entrypoint in `cmd/`
- private application code in `internal/`
- clear package ownership and minimal coupling

## Quick start

Prerequisite: Go `1.26`.

Build the CLI:

```bash
go build -o build/praetor ./cmd/praetor
```

List supported providers:

```bash
./build/praetor providers
```

Run with Codex:

```bash
./build/praetor run --provider codex --prompt "Reply with OK"
```

Run with Claude:

```bash
echo "Reply with OK" | ./build/praetor run --provider claude
```

## Development

Run formatting, lint, and tests:

```bash
make fmt
make lint
make test
```

Run the full local CI target:

```bash
make ci
```

## Documentation

- Canonical project documentation lives in `docs/` (docsify).
- Provider-level documentation lives in `docs/providers/`.
- `internal/providers/*/README.md` files are intentionally minimal pointers to canonical docs.

## Design principles

- Keep packages small and focused.
- Prefer explicit dependencies over global state.
- Keep provider-specific logic isolated behind a common interface.
- Build a simple core that can evolve without breaking package boundaries.
