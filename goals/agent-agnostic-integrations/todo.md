# Agent-agnostic integrations — execution ledger

Durable checklist for milestones 0–12. Each milestone closes only after its
focused checks, the full gate, both fresh-context review passes, and its
checkpoint commit(s) are recorded below.

Full gate (required after each pass): `gofmt -l .`; `go vet ./...`; `go test ./...`;
`go test -race ./...`; `git diff --check`.

## Simplified branch history

After completion, the local-only branch history was consolidated to one coherent
commit per milestone so implementation, review fixes, plan updates, and this ledger
travel together. The active sequence is M0 `e924493`, M1 `eb1992c`, M2 `22fbb3e`,
M3 `185e511`, M4 `5f87742`, M5 `8dafe64`, M6 `fa387e9`, M7 `2087a25`, M8
`95f7814`, M9 `5e7760e`, M10 `92e149f`, M11 `b492b78`, and M12 at the current
branch tip. Historical pass/checkpoint hashes below remain reachable only through
the local recovery ref `refs/archive/agent-agnostic-integrations-pre-flatten`; they
record the review sequence but are no longer separate commits in the active branch.

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

- [x] Port Claude/Hermes hook parsing, framing, registration/manual recipes, bounded first-turn behavior, and fail-open handling to Go.
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

- [x] Implement stdlib JSON-RPC/MCP handshake, resources, tools, clean shutdown, and shared validation with no socket.
- Baseline: branch `agent-agnostic-integrations`; base checkpoint `dfa32c2`;
  staged, unstaged, and untracked files none.
- Scope/owners: `internal/mcp`, MCP CLI, MCP protocol tests/transcript.
- High-risk surfaces: newline-delimited JSON-RPC framing and stdout purity,
  initialize/initialized lifecycle ordering, request-id preservation,
  notification silence, bounded malformed input, capability/schema accuracy,
  resource URI traversal, fact identity ambiguity, tool error versus protocol
  error semantics, write provenance/trust parity, and clean EOF shutdown.
- Verify: MCP/JSONRPC tests; initialize→tools/list→tools/call→resources/read stdin transcript; race/full gate.
- Skip list/findings carried (including dependency decision): the official MCP
  lifecycle, tools, resources, and stdio transport are small enough to implement
  confidently with `encoding/json` and buffered I/O under transcript tests, so
  no SDK or module dependency is introduced. Raw Go race runs use isolated
  `GOCACHE`/`GOPATH`; capability diagnostics and full documentation remain M10,
  packaging/legacy-upgrade proof M11, and live-host evidence M12.
- Review shards: protocol conformance; stdout/error framing and no-network posture; tool validation/tests.
- Review pass 1: accepted and fixed invalid UTF-8 normalization, unbounded
  JSON nesting/store reads/response lines, protocol-extension over-rejection,
  null cursor/meta acceptance, missing-resource-URI misclassification, raw
  filesystem error disclosure, backend/not-found conflation, notification
  side effects, unchecked/ambiguous resource identities, symlink escape/swap
  reads, and ignored CLI arguments. The stdlib server now pins protocol
  revision 2025-06-18, requires initialized operation,
  preserves request ids, emits only bounded newline-delimited JSON-RPC, reads
  canonical facts through rooted descriptor-verified limits, and exposes the
  four planned tools plus live index/fact resources through shared operations.
  Recorded success/error/lifecycle/extension/limit/URI/symlink/write tests and
  all targeted re-reviews are clean. Focused tests and the full
  formatting/vet/test/race/bash/no-network/diff gate passed with isolated Go
  caches. Checkpoint `73badac` — `feat: add local stdio MCP adapter`.
- Pass 2 baseline: checkpoint `73badac`; review the full milestone diff from
  `dfa32c2`. Carry only isolated-cache requirements and the explicit M10
  diagnostics/documentation, M11 packaging/legacy-upgrade proof, and M12
  live-host boundaries; all pass-1 findings are closed.
