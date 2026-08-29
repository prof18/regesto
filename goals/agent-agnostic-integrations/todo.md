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

- [x] Replace product-name trust conditionals with policy-driven quarantine/trust while preserving trusted sources and schema.
- Baseline: branch `agent-agnostic-integrations`; base checkpoint `4b50ed3`;
  staged, unstaged, and untracked files none.
- Scope/owners: `internal/normalize`, `internal/config`, normalize tests,
  `SCHEMA.md`, plus current DESIGN/setup/init trust claims found stale in review.
- High-risk surfaces: default-deny behavior for unknown/unconfigured sources,
  exact-source override precedence, legacy Claude/Codex/Hermes compatibility,
  distinct integration IDs for multiple surfaces, quarantine invisibility, and
  removing every product-name conditional without broadening trust.
- Verify: `go test ./tests -run 'Normalize|Trust|Human'` plus full gate.
- Skip list/findings carried: raw Go race runs use isolated `GOCACHE`/`GOPATH`;
  installation and hook registration remain M5/M6, while profile trust metadata
  and exact legacy output are closed M2 contracts.
- Review shards: trust resolver; normalization/index visibility; schema/compatibility tests.
- Review pass 1: accepted and fixed the initially omitted `[source_policies]`
  exact/trailing-wildcard layer, strict config-load validation and duplicate
  rejection, canonical agent/machine/source binding, deterministic precedence,
  source-policy end-to-end coverage, and stale current trust claims. Exact
  source policy beats legacy exact trust, which beats the longest pattern,
  configured default, and final quarantine. All three reviewers and targeted
  re-reviews are clean. Focused tests and the full formatting/vet/test/race/diff
  gate passed with isolated Go caches. Checkpoint `0495f68` —
  `feat: generalize source trust policy`.
- Pass 2 baseline: checkpoint `0495f68`; review the full milestone diff from
  `4b50ed3`. Carry only the isolated-cache environment note and later-milestone
  installer/hook boundaries; all pass-1 findings are closed.
- Review pass 2: accepted and fixed a quarantine bypass in `normalize
  --show-prompt`, restored the documented `human@<machine>` raw-note workflow
  without weakening human authority, reserved `human` against configured or
  harvested integration impersonation, corrected trust-precedence comments and
  schema text, replaced the stale fourth-agent adapter claim, and expanded focused
  init/policy/CLI/end-to-end regressions. All three reviewers and targeted
  re-reviews are clean. Focused tests and the full formatting/vet/test/race/diff
  gate passed with isolated Go caches. Remediation `8d03c7b` —
  `fix: close source trust policy gaps`.
- [x] Milestone 3 complete at reviewed checkpoint `8d03c7b`; final ledger-only
  closure follows with no implementation changes.

## Milestone 4 — stable JSON operations and validated writes

- [x] Extract shared validation/stamping/atomic writes; add stable JSON command forms without changing text output.
- Baseline: branch `agent-agnostic-integrations`; base checkpoint `cb4b72b`;
  staged, unstaged, and untracked files none.
- Scope/owners: new `internal/write`; write/search/context/project/config CLI
  operations and focused tests; normalization candidate-write reuse; current
  write skill plus narrow README/SCHEMA claims. Installation, hooks, artifact
  rendering, harvesting, MCP, and doctor remain later milestones.
- High-risk surfaces: authoritative source/schema/timestamp stamping, duplicate
  frontmatter keys and IDs, atomic no-replace publication under races, project
  path containment, agent-versus-human reconciliation preview, JSON stdout
  purity/schema stability, and byte-compatible default text output.
- Verify: `go test ./tests -run 'Write|Search|Context|Project|Config'`; command
  package JSON/text tests; `go test -race ./...`; full gate.
- Skip list/findings carried: raw Go race runs use isolated `GOCACHE`/`GOPATH`;
  pass-zero policy review found duplicate candidate frontmatter can currently
  bypass normalization's first-key stamping, so shared writer migration and a
  regression are required before M4 closes.
