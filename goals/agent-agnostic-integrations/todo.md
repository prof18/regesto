# Agent-agnostic integrations — execution ledger

Durable checklist for milestones 0–12. Each milestone closes only after its
focused checks, the full gate, both fresh-context review passes, and its
checkpoint commit(s) are recorded below.

Full gate (required after each pass): `gofmt -l .`; `go vet ./...`; `go test ./...`;
`go test -race ./...`; `git diff --check`.

## Milestone 0 — safe start and baseline

- [x] Record baseline and commit reviewed goal package before M1.
- [x] Scope: goal package, branch setup, baseline evidence, and this ledger.
- [x] Owners: implementation engine untouched; goal-package files owned by this
  milestone; preserve any unrelated user changes.
- [x] High-risk surfaces: branch/base identity, untracked-state isolation,
  repository-wide formatting, vet/tests, and accidental changes outside the goal.
- [x] Baseline: branch `agent-agnostic-integrations`; base
  `d43400a3dff5147220aeb57da8349c85067accbe`; pre-branch state was only the
  untracked `goals/agent-agnostic-integrations/` package.
- [x] Pre-pass-1 state: staged files none; unstaged tracked files none; untracked
  files are `goal.md`, `facts.md`, `facts.meta.json`, `plan.md`, `interview.json`,
  `interview-result.json`, and `todo.md`, all under this goal package.
- [x] Verify: `git status --short`; `gofmt -l .`; `go vet ./...`; `go test ./...`;
  then confirm clean status before M1.
- [x] Skip list: no code or package findings. The raw race command's default
  module-cache write was rejected by the workspace sandbox; the same command
  passed with `GOCACHE` and `GOPATH` redirected to `/private/tmp`.
- [x] Fresh-context reviewer shards: (A) baseline/branch and package scope;
  (B) goal/facts/plan consistency; (C) repository hygiene and gate commands.
- Review pass 1: clean. Specification shard found no consistency, count, scope,
  or acceptance defects. Hygiene shard's race-gate failure was reproduced and
  classified as sandbox-only after the isolated-cache rerun passed. Main audit
  confirmed 30 aligned fact IDs, 13 milestones, valid JSON, and isolated status.
  Full gate passed. Checkpoint `6e78303` — `docs: add agent-agnostic integrations goal`.
- Pass 2 baseline: checkpoint `6e78303`; review the full milestone diff from
  `d43400a` with the sandbox-cache item carried as a rejected environment-only
  finding.
- Review pass 2: accepted and fixed two specification gaps: Hermes/channel trust
  now has a quarantine-safe resolution order, and shared instruction targets
  now have deterministic grouping, conflict, missing-path, symlink authority,
  backup, and swap-refusal rules with required fixtures. Targeted re-reviews
  closed the missing-target and outside-HOME symlink edge cases. Hygiene shard
  and final targeted review are clean; full gate passed with isolated Go caches.
  Remediation `91d5dcd` — `docs: clarify integration trust and installer targets`.
- [x] Milestone 0 complete at reviewed checkpoint `91d5dcd`; final ledger-only
  closure follows with no implementation changes.

## Milestone 1 — compatibility fixtures

- [x] Add legacy config/integration fixtures and compatibility tests, including generic profile target cases.
- Baseline: branch `agent-agnostic-integrations`; base checkpoint `d2941b9`;
  staged files none; unstaged tracked files none; untracked files are
  `tests/integrations_test.go` and four files under `tests/fixtures/config/`.
- Scope/owners: test-only compatibility boundary in `tests/fixtures/config` and
  `tests/integrations_test.go`; production behavior and existing user files are
  untouched.
- High-risk surfaces: byte-compatible legacy config output, exact adapter and
  instance-file defaults, HOME-dependent detection, current-vs-target trust
  expectations, and future contracts that must remain explicit skips.
- Verify: `go test ./tests -run 'Adapter|Integration|Upgrade|Trust'` plus full gate.
- Skip list/findings carried: raw Go race runs need isolated `GOCACHE`/`GOPATH`
  under the workspace sandbox; no milestone-0 code findings carry forward.
- Review shards: configuration/defaults/detection; trust and future-contract
  boundary; instance/upgrade fixture and test determinism.
- Review pass 1: accepted and fixed exact CLI override serialization, none/one/all
  and second-HOME detection, dead generic-fixture coverage, and duplicate instance
  inventory. Unknown/custom quarantine is an explicit skipped M3 contract; the
  v0.3.1 fixture pins parser/config compatibility while zero-edit upgrade remains
  M11. Focused output records the M2/M3 skips. Full gate passed with isolated Go
  caches. Checkpoint `1df6540` — `test: pin legacy integration compatibility`.
- Pass 2 baseline: checkpoint `1df6540`; review the full milestone diff from
  `d2941b9`. Carry the isolated-cache environment note and the explicit M2/M3/M11
  deferrals; no production trust or upgrade behavior belongs in this milestone.
