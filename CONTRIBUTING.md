# Contributing to Praetor

Thank you for contributing to Praetor. We prioritize reliability, clear UX, and predictable APIs.

## Before You Start

- Check existing issues and pull requests to avoid duplicate work.
- Open an issue first for large changes or API-affecting proposals.
- Keep changes focused: one logical improvement per PR.

## Local Setup

1. Fork and clone your fork:
   ```bash
   git clone https://github.com/<your-user>/praetor.git
   cd praetor
   ```
2. Install development tools:
   ```bash
   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
   go install golang.org/x/vuln/cmd/govulncheck@latest
   ```
3. Create a feature branch:
   ```bash
   git checkout -b feat/short-descriptive-name
   ```

## Development Workflow

- Implement changes with minimal complexity and clear behavior.
- Add or update tests alongside code changes.
- Run the quality pipeline before opening or updating a PR:
  ```bash
  make ci-fast    # fmt + lint + tidy + test + security
  ```
- For broader verification, also run:
  ```bash
  make ci-full    # ci-fast + test-coverage + benchmark
  ```
- Targeted commands are available when iterating:
  ```bash
  make test             # run tests
  make test-coverage    # tests with race detection + coverage
  make benchmark        # run benchmarks
  make lint             # lint code
  make fmt              # format code
  make security         # run govulncheck
  ```
- `make ci` remains the full local gate and should pass before merge.

## Coding and Testing Expectations

- Follow idiomatic Go and existing project conventions.
- Prefer explicit, readable APIs over clever abstractions.
- Keep tests deterministic, focused, and parallel (`t.Parallel()`) unless using `t.Setenv()`.
- Avoid unnecessary dependencies.
- Update docs when commands, APIs, or behavior change.

## Commit Message Guidelines

Use [Conventional Commits](https://www.conventionalcommits.org/) to keep automated versioning and changelogs consistent.
Use concise, imperative commits. Preferred prefixes:

- `feat: add ...`
- `fix: handle ...`
- `refactor: ...`
- `perf: ...`
- `test: cover ...`
- `docs: ...`
- `chore: ...`
- `ci: ...`

Breaking changes:

- `feat!: ...` or `fix!: ...`
- include `BREAKING CHANGE:` in the commit body when needed.

## Pull Request Guidelines

- Describe what changed, why, and how it was validated.
- Link related issues (for example, `Closes #42`).
- Include examples or output snippets when behavior changes.
- Call out breaking changes clearly.

## Code Review

Maintainers review PRs as time permits. Please keep discussions technical, objective, and collaborative.

## License

By contributing, you agree that your contributions are licensed under the project [LICENSE](LICENSE).