- Review shards: shared validation/atomicity and normalize reuse; stable JSON/text
  compatibility; write workflow/docs and race/error coverage.
- Review pass 1: accepted and fixed duplicate-frontmatter authority bypasses,
  noncanonical explicit sources, dropped `review_after`, duplicate JSON object
  keys, nullable empty config IDs, and a machine-scoped global-ID race. The
  shared writer now owns strict semantic validation, authoritative stamping,
  project-safe placement, KB-wide ID serialization, atomic no-replace
  publication, and human-contest reconciliation preview; normalize and promote
  reuse it. Stable versioned JSON covers search/context/project/config/write,
  default text stays compatible, and the write shim/workflow are packaged.
  All three reviewers and targeted re-reviews are clean. Focused tests and the
  full formatting/vet/test/race/bash/diff gate passed with isolated Go caches.
  Checkpoint `e8ab6f5` — `feat: add validated JSON operations`.
- Pass 2 baseline: checkpoint `e8ab6f5`; review the full milestone diff from
  `cb4b72b`. Carry only the isolated-cache environment note and later-milestone
  install/hook/render/harvest/MCP/doctor boundaries; all pass-1 findings are closed.
- Review pass 2: accepted and fixed outside-KB symlink publication, unrelated
  `supersedes` retirement, shared lock-directory cleanup races, and stale PATH
  engine selection by the new checkout shim. Publication now uses Go's rooted
  descriptor-relative filesystem operations, eliminating the residual
  check/open race while retaining atomic no-replace links. KB-wide lock state is
  documented, mismatched claim identities fail before publication, and a
  checkout runs its matching source before release binaries. All fresh reviewers
  and targeted re-reviews are clean. Focused tests and the full
  formatting/vet/test/race/bash/diff gate passed with isolated Go caches.
  Remediation `012ab66` — `fix: harden validated write boundaries`.
- [x] Milestone 4 complete at reviewed checkpoint `012ab66`; final ledger-only
  closure follows with no implementation changes.

## Milestone 5 — Go installation planning/application

- [x] Implement plan/apply/instructions/skills/hooks installers; reduce shim; preserve ownership, backups, dry-run, idempotence.
- Baseline: branch `agent-agnostic-integrations`; base checkpoint `1f1b687`;
  staged, unstaged, and untracked files none.
- Scope/owners: new `internal/install`; install/upgrade CLI;
  `bin/regesto-install`; focused installer tests. Engine build and PATH linking
  remain compatibility-shim responsibilities because an executing `regesto
  install` already has an engine; hook payload translation and uncertain Hermes
  YAML mutation remain M6, and portable host overlays remain M7.
- High-risk surfaces: missing-target canonicalization through symlinked
  ancestors, target swaps between plan/apply, shared instruction grouping and
  conflicts, marker replacement without foreign-content loss, adjacent backups,
  foreign skill ownership, rendered-stage pruning, JSON stability, complete
  inert dry-runs, and zero-change second installs.
- Verify: `go test ./tests -run 'Install|Upgrade|Manifest'`; `go test -race ./...`; temporary-HOME dry-run/second-install checks; full gate.
- Skip list/findings carried: raw Go race runs use isolated
  `GOCACHE`/`GOPATH`; protocol-owned hook payloads and Hermes YAML registration
  are M6, portable artifact variants are M7, and v0.3.1 end-to-end packaging is
  M11.
- Review shards: mutation/canonicalization/backup ownership; rendered-skill and
  hook ownership plus shim portability; CLI/upgrade, dry-run, and idempotence.
- Review pass 1: accepted and fixed target-swap races with rooted filesystem
  operations and immediate rechecks; cross-kind canonical-target conflicts;
  unproved rendered-stage pruning/adoption (including empty directories);
  changed generated content and ownership markers between plan/apply; first-pass
  stale skill/link cleanup; executable hook validation and legacy hook routing;
  checkout PATH-link disclosure and no-Go fallback. The Go planner now describes
  every mutation, groups shared instruction/settings targets deterministically,
  preserves foreign paths, creates collision-safe adjacent backups, and applies
  idempotently. All three reviewers and targeted re-reviews are clean. Focused
  tests and the full formatting/vet/test/race/bash/diff gate passed with isolated
  Go caches. Checkpoint `32d0cde` — `feat: add safe Go installation plans`.