- Review pass 2: accepted and fixed false compatibility claims for older MCP
  schemas/batching, invalid ping/initialized metadata, lossy empty-topic
  rendering, approximate/per-ID-only capacity enforcement, unrooted canonical
  fact reads and lock creation, and missing fact-count bounds. The server now
  advertises only revision 2025-06-18; shared fact loading rejects symlinks and
  verifies rooted descriptors; every validated writer holds a rooted KB-wide
  store lock through load, exact rendered/store/count limits, and publication;
  MCP writes pass 4 MiB fact, 8 MiB store, and 10,000-fact limits as
  authority-owned constraints. Concurrent different-ID capacity, external fact
  and lock-root symlink, exact render expansion, metadata, negotiation, and
  empty-topic regressions are covered. All fresh reviewers and targeted
  re-reviews are clean. Focused tests and the full
  formatting/vet/test/race/bash/no-network/diff gate passed with isolated Go
  caches. Remediation `4cab257` — `fix: harden MCP and shared write
  boundaries`.
- [x] Milestone 9 complete at reviewed checkpoint `4cab257`; final ledger-only
  closure follows with no implementation changes.

## Milestone 10 — diagnostics and documentation

- [x] Add `doctor`, canonical capability matrix, product recipes, Hermes documentation, and profile-checked claims.
- Baseline: branch `agent-agnostic-integrations`; base checkpoint `7ea861a`;
  staged, unstaged, and untracked files none. The personal knowledge base has
  no recorded claim governing doctor or integration-documentation behavior.
- Scope/owners: doctor CLI; README/DESIGN/CONTRIBUTING/SCHEMA; docs and init template.
- High-risk surfaces: distinguishing configured/detected/installed/tested and
  deterministic/probabilistic behavior; filesystem checks without mutation;
  custom/generic profile remediation; hook registration versus protocol
  availability; memory missing/none/unsupported reporting; trust-policy
  precedence; canonical matrix drift from profile declarations; product recipe
  cross-links; and claims about closed hosts or live validation.
- Verify: Doctor/Docs/Profile tests; stale-claim `rg`; full gate.
- Skip list/findings carried: raw Go race runs use isolated `GOCACHE`/`GOPATH`;
  v0.3.1 standalone packaging proof remains M11 and live Claude/Codex/Hermes
  host evidence remains M12. Documentation may call a protocol/fixture tested
  only when it says so, and may call a product live-tested only after M12.
- Review shards: diagnostic accuracy; capability matrix/recipe consistency; documentation terminology.
- Pre-pass-1 state: implementation adds the read-only, versioned `doctor`
  report and CLI; deterministic capability/artifact/memory/trust diagnostics;
  canonical profile-derived documentation matrix; standalone generic and MCP
  recipe; product cross-links; canonical new-instance configuration vocabulary;
  and focused filesystem-inertness, repeatability, filter, matrix, recipe, and
  init-template tests. Legacy `agents` parsing and fixtures remain unchanged.
- Initial verification: `go test ./tests -run 'Doctor|Docs|Profile'` passed;
  the stale-claim search has no match; formatting, vet, all normal and race
  tests, explicit shell syntax checks, and `git diff --check` passed with
  isolated Go caches.
- Review pass 1: checkpoint `7e1dbde` (`feat: add integration diagnostics
  and matrix`) received three fresh-context audits. Accepted and fixed omitted
  detected-but-unconfigured profiles; global and filtered plan-error poisoning;
  unsupported-only success; blank legacy-unknown trust; multi-hook status
  fan-out; missing text trust/remedies; mismatched new-config/legacy override
  examples; unconditional product-name guarantees; an invalid custom-profile
  recipe; matrix exclusion/set drift; path-versus-command detection wording;
  inactive empty `integrations`; incomplete host-path inertness; and missing
  malformed/unsupported memory regressions. The report now falls back to
  per-integration plans after a global failure, publishes full trust precedence,
  and keeps unsupported/unmanaged states actionable. Focused tests and the full
  formatting/vet/test/race/bash/diff gate passed with isolated Go caches;
  remediation checkpoint follows.
- Pass 2 baseline: remediation checkpoint `c92253b` (`fix: close
  integration diagnostic gaps`); review the full M10 diff from `7ea861a` with
  all pass-1 regressions active.