- Review pass 2: both fresh shards clean. They confirmed exact defaults and CLI
  output, override precedence, detection isolation, trust compatibility, exact
  instance inventory, fixture use, focused-regex participation, and accurate
  M2/M3/M11 deferrals. Full gate passed; no remediation commit required.
- [x] Milestone 1 complete at reviewed checkpoint `1df6540`; final ledger-only
  closure follows with no test or production changes.

## Milestone 2 — profiles and additive configuration

- [x] Add data-driven profiles, loader/validation, integrations config, and legacy compatibility mapping.
- Baseline: branch `agent-agnostic-integrations`; base checkpoint `2337d5c`;
  staged files none; implementation changes are limited to four new profile JSON
  files, `internal/adapters/profile.go`, the planned adapter/config/CLI callers,
  and focused adapter/integration/upgrade tests.
- Scope/owners: profile JSON; `internal/adapters`, `internal/config`, config/init/showconfig/upgrade/scaffold callers and tests. Trust normalization, installation application, hook execution, rendering, and typed harvesting remain later milestones.
- High-risk surfaces: zero-edit legacy configs (including absent lists and unknown
  overridden agents), data-driven detection, embedded-vs-instance profile
  precedence, strict profile/config validation, hook-settings consistency,
  byte-compatible legacy text output, new JSON capability output, and current
  shell-installer compatibility.
- Verify: `go test ./tests -run 'Adapter|Integration|Config|InstanceFiles'`; `go test ./...` plus full gate.
- Skip list/findings carried: raw Go race runs use isolated `GOCACHE`/`GOPATH`;
  unknown/custom trust enforcement remains the explicit M3 skip and v0.3.1
  end-to-end upgrade remains M11.
- Review shards: profile/config resolution and validation; legacy/CLI/installer
  compatibility; instance/scaffold metadata and test coverage.
- Review pass 1: accepted and fixed capability-combination validation for hooks,
  singular settings and memory override ambiguity, inactive unsafe skill variants,
  command detection validation/consumption, and silent harvest resolution errors.
  Main audit additionally preserved duplicate legacy agents, removed product-ID
  hook requirements, and brought all material M2 tests into the focused regex.
  Compatibility review and every targeted re-review are clean. Focused tests and
  the full formatting/vet/test/race/diff gate passed with isolated Go caches.
  Checkpoint `4c6f6b0` — `feat: add capability-driven integration profiles`.
- Pass 2 baseline: checkpoint `4c6f6b0`; review the full milestone diff from
  `2337d5c`. Carry only the isolated-cache environment note and the explicit
  M3 trust/M11 upgrade deferrals; all pass-1 findings and targeted re-reviews
  are closed.
- Review pass 2: accepted and fixed contradictory `none` hook/memory
  declarations, an incomplete init migration example, HOME/command-name edge
  validation, and missing strict-parser/fallback/duplicate/detection contracts.
  Duplicate instance profile IDs are structurally rejected during file/ID
  validation; the v0.3.1 end-to-end upgrade remains intentionally M11 while
  M2's exact legacy resolution/output fixtures stay green. All three fresh
  reviewers and final targeted re-reviews are clean. Focused tests and the full
  formatting/vet/test/race/diff gate passed with isolated Go caches.
  Remediation `a918b84` — `fix: harden integration profile contracts`.
- [x] Milestone 2 complete at reviewed checkpoint `a918b84`; final ledger-only
  closure follows with no implementation changes.

## Milestone 3 — generalized source trust

- [ ] Replace product-name trust conditionals with policy-driven quarantine/trust while preserving trusted sources and schema.
- Scope/owners: `internal/normalize`, `internal/config`, normalize tests, `SCHEMA.md`.
- Verify: `go test ./tests -run 'Normalize|Trust|Human'` plus full gate.
- Skip list/findings carried: ___
- Review shards: trust resolver; normalization/index visibility; schema/compatibility tests.
- Review pass 1: findings ___; gates ___; commit hash/purpose ___
- Review pass 2: findings ___; gates ___; remediation commit hash/purpose ___

## Milestone 4 — stable JSON operations and validated writes

- [ ] Extract shared validation/stamping/atomic writes; add stable JSON command forms without changing text output.
- Scope/owners: `internal/write`, CLI write/search/context/project/config, normalize reuse, command tests.
- Verify: focused Write/Search/Context/Project/Config tests and `go test -race ./...` plus full gate.
- Skip list/findings carried: ___
- Review shards: write validation/atomicity; CLI JSON compatibility; race/error tests.
- Review pass 1: findings ___; gates ___; commit hash/purpose ___
- Review pass 2: findings ___; gates ___; remediation commit hash/purpose ___

## Milestone 5 — Go installation planning/application

- [ ] Implement plan/apply/instructions/skills/hooks installers; reduce shim; preserve ownership, backups, dry-run, idempotence.
- Scope/owners: `internal/install`, install/upgrade CLI, `bin/regesto-install`, install tests.
- Verify: `go test ./tests -run 'Install|Upgrade|Manifest'`; `go test -race ./...`; temporary-HOME dry-run/second-install checks; full gate.
- Skip list/findings carried: ___
- Review shards: mutation/backup ownership; manifest/upgrade; dry-run/idempotence tests.
- Review pass 1: findings ___; gates ___; commit hash/purpose ___
- Review pass 2: findings ___; gates ___; remediation commit hash/purpose ___