- Pass 2 baseline: checkpoint `32d0cde`; review the full milestone diff from
  `1f1b687`. Carry only the isolated-cache environment note and the explicit M6
  hook-payload/Hermes-YAML, M7 portable-overlay, and M11 packaging boundaries;
  all pass-1 findings are closed.
- Review pass 2: accepted and fixed upgrade mutation escape through symlinked
  instance paths, collision-prone/umask-truncated backups, final-manifest symlink
  publication, insufficient ownership proof for cross-skill stage links, and
  filename-dependent marker ordering. Upgrade instance and manifest writes are
  now rooted and atomically published, backups are exclusive and preserve exact
  modes, link replacement requires same-skill proof, and markers precede every
  payload. A raised retry-sequencing concern was closed by direct verification:
  new-engine bytes classify `Current` before old hashes are consulted and prior
  removals classify `Missing`. All fresh reviewers and targeted re-reviews are
  clean. Focused tests and the full formatting/vet/test/race/bash/diff gate
  passed with isolated Go caches. Remediation `c24314d` — `fix: harden installer
  ownership boundaries`.
- [x] Milestone 5 complete at reviewed checkpoint `c24314d`; final ledger-only
  closure follows with no implementation changes.

## Milestone 6 — protocol-owned hooks

- [ ] Port Claude/Hermes hook parsing, framing, registration/manual recipes, bounded first-turn behavior, and fail-open handling to Go.
- Baseline: branch `agent-agnostic-integrations`; base checkpoint `adeecb1`;
  staged, unstaged, and untracked files none.
- Scope/owners: `internal/hooks`, hook CLI/shim, adapter hook compatibility, hook fixtures/tests.
- High-risk surfaces: exact plain-text versus JSON framing, malformed/oversized
  payload fail-open behavior, stdout purity, project-directory extraction, atomic
  once-per-session claims under repeat calls, bounded anonymous marker state,
  symlink-safe runtime state, preservation of existing Claude settings and Hermes
  YAML/allowlist content, and legacy wrapper migration without double injection.
- Verify: Hook/Claude/Hermes tests; `bash -n bin/regesto-* adapters/*/hooks/*.sh`;
  three documented manual probes; full gate.
- Skip list/findings carried: raw Go race runs use isolated `GOCACHE`/`GOPATH`;
  shared artifact portability remains M7, native memory harvesting M8, `doctor`
  validation and full Hermes product documentation M10, and live host/release
  packaging evidence M11/M12. Existing unfamiliar Hermes YAML must never be
  rewritten; the registrar falls back to an exact manual recipe while JSON
  allowlist ownership remains independently reviewable.
- Review shards: protocol framing; registration/YAML safety; failure/session tests.
- Review pass 1: accepted and fixed Claude's unsafe process-cwd fallback for
  missing/unusable payload directories; Hermes command-path shell injection
  through unquoted metacharacters; first-turn marker consumption before a
  successful context build; and stale/incomplete Hermes registration and probe
  documentation. The protocol boundary now fails open without cross-project
  context, every Hermes command is single-quoted for `shlex` parsing, context is
  built before the atomic session claim, and setup guidance uses explicit
  instance configuration/shims. All three reviewers and targeted re-reviews are
  clean. Focused tests, three manual Claude/Hermes probes, and the full
  formatting/vet/test/race/bash/diff gate passed with isolated Go caches.
  Checkpoint `89dede0` — `feat: add protocol-owned hooks`.
- Pass 2 baseline: checkpoint `89dede0`; review the full milestone diff from
  `adeecb1`. Carry only the isolated-cache environment note and the explicit M7
  artifact-portability, M8 harvesting, M10 doctor/documentation, and M11/M12
  packaging/live-host boundaries; all pass-1 findings are closed.