- Review pass 2: accepted and fixed harvest-incompatible direct/symlink memory
  overlap being reported healthy; same-protocol hooks with different settings
  cross-associating; and remaining unconditional hook, Codex, generic-target,
  quickstart, and upgrade language. Doctor now canonicalizes the KB and memory
  roots before applying the same overlap refusal as harvest, while hook
  association falls back to protocol only for settings-less hooks. Direct,
  symlink, and same-protocol regressions pass. All three targeted re-reviews are
  clean; focused checks and the full formatting/vet/test/race/bash/diff gate
  passed with isolated Go caches. Remediation `35a0772` — `fix: align
  diagnostics with capability boundaries`.
- [x] Milestone 10 complete at reviewed checkpoint `35a0772`; final ledger-only
  closure follows with no implementation changes.

## Milestone 11 — upgrade proof and packaging

- [x] Extend CI/fixtures for v0.3.1 zero-edit upgrade, dry-run/install/doctor/MCP, and package result.
- Baseline: branch `agent-agnostic-integrations`; base checkpoint `dd644b3`;
  staged, unstaged, and untracked files none.
- Scope/owners: upgrade/engine tests, CI workflow, changelog.
- High-risk surfaces: byte-preserving legacy config; historical manifest hash
  attribution; dry-run inertness across instance and HOME; upgrade ownership and
  executable modes; adapter reinstallation into an isolated HOME; compiled
  engine/instance separation; line-delimited MCP stdout; CI shell portability;
  and release-note/version conventions.
- Verify: full gate and standalone temporary-HOME proof from plan; inspect failures before cleanup.
- Skip list/findings carried: use isolated Go caches under the sandbox; live
  product behavior remains M12. No test or CI command may read or modify the
  real HOME. The standalone proof keeps its temporary directory for inspection
  until all checks finish.
- Review shards: migration/ownership; CI and packaging; standalone proof/release docs.
- Pre-pass-1 state: committed historical package bytes and manifest hashes now
  back a compiled-engine v0.3.1 upgrade regression. It snapshots the instance
  and isolated HOME around dry-runs, applies the upgrade, proves config bytes,
  current package contents, modes, manifest ownership, rendered host artifacts,
  doctor JSON, and MCP JSONL, then proves repeat inertness. Standalone CI now
  covers temporary-HOME install dry-run/apply/no-op, doctor, and MCP. Upgrade
  dry-run plans adapter rendering from the running engine's packaged sources,
  matching the real post-refresh order instead of rejecting historical skills.
- Initial verification: fixture hashes validate against the v0.3.1 tag; the
  focused compiled-upgrade regression passed; formatting, vet, all normal and
  race tests, explicit shell syntax, and diff checks passed with isolated Go
  caches. The plan's standalone proof passed and remains inspectable at
  `/private/tmp/regesto-m11-proof.gACPD5`.
- Review pass 1: checkpoint `364d8ad` (`test: prove standalone legacy
  upgrades`) received three fresh-context audits. Accepted and fixed a dry-run
  source view that substituted packaged bytes even where real upgrade would
  preserve an edited adapter; a historical fixture whose own manifest was not
  asserted before migration; executable-only rather than exact package-mode
  checks; an environment-dependent doctor inventory assertion; an incomplete
  release-artifact gate; and release wording that did not distinguish CI's
  dry-run from the compiled fixture's applied upgrade. Dry-run now builds a
  temporary post-upgrade adapter tree by refreshing/removing only
  manifest-authorized paths and threads that source root through profiles,
  skills, hooks, and instructions while keeping real host targets. A modified
  portable-skill regression proves dry-run/apply agreement and preservation.
  The release tarball smoke now runs isolated-HOME install
  dry-run/apply/no-op, doctor, MCP, and actual v0.3.1 apply/repeat with config
  byte checks. All targeted re-reviews are clean. The exact standalone workflow
  and release-tarball smoke passed at retained directories
  `/private/tmp/regesto-m11-ci-20260829-43104-uqjkn3` and
  `/private/tmp/regesto-m11-release-20260829-44753-59oy4d`; formatting, vet,
  focused/full tests, race tests, explicit shell syntax, and diff checks passed
  with isolated Go caches. Remediation `128e29c` — `fix: close standalone
  upgrade proof gaps`.
- Pass 2 baseline: remediation checkpoint `128e29c` (`fix: close standalone
  upgrade proof gaps`); review the full M11 diff from `dd644b3` with all pass-1
  regressions active. Carry only isolated-cache requirements and the explicit
  M12 live-host boundary; all pass-1 findings are closed.
