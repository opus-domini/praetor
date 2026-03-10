# Changelog

## [0.1.4](https://github.com/opus-domini/praetor/compare/v0.1.3...v0.1.4) (2026-03-10)


### Features

* **commands:** add praetor-workflow command with end-to-end guide ([279a060](https://github.com/opus-domini/praetor/commit/279a060d37f2fa2d6c69381854c0392cc996e3bf))

## [0.1.3](https://github.com/opus-domini/praetor/compare/v0.1.2...v0.1.3) (2026-03-10)


### Bug Fixes

* strip agent nesting-detection env vars from spawned processes ([af94301](https://github.com/opus-domini/praetor/commit/af943016f21121bbeaa1996cf8028a6077637712))


### Refactors

* centralize agent nesting env vars in domain.CleanAgentEnv ([a6a4217](https://github.com/opus-domini/praetor/commit/a6a4217fdc20ad5718c11125e4b15e67c4ce5fb0))

## [0.1.2](https://github.com/opus-domini/praetor/compare/v0.1.1...v0.1.2) (2026-03-09)


### Features

* **mcp:** add exec and plan_run tools for agent action capability ([d89cc1c](https://github.com/opus-domini/praetor/commit/d89cc1cb733470fcccb836b778da86bc096b1b9a))


### Bug Fixes

* **mcp:** doctor respects config overrides and improve tool descriptions ([a1417f9](https://github.com/opus-domini/praetor/commit/a1417f98701ccdc4a090ab8d235f1b99123ecac3))

## [0.1.1](https://github.com/opus-domini/praetor/compare/v0.1.0...v0.1.1) (2026-03-09)


### Features

* add LM Studio provider ([8948c41](https://github.com/opus-domini/praetor/commit/8948c414099d8225751edf8edd40da8bfe7e0965))
* add make install target ([1e0a0d4](https://github.com/opus-domini/praetor/commit/1e0a0d4f4b96091424ba117dca07b64b3e557fc9))
* add MCP server and shared agent commands ([5f0f8ef](https://github.com/opus-domini/praetor/commit/5f0f8ef86a417cc3a2a4937e8b93bc5ea5cdeab9))
* add per-task agent/model overrides, tool constraints, and standards gate ([dc4e023](https://github.com/opus-domini/praetor/commit/dc4e023e881081e6530ce553f69968428816437d))
* add praetor init command and enrich CLI help text ([8f7b299](https://github.com/opus-domini/praetor/commit/8f7b299278c1a1653ebd7b22261b016bdc20fc08))
* center mermaid diagrams and add zoom/pan controls ([b2bd4a7](https://github.com/opus-domini/praetor/commit/b2bd4a7a40103835894a7de0f9476535bac0288c))
* strengthen local operations flow, evals, and planner UX ([b67b81a](https://github.com/opus-domini/praetor/commit/b67b81adfaea86eaf019bd078abafe52ecc067eb))


### Bug Fixes

* **ci:** remove release-as pin causing duplicate v0.1.0 releases ([e651815](https://github.com/opus-domini/praetor/commit/e65181549e03bd9e5ca22850ee082de989707220))
* correct all orchestration diagrams against actual implementation ([5549f47](https://github.com/opus-domini/praetor/commit/5549f4746255acbe27ed34a62186f5804d2781d8))
* resolve pre-existing lint warnings and data races ([391ac0f](https://github.com/opus-domini/praetor/commit/391ac0f5cadf1d9b241113e53953b23a68d777a4))
* use &lt;br&gt; instead of \n for line breaks in mermaid diagrams ([978da7c](https://github.com/opus-domini/praetor/commit/978da7cac0fa1ed5f6dd077f1de94310f5d29fb3))


### Refactors

* redesign praetor init as project installer, not project creator ([ee9b0e6](https://github.com/opus-domini/praetor/commit/ee9b0e66f40db6a983a850e4a02cf48c29001fdf))
* remove plan schema versioning and backward compatibility ([0d37e8c](https://github.com/opus-domini/praetor/commit/0d37e8c131564ce3cc78acff732d44a2a9a5ec2f))
* remove praetor commands CLI, consolidate into init ([85e1d21](https://github.com/opus-domini/praetor/commit/85e1d2147b87050b5745b03c8f5253cab8220cee))
* standardize CLI output through Renderer and normalize UX ([5946405](https://github.com/opus-domini/praetor/commit/59464050f5c844acf38f886dba933be364bd98f4))


### Documentation

* add LICENSE and CONTRIBUTING guidelines ([2bc4223](https://github.com/opus-domini/praetor/commit/2bc4223f208357947e030f8ddf5a7ec665239927))
* add LM Studio to documentation site ([d05f672](https://github.com/opus-domini/praetor/commit/d05f67232a711516f8994881b1bc227bad8bc055))
* add mermaid diagrams to architecture, MCP, and orchestration docs ([5ff16f4](https://github.com/opus-domini/praetor/commit/5ff16f4e41a99f8444a22db08f72c74647a51b9a))
* analyze Paperclip data model, communication patterns, and skill protocol ([e144d4d](https://github.com/opus-domini/praetor/commit/e144d4d9152ff1c281fd09e6aa7a86b69add6da4))
* analyze Paperclip governance, budgets, approvals, and org hierarchy ([d3f666a](https://github.com/opus-domini/praetor/commit/d3f666acc9e413b689f91a58380250552fe87c01))
* analyze Paperclip heartbeat protocol, adapters, and execution lifecycle ([42af3f6](https://github.com/opus-domini/praetor/commit/42af3f6389d38d8b2cb78a69d98eb38119f83d6b))
* design 6-milestone absorption plan with 30 atomic tasks from Paperclip analysis ([796a116](https://github.com/opus-domini/praetor/commit/796a1165193cc97d1a0647912f382a32a368d627))
* distill 14 key learnings from Paperclip applicable to Praetor ([9d386ce](https://github.com/opus-domini/praetor/commit/9d386cea625a7f8382663f98734b64b3a9106fcc))

## [0.1.0](https://github.com/opus-domini/praetor/compare/v0.1.0...v0.1.0) (2026-03-09)


### Features

* add LM Studio provider ([8948c41](https://github.com/opus-domini/praetor/commit/8948c414099d8225751edf8edd40da8bfe7e0965))
* add make install target ([1e0a0d4](https://github.com/opus-domini/praetor/commit/1e0a0d4f4b96091424ba117dca07b64b3e557fc9))
* add MCP server and shared agent commands ([5f0f8ef](https://github.com/opus-domini/praetor/commit/5f0f8ef86a417cc3a2a4937e8b93bc5ea5cdeab9))
* add per-task agent/model overrides, tool constraints, and standards gate ([dc4e023](https://github.com/opus-domini/praetor/commit/dc4e023e881081e6530ce553f69968428816437d))
* add praetor init command and enrich CLI help text ([8f7b299](https://github.com/opus-domini/praetor/commit/8f7b299278c1a1653ebd7b22261b016bdc20fc08))
* center mermaid diagrams and add zoom/pan controls ([b2bd4a7](https://github.com/opus-domini/praetor/commit/b2bd4a7a40103835894a7de0f9476535bac0288c))
* strengthen local operations flow, evals, and planner UX ([b67b81a](https://github.com/opus-domini/praetor/commit/b67b81adfaea86eaf019bd078abafe52ecc067eb))


### Bug Fixes

* correct all orchestration diagrams against actual implementation ([5549f47](https://github.com/opus-domini/praetor/commit/5549f4746255acbe27ed34a62186f5804d2781d8))
* resolve pre-existing lint warnings and data races ([391ac0f](https://github.com/opus-domini/praetor/commit/391ac0f5cadf1d9b241113e53953b23a68d777a4))
* use &lt;br&gt; instead of \n for line breaks in mermaid diagrams ([978da7c](https://github.com/opus-domini/praetor/commit/978da7cac0fa1ed5f6dd077f1de94310f5d29fb3))


### Refactors

* redesign praetor init as project installer, not project creator ([ee9b0e6](https://github.com/opus-domini/praetor/commit/ee9b0e66f40db6a983a850e4a02cf48c29001fdf))
* remove plan schema versioning and backward compatibility ([0d37e8c](https://github.com/opus-domini/praetor/commit/0d37e8c131564ce3cc78acff732d44a2a9a5ec2f))
* remove praetor commands CLI, consolidate into init ([85e1d21](https://github.com/opus-domini/praetor/commit/85e1d2147b87050b5745b03c8f5253cab8220cee))
* standardize CLI output through Renderer and normalize UX ([5946405](https://github.com/opus-domini/praetor/commit/59464050f5c844acf38f886dba933be364bd98f4))


### Documentation

* add LICENSE and CONTRIBUTING guidelines ([2bc4223](https://github.com/opus-domini/praetor/commit/2bc4223f208357947e030f8ddf5a7ec665239927))
* add LM Studio to documentation site ([d05f672](https://github.com/opus-domini/praetor/commit/d05f67232a711516f8994881b1bc227bad8bc055))
* add mermaid diagrams to architecture, MCP, and orchestration docs ([5ff16f4](https://github.com/opus-domini/praetor/commit/5ff16f4e41a99f8444a22db08f72c74647a51b9a))
* analyze Paperclip data model, communication patterns, and skill protocol ([e144d4d](https://github.com/opus-domini/praetor/commit/e144d4d9152ff1c281fd09e6aa7a86b69add6da4))
* analyze Paperclip governance, budgets, approvals, and org hierarchy ([d3f666a](https://github.com/opus-domini/praetor/commit/d3f666acc9e413b689f91a58380250552fe87c01))
* analyze Paperclip heartbeat protocol, adapters, and execution lifecycle ([42af3f6](https://github.com/opus-domini/praetor/commit/42af3f6389d38d8b2cb78a69d98eb38119f83d6b))
* design 6-milestone absorption plan with 30 atomic tasks from Paperclip analysis ([796a116](https://github.com/opus-domini/praetor/commit/796a1165193cc97d1a0647912f382a32a368d627))
* distill 14 key learnings from Paperclip applicable to Praetor ([9d386ce](https://github.com/opus-domini/praetor/commit/9d386cea625a7f8382663f98734b64b3a9106fcc))

## [0.1.0](https://github.com/opus-domini/praetor/compare/v0.1.0...v0.1.0) (2026-03-09)


### Features

* add LM Studio provider ([8948c41](https://github.com/opus-domini/praetor/commit/8948c414099d8225751edf8edd40da8bfe7e0965))
* add make install target ([1e0a0d4](https://github.com/opus-domini/praetor/commit/1e0a0d4f4b96091424ba117dca07b64b3e557fc9))
* add MCP server and shared agent commands ([5f0f8ef](https://github.com/opus-domini/praetor/commit/5f0f8ef86a417cc3a2a4937e8b93bc5ea5cdeab9))
* add per-task agent/model overrides, tool constraints, and standards gate ([dc4e023](https://github.com/opus-domini/praetor/commit/dc4e023e881081e6530ce553f69968428816437d))
* add praetor init command and enrich CLI help text ([8f7b299](https://github.com/opus-domini/praetor/commit/8f7b299278c1a1653ebd7b22261b016bdc20fc08))
* center mermaid diagrams and add zoom/pan controls ([b2bd4a7](https://github.com/opus-domini/praetor/commit/b2bd4a7a40103835894a7de0f9476535bac0288c))
* strengthen local operations flow, evals, and planner UX ([b67b81a](https://github.com/opus-domini/praetor/commit/b67b81adfaea86eaf019bd078abafe52ecc067eb))


### Bug Fixes

* correct all orchestration diagrams against actual implementation ([5549f47](https://github.com/opus-domini/praetor/commit/5549f4746255acbe27ed34a62186f5804d2781d8))
* resolve pre-existing lint warnings and data races ([391ac0f](https://github.com/opus-domini/praetor/commit/391ac0f5cadf1d9b241113e53953b23a68d777a4))
* use &lt;br&gt; instead of \n for line breaks in mermaid diagrams ([978da7c](https://github.com/opus-domini/praetor/commit/978da7cac0fa1ed5f6dd077f1de94310f5d29fb3))


### Refactors

* redesign praetor init as project installer, not project creator ([ee9b0e6](https://github.com/opus-domini/praetor/commit/ee9b0e66f40db6a983a850e4a02cf48c29001fdf))
* remove plan schema versioning and backward compatibility ([0d37e8c](https://github.com/opus-domini/praetor/commit/0d37e8c131564ce3cc78acff732d44a2a9a5ec2f))
* remove praetor commands CLI, consolidate into init ([85e1d21](https://github.com/opus-domini/praetor/commit/85e1d2147b87050b5745b03c8f5253cab8220cee))
* standardize CLI output through Renderer and normalize UX ([5946405](https://github.com/opus-domini/praetor/commit/59464050f5c844acf38f886dba933be364bd98f4))


### Documentation

* add LICENSE and CONTRIBUTING guidelines ([2bc4223](https://github.com/opus-domini/praetor/commit/2bc4223f208357947e030f8ddf5a7ec665239927))
* add LM Studio to documentation site ([d05f672](https://github.com/opus-domini/praetor/commit/d05f67232a711516f8994881b1bc227bad8bc055))
* add mermaid diagrams to architecture, MCP, and orchestration docs ([5ff16f4](https://github.com/opus-domini/praetor/commit/5ff16f4e41a99f8444a22db08f72c74647a51b9a))
* analyze Paperclip data model, communication patterns, and skill protocol ([e144d4d](https://github.com/opus-domini/praetor/commit/e144d4d9152ff1c281fd09e6aa7a86b69add6da4))
* analyze Paperclip governance, budgets, approvals, and org hierarchy ([d3f666a](https://github.com/opus-domini/praetor/commit/d3f666acc9e413b689f91a58380250552fe87c01))
* analyze Paperclip heartbeat protocol, adapters, and execution lifecycle ([42af3f6](https://github.com/opus-domini/praetor/commit/42af3f6389d38d8b2cb78a69d98eb38119f83d6b))
* design 6-milestone absorption plan with 30 atomic tasks from Paperclip analysis ([796a116](https://github.com/opus-domini/praetor/commit/796a1165193cc97d1a0647912f382a32a368d627))
* distill 14 key learnings from Paperclip applicable to Praetor ([9d386ce](https://github.com/opus-domini/praetor/commit/9d386cea625a7f8382663f98734b64b3a9106fcc))

## [0.1.0](https://github.com/opus-domini/praetor/compare/v0.1.0...v0.1.0) (2026-03-09)


### Features

* add LM Studio provider ([8948c41](https://github.com/opus-domini/praetor/commit/8948c414099d8225751edf8edd40da8bfe7e0965))
* add make install target ([1e0a0d4](https://github.com/opus-domini/praetor/commit/1e0a0d4f4b96091424ba117dca07b64b3e557fc9))
* add MCP server and shared agent commands ([5f0f8ef](https://github.com/opus-domini/praetor/commit/5f0f8ef86a417cc3a2a4937e8b93bc5ea5cdeab9))
* add per-task agent/model overrides, tool constraints, and standards gate ([dc4e023](https://github.com/opus-domini/praetor/commit/dc4e023e881081e6530ce553f69968428816437d))
* add praetor init command and enrich CLI help text ([8f7b299](https://github.com/opus-domini/praetor/commit/8f7b299278c1a1653ebd7b22261b016bdc20fc08))
* center mermaid diagrams and add zoom/pan controls ([b2bd4a7](https://github.com/opus-domini/praetor/commit/b2bd4a7a40103835894a7de0f9476535bac0288c))
* strengthen local operations flow, evals, and planner UX ([b67b81a](https://github.com/opus-domini/praetor/commit/b67b81adfaea86eaf019bd078abafe52ecc067eb))


### Bug Fixes

* correct all orchestration diagrams against actual implementation ([5549f47](https://github.com/opus-domini/praetor/commit/5549f4746255acbe27ed34a62186f5804d2781d8))
* resolve pre-existing lint warnings and data races ([391ac0f](https://github.com/opus-domini/praetor/commit/391ac0f5cadf1d9b241113e53953b23a68d777a4))
* use &lt;br&gt; instead of \n for line breaks in mermaid diagrams ([978da7c](https://github.com/opus-domini/praetor/commit/978da7cac0fa1ed5f6dd077f1de94310f5d29fb3))


### Refactors

* redesign praetor init as project installer, not project creator ([ee9b0e6](https://github.com/opus-domini/praetor/commit/ee9b0e66f40db6a983a850e4a02cf48c29001fdf))
* remove plan schema versioning and backward compatibility ([0d37e8c](https://github.com/opus-domini/praetor/commit/0d37e8c131564ce3cc78acff732d44a2a9a5ec2f))
* remove praetor commands CLI, consolidate into init ([85e1d21](https://github.com/opus-domini/praetor/commit/85e1d2147b87050b5745b03c8f5253cab8220cee))
* standardize CLI output through Renderer and normalize UX ([5946405](https://github.com/opus-domini/praetor/commit/59464050f5c844acf38f886dba933be364bd98f4))


### Documentation

* add LICENSE and CONTRIBUTING guidelines ([2bc4223](https://github.com/opus-domini/praetor/commit/2bc4223f208357947e030f8ddf5a7ec665239927))
* add LM Studio to documentation site ([d05f672](https://github.com/opus-domini/praetor/commit/d05f67232a711516f8994881b1bc227bad8bc055))
* add mermaid diagrams to architecture, MCP, and orchestration docs ([5ff16f4](https://github.com/opus-domini/praetor/commit/5ff16f4e41a99f8444a22db08f72c74647a51b9a))
* analyze Paperclip data model, communication patterns, and skill protocol ([e144d4d](https://github.com/opus-domini/praetor/commit/e144d4d9152ff1c281fd09e6aa7a86b69add6da4))
* analyze Paperclip governance, budgets, approvals, and org hierarchy ([d3f666a](https://github.com/opus-domini/praetor/commit/d3f666acc9e413b689f91a58380250552fe87c01))
* analyze Paperclip heartbeat protocol, adapters, and execution lifecycle ([42af3f6](https://github.com/opus-domini/praetor/commit/42af3f6389d38d8b2cb78a69d98eb38119f83d6b))
* design 6-milestone absorption plan with 30 atomic tasks from Paperclip analysis ([796a116](https://github.com/opus-domini/praetor/commit/796a1165193cc97d1a0647912f382a32a368d627))
* distill 14 key learnings from Paperclip applicable to Praetor ([9d386ce](https://github.com/opus-domini/praetor/commit/9d386cea625a7f8382663f98734b64b3a9106fcc))

## [0.1.0](https://github.com/opus-domini/praetor/compare/v0.1.0...v0.1.0) (2026-03-03)


### Features

* add LM Studio provider ([8948c41](https://github.com/opus-domini/praetor/commit/8948c414099d8225751edf8edd40da8bfe7e0965))
* add MCP server and shared agent commands ([5f0f8ef](https://github.com/opus-domini/praetor/commit/5f0f8ef86a417cc3a2a4937e8b93bc5ea5cdeab9))
* add per-task agent/model overrides, tool constraints, and standards gate ([dc4e023](https://github.com/opus-domini/praetor/commit/dc4e023e881081e6530ce553f69968428816437d))


### Bug Fixes

* resolve pre-existing lint warnings and data races ([391ac0f](https://github.com/opus-domini/praetor/commit/391ac0f5cadf1d9b241113e53953b23a68d777a4))


### Refactors

* remove plan schema versioning and backward compatibility ([0d37e8c](https://github.com/opus-domini/praetor/commit/0d37e8c131564ce3cc78acff732d44a2a9a5ec2f))


### Documentation

* add LICENSE and CONTRIBUTING guidelines ([2bc4223](https://github.com/opus-domini/praetor/commit/2bc4223f208357947e030f8ddf5a7ec665239927))
* add LM Studio to documentation site ([d05f672](https://github.com/opus-domini/praetor/commit/d05f67232a711516f8994881b1bc227bad8bc055))

## 0.1.0 (2026-02-28)


### Features

* initial project implementation ([fd920a8](https://github.com/opus-domini/praetor/commit/fd920a85c026dd571e65318f6629c3b5496c7125))


### Documentation

* update README to reflect current project state ([ea088f7](https://github.com/opus-domini/praetor/commit/ea088f757176d6718f0fd1c07f21e5b972ae8746))