- Review pass 2: accepted and fixed direct-shim `REGESTO_KB_ROOT` routing,
  out-of-order Hermes false-before-true marker consumption, an invalid shell
  verification glob that included the compiled binary, and missing shell quotes
  in the manual Hermes YAML example. Both entry points now select the same
  instance, explicit false is state-free, the written gate targets only shims
  and wrappers, and the documented YAML matches the safe generated command.
  All fresh reviewers and targeted re-reviews are clean. Focused tests, the
  three manual probes, and the full formatting/vet/test/race/bash/diff gate
  passed with isolated Go caches. Remediation `97a801a` — `fix: close hook
  protocol edge cases`.
- [x] Milestone 6 complete at reviewed checkpoint `97a801a`; final ledger-only
  closure follows with no implementation changes.

## Milestone 7 — portable skills/instructions

- [x] Make shared artifacts contract-compliant and render only declared host overlays/optimizations.
- Baseline: branch `agent-agnostic-integrations`; base checkpoint `58d82a3`;
  staged, unstaged, and untracked files none.
- Scope/owners: shared skills/instructions, declarative variants, per-integration
  rendered trees, installer renderer/application support, and skill tests.
- High-risk surfaces: portable Agent Skills frontmatter and body syntax,
  integration-specific overlay isolation and protocol requirements, traversal or
  unknown variants, shared target conflicts across different renderings,
  Regesto-owned legacy-stage migration, symlink/marker ownership proofs, stale
  generated-file pruning, and byte-identical repeated installs.
- Verify: Skill/Instruction/Render tests; forbidden-syntax `rg`; full gate.
- Skip list/findings carried: raw Go race runs use isolated
  `GOCACHE`/`GOPATH`; native-memory harvesting remains M8, stdio MCP M9,
  capability diagnostics/full product documentation M10, legacy packaging M11,
  and live-host evidence M12. Existing host-specific behavior may remain only
  in an explicitly selected variant and must never leak into portable renders.
- Review shards: Agent Skills contract; leakage/render matrix; installer tests/docs.
- Review pass 1: accepted and fixed non-portable/insufficiently validated
  frontmatter; generic errors for stale pre-M7 instance skills; vendor syntax
  and stale product/custom-host documentation; duplicate legacy render plans;
  and adoption through an unowned skill-directory symlink. Common skills now
  use a strict portable metadata/YAML subset, host syntax is isolated in a
  protocol-checked append variant, stale instances receive an explicit upgrade
  remediation, duplicate legacy ids coalesce, and ownership refuses directory
  aliases. Per-integration renders, shared-target conflicts, custom profiles,
  legacy-stage migration, and repeat installs have focused coverage. All three
  reviewers and targeted re-reviews are clean. The forbidden-syntax scan and
  full formatting/vet/test/race/bash/diff gate passed with isolated Go caches.
  Checkpoint `3d157f4` — `feat: render portable integration skills`.
- Pass 2 baseline: checkpoint `3d157f4`; review the full milestone diff from
  `58d82a3`. Carry only isolated-cache requirements and the explicit M8 memory,
  M9 MCP, M10 diagnostic/documentation, M11 packaging, and M12 live-host
  boundaries; all pass-1 findings are closed.
- Review pass 2: accepted and fixed ordinary YAML block-scalar support and the
  missing Agent Skills `compatibility` length bound; undocumented exclusion of
  host-specific `allowed-tools`; stale README, contributor, and Codex setup
  wording; rendered payload paths that could alias through symlinks; and a
  plan/apply root-swap race caused by reopening mutable absolute ancestors.
  Shared skills now enforce and document the conservative portable subset,
  generated payloads reject intermediate and final symlinks, and installer
  mutations traverse canonical paths component-by-component through retained
  `os.Root` descriptors with inode verification. The Unicode-name suggestion
  was not retained because the published name syntax explicitly uses `a-z`.
  All three targeted re-reviews are clean. Skill/render tests, formatting, vet,
  full tests, race tests, shell syntax, the zero-match forbidden-syntax scan,
  and diff checks passed with isolated Go caches. Remediation `e39537c` —
  `fix: close portable renderer safety gaps`.