- Review pass 2: accepted and fixed release proof that established internal
  idempotence without comparing the applied v0.3.1 result to the release
  package; shallow MCP assertions that did not identify tools, resources, or a
  served payload; doctor checks that proved only configured ids; and no exact
  tar inventory assertion. The release smoke now requires one executable in
  the archive, byte-identical deterministic doctor reports with current
  capabilities/artifacts/trust, the exact four MCP tools plus index resource
  content, and every manifest-owned legacy byte/mode/hash to equal a fresh
  instance from the same stamped tarball. It also verifies the installed legacy
  links and single instruction sections. The compiled fixture mirrors MCP
  surface/payload checks and includes the historical empty knowledge layout.
  All three fresh reviewers and targeted re-reviews are clean. The exact
  strengthened tarball smoke passed at
  `/private/tmp/regesto-m11-release-p2-20260829-68725-qm5370`; formatting, vet,
  full and race tests, current and fixture shell syntax, and diff checks passed
  with isolated Go caches. Remediation `528d91e` — `test: strengthen release
  package proof`.
- [x] Milestone 11 complete at reviewed checkpoint `528d91e`; final ledger-only
  closure follows with no implementation changes.

## Milestone 12 — live regression matrix

- [x] After automated gates, run and record Claude Code, Codex, Hermes, and generic fixture live checks; back up installer targets and dry-run first.
- Baseline: branch `agent-agnostic-integrations`; base checkpoint `3198fc2`;
  staged files none and the initial live dry-run produced no host writes. The
  milestone owns only live setup evidence, temporary fixtures, this ledger,
  and a narrowly scoped upgrade migration fix discovered by that dry-run.
- Scope/owners: host artifacts and temporary fixture only; preserve retained
  backups and every unrelated local file. Live installer targets are the
  resolved Claude/Codex/Hermes skills directories, shared Claude/Codex
  instructions, Claude JSON settings, Hermes `SOUL.md`, YAML hook settings and
  JSON allowlist, plus instance engine-owned files.
- High-risk surfaces: pre-marker legacy skill ownership, live shared dotfiles,
  lossless Hermes YAML handling, hook first-turn state, real CLI prompt cost,
  exact source trust, harvesting without capturing unrelated memory, and
  proving dry-run/apply agreement before host mutation.
- Verify: skills/instructions, hooks, trust, memory baseline/change, generic
  no-hook behavior; record commands and observed versions; focused installer
  migration regressions and the full gate after each review pass.
- Observed product versions before setup: Claude Code `2.1.216`; Codex CLI
  `0.144.6`; Hermes Agent `0.20.0 (2026.8.3)` with Python `3.11.15` and OpenAI
  SDK `2.24.0`. The development engine was rebuilt from checkpoint `3198fc2`
  and reached through the existing `~/.local/bin/regesto` checkout symlink.
- Initial dry-run command: `bin/regesto --config
  /Users/mg/regesto-kb/config.toml upgrade --dry-run`. It planned 16 instance
  updates and initially preserved all nine live skill links because the v0.3.1
  `.state/skills` tree predates ownership markers. No write occurred.
- Accepted pre-review finding: location alone cannot authorize adopting the
  old stage, but preserving it leaves Claude-rendered payloads active for Codex
  and Hermes. Upgrade now captures proof before refreshing sources and may
  replace a legacy link only when the complete unmarked stage is byte-identical
  to the historical shell renderer's output from the current instance source.
  Changed, extra, and non-regular trees remain foreign. Focused tests pass; a
  repeated live dry-run now plans all nine links as replacements while still
  preserving the unmarked evidence directories.
- Skip list/findings carried: isolated Go caches remain required by the
  workspace sandbox. Existing unfamiliar Hermes YAML is never automatically
  rewritten; use the exact manual recipe after backup if its current hook is
  stale. No proprietary cloud memory or remote connector is in scope.
