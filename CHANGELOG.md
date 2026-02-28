# Changelog

## 1.0.0 (2026-02-28)


### ⚠ BREAKING CHANGES

* delete internal/loop — T19-T22 of RFC-009
* `praetor run` is now `praetor plan run`.

### Features

* add ASCII logo with forest gradient to CLI help ([5f64151](https://github.com/opus-domini/praetor/commit/5f64151d3bd293807926a773490cce0db49f647d))
* add config CLI subcommands and rewrite provider documentation ([9ed16ad](https://github.com/opus-domini/praetor/commit/9ed16add27198335efb9907fd976de10802ef154))
* add live JSONL formatter for human-readable tmux pane output ([edf0da0](https://github.com/opus-domini/praetor/commit/edf0da0a9ac9d0ae756d46063ddbc8f517f5eaef))
* add metadata feedback to praetor exec and separate one-shot Execute path ([2be51ae](https://github.com/opus-domini/praetor/commit/2be51ae847b5d656c082c229e6b4fcfc2db763eb))
* add prompt template system with go:embed and project overlay ([07caea7](https://github.com/opus-domini/praetor/commit/07caea73e5ba2a6f1000a988774fee07bdab159f))
* consolidate RFC-010 architecture and docs ([3017041](https://github.com/opus-domini/praetor/commit/3017041d3265e9e43c49b246e8f5235c750c9e01))
* enhance documentation search and code copy functionality ([8aadbbf](https://github.com/opus-domini/praetor/commit/8aadbbf969ae39ee6a5d857f0c1bf6e80142a76a))
* evolve orchestration flow and improve CLI UX ([6ce95bd](https://github.com/opus-domini/praetor/commit/6ce95bdbc7cba404c0bde4e9475edeea6ac92c78))
* implement RFC-001 gap closure across providers and loop ([03d724b](https://github.com/opus-domini/praetor/commit/03d724b595ad6ed8b5af98abdcd1ac67ff10d071))
* implement RFC-013 phases 2-5 (fallback, middleware, observability, routing) ([f48ae3b](https://github.com/opus-domini/praetor/commit/f48ae3b8faf989a4070b43b3e288e3ccc1fabd27))
* implementar RFC-009 com agent abstraction, PTY, FSM e snapshots ([6191ebc](https://github.com/opus-domini/praetor/commit/6191ebc383502cce33bb616c520ae022b834cda2))
* **loop:** add tmux-first orchestration runtime ([1353447](https://github.com/opus-domini/praetor/commit/13534474291c486f606bab10c7611aeb57411d4c))
* **loop:** formalize task lifecycle as explicit state machine (RFC-006) ([b3f34a0](https://github.com/opus-domini/praetor/commit/b3f34a0a12d4878576f9f37553e24060ca434012))
* **loop:** replace git safety with worktree isolation and add config ([4bef952](https://github.com/opus-domini/praetor/commit/4bef9520413124e4d40b1c71a8b37d2ab2f7f5a8))
* **loop:** stream agent output in real-time via NDJSON ([1f44f3a](https://github.com/opus-domini/praetor/commit/1f44f3ab1986f21a0de6f29c4cf74026082cdd39))
* XDG-compliant paths, project context, and migration (RFC-008) ([7370ac0](https://github.com/opus-domini/praetor/commit/7370ac042613a2aa6c39c036cf9a0ba581297751))


### Bug Fixes

* add OneShot path to codex Execute for praetor exec ([160331b](https://github.com/opus-domini/praetor/commit/160331bc95dc7240f252b83556e3fac036a6addd))
* harden loop runtime consistency and config validation ([10e7155](https://github.com/opus-domini/praetor/commit/10e715573f422e60e7bdb37fc109f90887450755))
* move run under plan, show errors on missing args, fix tmux visibility ([82a711d](https://github.com/opus-domini/praetor/commit/82a711d1f153f0efbf2c74278c5ca6de634d6443))
* preserve streaming Execute for plan pipeline, use OneShot for praetor exec ([0abfa8c](https://github.com/opus-domini/praetor/commit/0abfa8c916088ae82f906e3f07ff525e1e4c963b))
* resolve golangci-lint issues from RFC-009 migration ([4cf0fce](https://github.com/opus-domini/praetor/commit/4cf0fcef963a6d62d3f3a93bffba87d1612e1eb8))
* update logo format and dimensions in README ([e3dc4b5](https://github.com/opus-domini/praetor/commit/e3dc4b52c96463beda432973d25537eb6ae43664))
* update meta description and stylesheet link in index.html ([f537f96](https://github.com/opus-domini/praetor/commit/f537f96435a472aba9f95beb05f0ece95c131075))


### Refactors

* **cli:** redesign command hierarchy for clarity ([299b667](https://github.com/opus-domini/praetor/commit/299b667d2c63960ac95bb8a32801c261b6c56815))
* consolidate all state under single home directory and use slug-based plan identity ([2ece7a4](https://github.com/opus-domini/praetor/commit/2ece7a434e9a6f904546e425eea8659fd0b08d6d))
* delete internal/loop — T19-T22 of RFC-009 ([0317eee](https://github.com/opus-domini/praetor/commit/0317eeee2bb8bf2b931ad5bf456ca68c4757b262))
* dissolve loop — T01-T07 of RFC-009 directory restructuring ([9ad7d75](https://github.com/opus-domini/praetor/commit/9ad7d7586b9d9a055486786a1c415557cefd0903))
* dissolve loop core — T08-T11, T15-T16 of RFC-009 ([07e73b0](https://github.com/opus-domini/praetor/commit/07e73b0c3d7fd2791a62d37687562d35e0badbf6))
* extract domain types and enforce RFC-009 directory structure ([19b7cb4](https://github.com/opus-domini/praetor/commit/19b7cb4f8b7862b72ddcc9e1f3f4c416de2a6707))
* extract runtimes and agent specs — T12-T14, T17-T18 of RFC-009 ([aae4cbe](https://github.com/opus-domini/praetor/commit/aae4cbe504c966deb123cdbe133476a7fd6ea191))
* **loop:** replace sha1 with sha256 hashes ([bdf8565](https://github.com/opus-domini/praetor/commit/bdf85658587ff161af2cdfb6d7a0a5d422d81931))
* **loop:** separate agent specs from process runners (RFC-005) ([9009dc7](https://github.com/opus-domini/praetor/commit/9009dc714795d900df30ed05a0c36d7b56838080))
* **loop:** simplify runner flow and split store modules ([dc4059c](https://github.com/opus-domini/praetor/commit/dc4059ce50d61b3d27e1667ff5111dffa2760d8c))
* remove all legacy migration code and backward-compatibility surfaces ([1377c84](https://github.com/opus-domini/praetor/commit/1377c848d3f5be96e71854e4240837cf0623fb2e))


### Documentation

* document task state machine in architecture and loop guides ([d755907](https://github.com/opus-domini/praetor/commit/d755907e9366e2bf4e053507ced2dcaa5fa54f2e))
* revamp README with new project overview ([20c5417](https://github.com/opus-domini/praetor/commit/20c5417b0342e0e98a7a915d49e9c204df840b39))
* rewrite all project documentation ([666dc10](https://github.com/opus-domini/praetor/commit/666dc10da5cbc8edfc50f0e46593cbbbffa9aec4))
* update all docs for post-RFC-009 directory structure ([1bc96b4](https://github.com/opus-domini/praetor/commit/1bc96b4949fa9aba95be8d92b6ef8166a41362bf))