- [x] Milestone 7 complete at reviewed checkpoint `e39537c`; final ledger-only
  closure follows with no implementation changes.

## Milestone 8 — optional Markdown memory harvesting

- [x] Add typed declared memory sources, Markdown glob harvesting, explicit unsupported/none reporting, and legacy snapshots.
- Baseline: branch `agent-agnostic-integrations`; base checkpoint `3f46103`;
  staged, unstaged, and untracked files none.
- Scope/owners: profile model, `internal/harvest`, harvest CLI, harvest tests.
- High-risk surfaces: unknown/none capability observability, multiple or
  overlapping source globs, stable legacy snapshot/blob keys, symlinked or
  non-regular vendor entries, first-run baselines, dry-run inertness, and
  vendor-store read-only behavior.
- Verify: Harvest/Memory/Integration tests plus full gate.
- Skip list/findings carried: raw Go race runs use isolated
  `GOCACHE`/`GOPATH`; stdio MCP remains M9, capability diagnostics/full product
  documentation M10, packaging/zero-edit legacy proof M11, and live-host
  evidence M12. Existing `.state/<machine>/<legacy-id>.json` snapshots and
  `.state/<machine>/<legacy-id>/last/` blobs must remain authoritative.
- Review shards: source typing/safety; diff/snapshot compatibility; generic integration tests.
- Review pass 1: accepted and fixed declaration-order-dependent overlap
  ownership, source-less CLI output, and silently ignored declared symlink
  roots. Harvest now dispatches every typed source; reports `none`, missing,
  and unsupported kinds; preserves the first source's legacy snapshot/blob
  namespace; gives additional sources stable hashed namespaces; retains full
  per-source baselines while deduplicating publication by resolved file and
  persisted content; follows only an explicitly declared root symlink while
  skipping nested symlinks/non-regular entries; rejects unsafe state namespace
  components and path-key collisions; and labels CLI results by stable source
  id. Generic, multiple, overlapping/reordered/re-added, legacy-snapshot,
  symlink, dry-run, and command-output cases have focused coverage. All three
  reviewers and targeted re-reviews are clean. The full
  formatting/vet/test/race/bash/diff gate passed with isolated Go caches.
  Checkpoint `3307002` — `feat: harvest declared memory sources`.
- Pass 2 baseline: checkpoint `3307002`; review the full milestone diff from
  `3f46103`. Carry only isolated-cache requirements and the explicit M9 MCP,
  M10 diagnostics/documentation, M11 packaging/legacy-upgrade proof, and M12
  live-host boundaries; all pass-1 findings are closed.
- Review pass 2: accepted and fixed knowledge-base/source overlap that could
  harvest Regesto's own state; unstable relative integration memory paths;
  parent-symlink aliases that could defeat overlap deduplication; `.state` or
  `inbox` output symlinks/races that could write into a vendor tree; and a
  candidate-file swap that could read through an outside symlink. Harvest now
  preflights every resolved source for all integrations before mutation,
  resolves legacy relative locations against the KB while requiring stable new
  locations, canonicalizes source identities, anchors all KB reads/writes under
  component-verified `os.Root` descriptors, publishes state atomically, and
  verifies an opened Markdown file's identity before reading it. Direct and
  symlinked overlap, output-root alias, relative-path, nested-alias, and
  file-swap regressions are covered. All fresh reviewers and targeted
  re-reviews are clean. Formatting, vet, focused/full tests, race tests, shell
  syntax, and diff checks passed with isolated Go caches. Remediation
  `1cd55d7` — `fix: harden memory harvest boundaries`.
- [x] Milestone 8 complete at reviewed checkpoint `1cd55d7`; final ledger-only
  closure follows with no implementation changes.

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