- Live backup/apply evidence: the 53-entry restore archive
  `/private/tmp/regesto-m12-backup-20260829-5JBHNz/before.tar` has SHA-256
  `4b16a5724f9067235736431ee87361e7acd46814fd5adc6a450cb0ec33ed5c6b`.
  Applying the reviewed plan refreshed 16 instance files and 30 adapter items,
  including nine proven legacy links; it created adjacent backups for the
  Hermes allowlist, shared instructions, and `SOUL.md`. The unfamiliar Hermes
  YAML was preserved, then its one stale hook line was changed through a
  reviewed temporary copy to the exact quoted `pre-llm.sh` recipe after a
  precondition hash check. `hermes hooks doctor` reports the hook executable,
  allowlisted, valid JSON, and healthy. A repeated upgrade dry-run reports zero
  instance and adapter changes (the YAML registrar remains intentionally
  `manual` because the installer never claims to parse arbitrary YAML).
- Live artifact/hook evidence: all nine Claude/Codex/Hermes links resolve to
  their integration-specific rendered stages; the shared Claude/Codex file and
  Hermes `SOUL.md` each contain exactly one marker pair. Claude's installed
  wrapper injects the Regesto project heading and fact
  `dec-agent-agnostic-per-milestone-review-commits`; a missing KB returns empty
  output with status zero. The registered Hermes wrapper injects that project
  fact on the first call for a unique session and returns `{}` on the second;
  a missing KB also returns `{}` with status zero. Codex declares and installs
  no hook. Doctor reports current skills/instructions for all three and the
  exact legacy trust approval `hermes@studio` for the private Telegram channel;
  Hermes and undeclared integrations retain quarantine defaults.
- Live memory evidence: isolated instance
  `/private/tmp/regesto-m12-memory-AYlggB/kb` pointed typed sources at uniquely
  named directories inside each product's real native memory root. First run
  baselined one Markdown file per product and captured none; one subsequent
  change produced exactly one Claude, Codex, and Hermes capture; repeat
  produced zero. After direct user approval, the three exact
  `regesto-m12-live` test directories were moved from the Claude, Codex, and
  Hermes native roots into the retained backup's `native-memory-probes`
  directory. All source paths are absent and all three probe files are present
  in the backup; no adjacent native memory was moved.
- Generic evidence: retained fixture
  `/private/tmp/regesto-m12-generic-4Ly3xa` installed three portable skill
  links and one marker-delimited instruction section into pre-created declared
  targets, preserving foreign text; the second plan had zero changes. Its
  profile declares no detection or hook capability. Typed Markdown memory
  baselined once, captured exactly one change, and was inert on repeat;
  `normalize --show-prompt` returned `no eligible captures`, proving the
  default-quarantine capture did not reach normalization.
- Accepted pass-1 live/review findings: the first real 4 KiB hook payload let
  the mature global store consume every project line; bounded context now
  retains global-first display while selecting current-project facts first,
  strictly honors `MaxBytes` (or returns empty when even the search header
  cannot fit), omits empty project blocks, and exactly reports dropped facts.
  Legacy skill proof is no longer a stale name boolean: the historical expected
  file tree is rechecked during planning and immediately before link mutation;
  `SKILL.md`, exact directories, and regular payloads are required. Regressions
  cover post-plan change, empty/hidden-only evidence, extra files/directories,
  symlinks, byte changes, exact caps, empty projects, and dropped counts.
- Consent boundary and F28 closure: initial external attempts were rejected
  without disclosure-specific consent and no request was sent. After the user
  directly approved bounded private-context disclosure and exact probe cleanup,
  fresh Claude (`--no-session-persistence`), Codex (`--ephemeral`, read-only),
  and Hermes (one-shot, low reasoning) runs each invoked the installed
  `regesto-search` skill for `Hermes Regesto capability` and returned exactly
  `Hermes uses Regesto's tested capability-profile integration` with status
  zero. Hermes created local session `20260829_232555_e8f3ed`; it was deleted by
  exact ID and confirmed absent, preserving the non-persistent test boundary.
- Reproducible local transcripts: live dry-run/doctor/artifact/fail-open checks
  are retained at `/private/tmp/regesto-m12-live-local-transcript.txt`; generic
  installation/quarantine and three-product memory repeat/capture paths are at
  `/private/tmp/regesto-m12-fixture-transcript.txt`. The successful first/second
  Hermes context probe required the live instance's session-marker write and
  reported `hermes-first-project-fact=present`, `hermes-second=empty`; its
  fail-open form is repeated in the local transcript without a marker write.
  Consent-gated external-session and cleanup evidence is retained at
  `/private/tmp/regesto-m12-external-session-transcript.txt`, SHA-256
  `7a13bf5341d3f05f7631e1153ff1887b9cc208a1e2ca6033c4aadbce33a2d744`;
  it records only modes, statuses, the returned title, and exact cleanup paths.