## Milestone 6 — protocol-owned hooks

- [ ] Port Claude/Hermes hook parsing, framing, registration/manual recipes, bounded first-turn behavior, and fail-open handling to Go.
- Scope/owners: `internal/hooks`, hook CLI/shim, adapter hook compatibility, hook fixtures/tests.
- Verify: Hook/Claude/Hermes tests; `bash -n bin/* adapters/*/hooks/*.sh`; three documented manual probes; full gate.
- Skip list/findings carried: ___
- Review shards: protocol framing; registration/YAML safety; failure/session tests.
- Review pass 1: findings ___; gates ___; commit hash/purpose ___
- Review pass 2: findings ___; gates ___; remediation commit hash/purpose ___

## Milestone 7 — portable skills/instructions

- [ ] Make shared artifacts contract-compliant and render only declared host overlays/optimizations.
- Scope/owners: shared skills/instructions, variants, installer renderer, skill tests.
- Verify: Skill/Instruction/Render tests; forbidden-syntax `rg`; full gate.
- Skip list/findings carried: ___
- Review shards: Agent Skills contract; leakage/render matrix; installer tests/docs.
- Review pass 1: findings ___; gates ___; commit hash/purpose ___
- Review pass 2: findings ___; gates ___; remediation commit hash/purpose ___

## Milestone 8 — optional Markdown memory harvesting

- [ ] Add typed declared memory sources, Markdown glob harvesting, explicit unsupported/none reporting, and legacy snapshots.
- Scope/owners: profile model, `internal/harvest`, harvest CLI, harvest tests.
- Verify: Harvest/Memory/Integration tests plus full gate.
- Skip list/findings carried: ___
- Review shards: source typing/safety; diff/snapshot compatibility; generic integration tests.
- Review pass 1: findings ___; gates ___; commit hash/purpose ___
- Review pass 2: findings ___; gates ___; remediation commit hash/purpose ___

## Milestone 9 — local stdio MCP

- [ ] Implement stdlib JSON-RPC/MCP handshake, resources, tools, clean shutdown, and shared validation with no socket.
- Scope/owners: `internal/mcp`, MCP CLI, MCP protocol tests/transcript.
- Verify: MCP/JSONRPC tests; initialize→tools/list→tools/call→resources/read stdin transcript; race/full gate.
- Skip list/findings carried (including dependency decision): ___
- Review shards: protocol conformance; stdout/error framing and no-network posture; tool validation/tests.
- Review pass 1: findings ___; gates ___; commit hash/purpose ___
- Review pass 2: findings ___; gates ___; remediation commit hash/purpose ___

## Milestone 10 — diagnostics and documentation

- [ ] Add `doctor`, canonical capability matrix, product recipes, Hermes documentation, and profile-checked claims.
- Scope/owners: doctor CLI; README/DESIGN/CONTRIBUTING/SCHEMA; docs and init template.
- Verify: Doctor/Docs/Profile tests; stale-claim `rg`; full gate.
- Skip list/findings carried: ___
- Review shards: diagnostic accuracy; capability matrix/recipe consistency; documentation terminology.
- Review pass 1: findings ___; gates ___; commit hash/purpose ___
- Review pass 2: findings ___; gates ___; remediation commit hash/purpose ___

## Milestone 11 — upgrade proof and packaging

- [ ] Extend CI/fixtures for v0.3.1 zero-edit upgrade, dry-run/install/doctor/MCP, and package result.
- Scope/owners: upgrade/engine tests, CI workflow, changelog.
- Verify: full gate and standalone temporary-HOME proof from plan; inspect failures before cleanup.
- Skip list/findings carried: ___
- Review shards: migration/ownership; CI and packaging; standalone proof/release docs.
- Review pass 1: findings ___; gates ___; commit hash/purpose ___
- Review pass 2: findings ___; gates ___; remediation commit hash/purpose ___

## Milestone 12 — live regression matrix

- [ ] After automated gates, run and record Claude Code, Codex, Hermes, and generic fixture live checks; back up installer targets and dry-run first.
- Scope/owners: host artifacts and temporary fixture only; preserve backups and unrelated local files.
- Verify: skills/instructions, hooks, trust, memory baseline/change, generic no-hook behavior; record commands and observed versions.
- Skip list/findings carried: ___
- Review shards: Claude live path; Codex/generic portability; Hermes hook/trust; evidence/release checklist.
- Review pass 1: findings ___; gates ___; commit hash/purpose ___
- Review pass 2: findings ___; gates ___; remediation commit hash/purpose ___

## Final completion record

- [ ] All accepted facts and automated checks accounted for; F28 live tests recorded; out-of-scope F29 preserved.
- [ ] Every milestone has two recorded review passes, gates, findings disposition, and cohesive checkpoint commit(s).
- [ ] Working tree contains only reviewed goal changes; no remote service or proprietary cloud-memory connector introduced.