- Review shards: Claude live path; Codex/generic portability; Hermes hook/trust; evidence/release checklist.
- Review pass 1: three fresh-context shards reviewed migration/ownership,
  bounded context/hooks, and evidence completeness. Accepted findings were the
  stale proof TOCTOU, empty/unexpected proof trees, post-publication rollback
  races, unbounded footer/tiny caps, empty-project budget use, dropped-count
  coverage, and initially thin ledger evidence. Proof publication now
  atomically captures the exact existing link and uses no-replace links for
  replacement/recovery, preserving and reporting rollback artifacts if a
  concurrent entry prevents safe restoration; five targeted migration
  re-reviews and the context re-review are clean. Focused install/context/hook/
  upgrade tests pass. The isolated-cache formatting, vet, full test, race test,
  current-wrapper shell syntax, and diff gate passed after the final fix.
  External product sessions and probe cleanup are explicitly deferred pending
  consent, so F28 and this milestone remain open. Checkpoint commit follows.
- Pass-1 checkpoint `83008f7` — `fix: harden live integration migration`.
- Review pass 2: three fresh-context reviewers independently found no actionable
  migration, context/hook, or evidence defects in the full `3198fc2..fb22ac4`
  milestone diff. Focused install/context/hook/upgrade tests, race variants, vet,
  and diff checks passed in the review shards. The final isolated-cache gate also
  passed formatting, repository-wide vet, full tests, race tests, current-wrapper
  shell syntax, and diff checks. No remediation commit is required. The
  subsequently authorized live sessions and cleanup close F28 without changing
  reviewed implementation code; final closure gate and ledger checkpoint follow.
- [x] Milestone 12 complete: implementation checkpoint `83008f7`, pass-1 ledger
  checkpoint `fb22ac4`, pass-2 ledger checkpoint `12fdb09`, and consent-gated
  live/cleanup evidence recorded above; final ledger-only checkpoint follows.

## Final completion record

- Fact audit: M2's validated data-driven capability profiles, additive legacy
  resolution, built-in identifiers, and support model prove F01, F02, F03, F04,
  F05, and F06. M3's
  source/surface policy and opaque provenance compatibility prove F19, F20, and
  F30. M4's shared operations and stable JSON/text interfaces prove F10 and F11.
  M5's rooted plan/apply ownership, backups, idempotence, and manifest lifecycle
  prove F12, F13, and F25. M6's protocol-owned Claude/Hermes fail-open hooks and
  explicit Codex `none` capability prove F14, F15, F16, and F17. M7's portable contract,
  isolated variants, generic declared targets, and host-neutral promotion prove
  F08, F09, F18, and F23. M8's typed optional Markdown harvesting and explicit
  unsupported states prove F21. M9's validated stdio-only MCP proves F22. M10's
  Hermes recipe, canonical capability matrix, doctor, and configured/tested/
  deterministic distinctions prove F07, F24, and F26. M11's exact v0.3.1
  upgrade/package equivalence and contract matrix prove F03, F25, and F27. M12's
  dated live Claude, Codex, Hermes, and generic evidence proves F07, F15, F16,
  F17, F18, and F28. F29 remains an explicit exclusion: the diff adds no remote HTTP
  exposure, hosted synchronization, or proprietary cloud-memory connector.
- Final gate after live closure passed `gofmt -l .`, repository-wide `go vet`,
  full tests, race tests, current wrapper shell syntax, and `git diff --check`
  with isolated Go caches. The final audit found every F01-F30 identifier mapped
  above, every M0-M12 pass recorded, and only this reviewed ledger edit pending.
- [x] All accepted facts and automated checks accounted for; F28 live tests recorded; out-of-scope F29 preserved.
- [x] Every milestone has two recorded review passes, gates, findings disposition, and cohesive checkpoint commit(s).
- [x] Working tree contains only reviewed goal changes; no remote service or proprietary cloud-memory connector introduced.
