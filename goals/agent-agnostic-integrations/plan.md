# Agent-agnostic integrations — implementation plan

## Solution approach

Keep the knowledge format and existing instances stable while replacing the closed vendor table with a capability-driven integration layer. Portable behavior lives in common CLI operations, Agent Skills, instructions, and stdio MCP; product profiles contribute discovery defaults, rendering choices, hook protocols, hook registration, and memory sources. Existing `claude`, `codex`, and `hermes` identifiers remain valid aliases so the migration is additive.

The work is deliberately staged. Every milestone ends with its own compatibility gate, two iterative codebase-review passes, and a committed checkpoint, so implementation can stop, review, or split without leaving a half-migrated installer in users' instances or accumulating one oversized final commit.

## Specification

### Support levels

| Level | Capability | Promise |
|---|---|---|
| Core | Local CLI or MCP access | The host can search, read, and submit validated facts. |
| Portable | Agent Skills and/or persistent instructions | Regesto is discoverable, but model compliance is probabilistic. |
| Integrated | Context-injection hook or plugin | Consultation is deterministic when the host invokes the hook successfully. |
| Harvested | Accessible native-memory source | New native-memory writes are captured as a safety net. |
| Manual | Transcript import | Closed hosts can contribute through explicit human promotion. |

These are independent capabilities, not a ranking of vendors. Documentation must state configured, live-tested, and deterministic status separately.

### Integration profile

Add data-driven profiles under `adapters/profiles/<id>.json`, loaded with the other instance-owned adapter files. A profile has this logical schema:

```json
{
  "schema_version": 1,
  "id": "claude",
  "display_name": "Claude Code",
  "detect": {"paths": ["~/.claude"], "commands": ["claude"]},
  "skills": {"targets": ["~/.claude/skills"], "variant": "claude"},
  "instructions": {"targets": ["~/.claude/CLAUDE.md"], "create": false},
  "hooks": [
    {
      "protocol": "claude-session-start-v1",
      "registrar": "claude-settings-json-v1",
      "settings": "~/.claude/settings.json"
    }
  ],
  "memory": [
    {"kind": "markdown-glob-v1", "location": "~/.claude/projects/*/memory"}
  ],
  "default_trust": "supervised"
}
```

Rules:

- Profile JSON is declarative and parsed with the standard library.
- `id`, capability kinds, protocol names, and registrar names are validated; unknown kinds are reported, never guessed.
- Built-in registrars are small Go implementations. A custom profile can always declare skills, instructions, Markdown memory, and `manual` hook instructions without compiling Go.
- Existing identifiers remain `claude`, `codex`, and `hermes`; terminology changes without changing source provenance.
- Paths are home-expanded at resolution time and may be overridden by instance config.
- No profile may contain an absolute personal path.

### Configuration compatibility

Continue accepting:

```toml
agents = ["claude", "codex", "hermes"]
[skills_dirs]
claude = "~/.agents/skills"
[instructions]
claude = "~/.dotfiles/AGENTS.md"
[settings_files]
claude = "~/.claude/settings.json"
[memory_dirs]
claude = "~/.claude/projects/*/memory"
```

New instances may use:

```toml
integrations = ["claude", "codex", "hermes", "my-agent"]

[integrations.my-agent]
profile = "generic"
skills_dir = "~/.my-agent/skills"
instructions_file = "~/.my-agent/AGENTS.md"
memory_kind = "markdown-glob-v1"
memory_location = "~/.my-agent/memory"
trust = "quarantine"
```

Resolution rules:

1. Reject a config that sets both `agents` and `integrations`.
2. Treat legacy `agents` as the integration list with unchanged identifiers.
3. Apply `[integrations.<id>]` values over profile defaults.
4. Apply legacy override tables last for old configs, preserving their exact current behavior.
5. A custom integration defaults to no hook, no memory, portable Agent Skills, and quarantine.
6. `regesto config --json` exposes the resolved capabilities; current key/value output stays byte-compatible for legacy keys.

### Trust policy

Trust belongs to a source/surface, not a product name.

- `human` remains authoritative.
- A built-in supervised local profile may default to `supervised`; the built-in
  Claude and Codex profiles do so.
- The built-in Hermes profile defaults to `quarantine`. Its supervised private
  channel is trusted only through the existing exact `[trusted_sources]`
  entry, preserving current behavior without treating every Hermes surface as
  supervised.
- A custom, unknown, unattended, or channel-based integration defaults to `quarantine`.
- Existing `[trusted_sources]` exact entries upgrade a matching source to trusted.
- A new optional `[source_policies]` table accepts exact source ids and simple `*` suffix patterns with values `supervised` or `quarantine`.
- Exact rules beat patterns; instance rules beat profile defaults; quarantine is the final fallback.
- `source` remains `<integration>@<machine>` and needs no fact migration. A
  profile's default applies only after exact and pattern source rules. Because
  the source syntax intentionally carries no separate channel field, supervised
  and unattended/channel surfaces must use distinct integration ids whenever
  they need different defaults; an unattended/channel profile must declare
  `quarantine`. Unknown profiles and omitted defaults always resolve to
  `quarantine`.

### Portable skill contract

The common skill frontmatter relies only on portable fields: `name`, `description`, and optional standard metadata. The common body never depends on dynamic command expansion, slash-argument substitution, or vendor tool names.

- `regesto-search` always tells the host to call the stable search CLI or MCP tool with the user's actual query.
- `regesto-write` calls the validated write interface after choosing the semantic fields; Regesto supplies time, machine, output path, and validation.
- `regesto-promote` is described as explicit transcript promotion for any inaccessible host.
- A renderer may add a host optimization in `.state/integrations/<id>/skills`, but the portable source remains authoritative.
- Render tests inspect every profile and fail when a variant contains a protocol the profile did not declare.

### Machine-facing commands

Add or extend:

```text
regesto search --json ...
regesto context --json ...
regesto project --json ...
regesto config --json
regesto install [--dry-run] [--json]
regesto doctor [--integration ID] [--json]
regesto write --source SOURCE [--dir DIR] [--json-input] [--json]
regesto hook <protocol>
regesto mcp
```

`regesto write --json-input` reads one object from stdin containing `id`, `title`, `type`, `scope`, `subject`, `relation`, optional `topics`, optional `status`/`supersedes`, `body`, and `why`. It stamps `schema_version`, `source`, `created`, and `modified`; resolves the project when requested; validates the candidate; writes atomically; and returns the relative path plus pending reconciliation. It never accepts an empty source and never silently labels an agent assertion `human`.

Existing human-readable command output and bin shims remain supported.

### Hook protocol boundary

`regesto hook <protocol>` reads the host payload from stdin and writes exactly the host response format to stdout. Debugging goes to stderr. Every hook is fail-open for host availability while never leaking malformed partial output.

Required protocols:

- `claude-session-start-v1`: extract `workspace.current_dir` or `cwd`; emit plain context text; exit zero with no text on failure.
- `hermes-pre-llm-v1`: honor `extra.is_first_turn`, fall back to a bounded per-session marker, extract `cwd`, emit `{}` or `{"context":"..."}`.
- `none`: no registration and an explicit portable fallback.

Registration is separate from payload translation:

- `claude-settings-json-v1` merges a command into `hooks.SessionStart` using Go JSON handling and a backup.
- `hermes-config-yaml-v1` owns the narrow Hermes configuration/allowlist update. If the existing YAML cannot be modified without preserving it safely, installation produces an exact manual recipe and `doctor` reports the remaining action rather than rewriting uncertain YAML.
- `manual` prints the command/event/protocol recipe for a custom host.

### Local MCP boundary

`regesto mcp` is stdio-only and opens no socket. It implements the smallest useful MCP surface:

- Resources: `regesto://index`, `regesto://facts/<id>`.
- Tools: `regesto_search`, `regesto_get_fact`, `regesto_resolve_project`, `regesto_write_fact`.
- All handlers call the same internal operations as the CLI.
- Write calls require an explicit source and return validation/reconciliation state.
- Protocol errors are JSON-RPC errors; operational failures never corrupt stdout framing.
- Remote HTTP transport and authentication are excluded.

## Ordered implementation

### Completion protocol for every milestone

Apply this protocol after **every numbered milestone**, including milestone 0. A milestone is not complete until both review passes, verification, and the appropriate commits are recorded. Both passes are required even when pass 1 reports no findings.

Before pass 1, record the milestone review baseline in `goals/agent-agnostic-integrations/todo.md`:

- milestone number and intended behavior;
- branch, base commit, and staged/unstaged/untracked state;
- files and owner boundaries in scope;
- high-risk surfaces and the milestone's verification commands;
- current skip list of fixed, rejected, deferred, or repeated findings;
- planned fresh-context reviewer shards.

For each pass:

1. Launch fresh-context, read-only reviewers with non-overlapping scopes appropriate to the milestone. Across cross-cutting milestones, cover configuration/trust, installer/hooks/upgrade, MCP/CLI/protocol behavior, and tests/docs/release automation. Review the entire milestone diff against its recorded base, not only unstaged changes.
2. Independently inspect the milestone's highest-risk paths in the main agent while reviewers run.
3. Treat every reviewer result as advisory. Reproduce or prove each finding in the real code path and classify it as an in-scope blocker, follow-up, or stop-and-escalate decision.
4. Fix accepted in-scope findings with focused regression tests. Keep unrelated work and locally edited instance files untouched.
5. Run focused milestone verification, followed by the full repository gate. If a fix or generated artifact invalidates earlier evidence, rerun the affected review and tests before closing the pass.
6. Record accepted, fixed, rejected, deferred, and repeated findings in `todo.md`; carry the updated skip list and baseline into pass 2.

Required full gate after each pass:

```bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
git diff --check
```

Commit discipline:

- Start each milestone from the preceding milestone's committed checkpoint.
- After pass 1 is closed and its gates pass, commit the milestone implementation and pass-1 fixes as one or more cohesive, reviewable commits. Split independent code, migration, test, or documentation changes when that improves review or rollback; do not create arbitrary file-by-file commits.
- Run pass 2 against that committed checkpoint and the updated skip list. If pass 2 finds issues, fix and verify them, then create a separate focused remediation commit. If pass 2 is clean, do not create an empty commit.
- If pass 2 changes code, perform a targeted re-review of the affected bug class and sibling surfaces within pass 2, then rerun the full gate before committing the remediation.
- Record every commit hash and one-line purpose in `todo.md`. Do not defer milestone commits to the end, squash the whole goal into a single bag commit, amend, or push unless the user explicitly requests it.
- Preserve unrelated pre-existing changes. If the milestone cannot be isolated safely, stop and ask before staging or committing.

Milestone acceptance: both passes are recorded; all accepted in-scope findings are fixed and verified; rejected and deferred findings have reasons; the required gates pass after the final changes; stop-and-escalate items are reported precisely; and the milestone ends at a documented committed checkpoint with no uncommitted changes from that milestone.

### 0. Start safely and capture the baseline

At implementation start—not during planning:

```bash
cd /Users/mg/Workspace/regesto
git status --short
git switch -c agent-agnostic-integrations
gofmt -l .
go vet ./...
go test ./...
```

Record the baseline before changing implementation files. The expected planning state is the untracked `goals/agent-agnostic-integrations/` package only; stop if any unrelated change cannot be identified and isolated. Do not carry changes from `~/regesto-kb` into the public engine repository.

Create `goals/agent-agnostic-integrations/todo.md` from the numbered milestones and the completion protocol. Commit the reviewed goal package as the first branch checkpoint before implementation work begins.

Verification: status before branch creation contains only the expected goal package; the current full gate passes; the reviewed goal package is committed as the first checkpoint; and status is clean before milestone 1 begins. The planning baseline on 2026-08-29 already passes `gofmt -l .`, `go vet ./...`, and `go test ./...`.

### 1. Add compatibility fixtures before changing behavior

Touch:

- `tests/fixtures/config/legacy-*.toml` (new)
- `tests/integrations_test.go` (new)
- `tests/adapters_test.go`
- `tests/upgrade_test.go`

Add golden assertions for the current Claude/Codex/Hermes resolved paths, detection, legacy config output, instance file list, and trust behavior. Add a synthetic generic profile fixture.

Verification:

```bash
go test ./tests -run 'Adapter|Integration|Upgrade|Trust'
```

The new tests initially describe current compatibility and the target generic profile; target-only cases may be skipped with explicit TODO references until their milestone lands.

### 2. Introduce the profile loader and additive configuration model

Touch:

- `adapters/profiles/claude.json` (new)
- `adapters/profiles/codex.json` (new)
- `adapters/profiles/hermes.json` (new)
- `adapters/profiles/generic.json` (new)
- `internal/adapters/adapters.go`
- `internal/adapters/profile.go` (new)
- `internal/config/config.go`
- `cmd/regesto/showconfig.go`
- `cmd/regesto/init.go`
- `scaffold.go`

Load and validate embedded/instance profiles, map legacy `agents` to resolved integrations, support `[integrations.<id>]`, and preserve `adapters.For` through a compatibility wrapper until callers migrate.

Verification:

```bash
go test ./tests -run 'Adapter|Integration|Config|InstanceFiles'
go test ./...
```

Acceptance: legacy fixtures resolve exactly as before; a custom profile resolves its portable capabilities without code changes; invalid kinds produce actionable errors.

### 3. Generalize source trust before enabling arbitrary integrations

Touch:

- `internal/normalize/normalize.go`
- `internal/normalize/run.go`
- `internal/config/config.go`
- `tests/normalize_test.go`
- `SCHEMA.md`

Replace the Hermes-name conditional with a policy resolver. Preserve `[trusted_sources]`; use profile defaults and safe quarantine fallback. Clarify the schema text without changing fact fields or `schema_version`.

Verification:

```bash
go test ./tests -run 'Normalize|Trust|Human'
```

Acceptance: an unknown integration is quarantined; trusted Claude/Codex compatibility remains; Hermes is trusted only through the declared private source; quarantined facts never reach search/index/context.

### 4. Add stable JSON operations and validated writes

Touch:

- `internal/write/` (new)
- `cmd/regesto/write.go` (new)
- `cmd/regesto/main.go`
- `cmd/regesto/search.go` or current search path
- `cmd/regesto/context.go`
- `cmd/regesto/project.go`
- `cmd/regesto/showconfig.go`
- `internal/normalize/run.go` (extract candidate validation for reuse)
- `tests/write_test.go` (new)
- existing command tests

Extract candidate stamping/validation/atomic writing from normalization into a shared operation. Add JSON output without changing existing text output.

Verification:

```bash
go test ./tests -run 'Write|Search|Context|Project|Config'
go test -race ./...
```

Acceptance: malformed input writes nothing; duplicate ids fail; source and timestamps cannot be forged through JSON input; project paths are correct; an agent claim contesting a human claim is reported for review.

### 5. Move installation planning and application into Go

Touch:

- `internal/install/plan.go` (new)
- `internal/install/apply.go` (new)
- `internal/install/instructions.go` (new)
- `internal/install/skills.go` (new)
- `internal/install/hooks.go` (new)
- `cmd/regesto/install.go` (new)
- `bin/regesto-install` (reduce to a shim)
- `cmd/regesto/upgrade.go`
- `tests/install_test.go` (new)

Represent every install mutation as a plan item with owner, target, current state, intended state, backup action, and dry-run description. Use `filepath.EvalSymlinks`; remove the `readlink -f` and `jq` runtime dependencies. Preserve foreign files and deduplicate shared real paths.

Group instruction mutations by canonical target with a stable integration-id
order. Start from `filepath.Abs` plus `filepath.Clean`; when the complete target
exists, use `filepath.EvalSymlinks` on it. When it does not, walk upward to the
nearest existing ancestor, resolve that ancestor with `EvalSymlinks`, and append
the cleaned missing suffix without creating anything. Identical Regesto
sections produce one plan item whose owner list names every integration, one
backup before the first real mutation, and one marker-delimited update that
preserves all foreign content. If integrations render different instruction
sections to the same canonical target, planning fails with an actionable
conflict instead of selecting one by iteration order. Dry runs expose the
declared path, resolved canonical target, grouping, backup, and conflict without
writing. A configured path or profile default deliberately authorizes its
resolved target even when a symlink places it outside HOME or the knowledge
base; this preserves existing dotfiles overrides. Apply re-resolves the target
immediately before mutation and refuses if it differs from the planned
canonical target, and the backup is created beside the resolved target before
the first edit.

Verification:

```bash
go test ./tests -run 'Install|Upgrade|Manifest'
go test -race ./...
```

Run installer tests under a temporary HOME. Assert zero changes after dry-run and after a second real install.
Include fixtures where two integrations share an existing instruction target,
where they share a not-yet-created target beneath a symlinked ancestor, and
where their rendered sections conflict. The symlink fixture resolves outside
the temporary HOME and asserts explicit dry-run disclosure, backup placement,
and refusal after a symlink-target swap.

### 6. Implement protocol-owned hooks and preserve Claude/Hermes behavior

Touch:

- `internal/hooks/claude.go` (new)
- `internal/hooks/hermes.go` (new)
- `internal/hooks/runner.go` (new)
- `cmd/regesto/hook.go` (new)
- `bin/regesto-hook` (new shim)
- `adapters/claude/hooks/` (compatibility wrapper or retirement through manifest)
- `adapters/hermes/hooks/` (ship the tested integration)
- `tests/hooks_test.go` (new)
- `tests/fixtures/hooks/` (new payload/result fixtures)

Port JSON parsing/encoding into Go so hooks do not require `jq`. Add bounded Hermes session markers and protocol-specific registrars/manual recipes.

Verification:

```bash
go test ./tests -run 'Hook|Claude|Hermes'
bash -n bin/* adapters/*/hooks/*.sh
```

Manual probes:

```bash
printf '%s' '{"workspace":{"current_dir":"/tmp/project"}}' | regesto hook claude-session-start-v1
printf '%s' '{"cwd":"/tmp/project","session_id":"s1","extra":{"is_first_turn":true}}' | regesto hook hermes-pre-llm-v1
printf '%s' '{"cwd":"/tmp/project","session_id":"s1","extra":{"is_first_turn":false}}' | regesto hook hermes-pre-llm-v1
```

Acceptance: output framing exactly matches each host; repeat Hermes calls inject once; malformed payloads exit zero with empty host-valid output.

### 7. Render portable skills and instructions by declared capabilities

Touch:

- `adapters/skills/regesto-search/SKILL.md`
- `adapters/skills/regesto-write/SKILL.md`
- `adapters/skills/regesto-promote/SKILL.md`
- `adapters/instructions/regesto-section.md`
- optional `adapters/variants/<profile>/` overlays (new)
- `internal/install/skills.go`
- `tests/skills_test.go` (new)

Make the common sources vendor-neutral and spec-valid. Render one profile-specific target tree per integration, including only declared optimizations.

Verification:

```bash
go test ./tests -run 'Skill|Instruction|Render'
rg -n 'Claude Code|Codex|Hermes|\$ARGUMENTS|!`' adapters/skills adapters/instructions
```

Any remaining product name or syntax must be inside a clearly profile-specific overlay or compatibility note.

### 8. Make file-memory harvesting a declared optional capability

Touch:

- `internal/adapters/profile.go`
- `internal/harvest/harvest.go`
- `cmd/regesto/harvest.go`
- `tests/harvest_test.go`

Replace the single implicit glob with one or more typed memory sources. Implement `markdown-glob-v1` using the existing diff engine. Report `none` and unknown kinds explicitly. Preserve snapshot locations for legacy ids so upgrades do not recapture whole stores.

Verification:

```bash
go test ./tests -run 'Harvest|Memory|Integration'
```

Acceptance: existing snapshots remain usable; generic Markdown memory captures; unsupported kinds do not run or disappear silently; vendor files remain read-only.

### 9. Add the local stdio MCP adapter

Touch:

- `internal/mcp/server.go` (new)
- `internal/mcp/protocol.go` (new)
- `cmd/regesto/mcp.go` (new)
- `tests/mcp_test.go` (new)

Implement the minimal JSON-RPC/MCP handshake, list/read resources, list/call tools, and clean shutdown. Keep the engine's no-network posture. If the protocol cannot be implemented confidently with the standard library, stop at this milestone and make dependency selection an explicit reviewed decision rather than introducing an SDK silently.

Verification:

```bash
go test ./tests -run 'MCP|JSONRPC'
go test -race ./...
```

Feed a recorded initialize → tools/list → tools/call → resources/read transcript through stdin and assert parseable JSON-RPC responses only on stdout.

### 10. Add diagnostics and finish documentation

Touch:

- `cmd/regesto/doctor.go` (new)
- `README.md`
- `DESIGN.md`
- `CONTRIBUTING.md`
- `SCHEMA.md`
- `docs/agent-integration.md` (new canonical matrix)
- `docs/setup-claude-code.md`
- `docs/setup-codex.md`
- `docs/setup-hermes.md` (new)
- `docs/setup-other-agents.md`
- `cmd/regesto/init.go` config template

Document Hermes as tested. Explain that deterministic consultation follows hooks, not product identity. Generate or test the compatibility matrix against profile metadata so docs cannot claim capabilities the profiles do not declare.

Verification:

```bash
go test ./tests -run 'Doctor|Docs|Profile'
rg -n -i 'only.*Claude|two adapters|Hermes.*not.*tested' README.md DESIGN.md docs adapters SCHEMA.md
```

Acceptance: an unknown-host user can install portable integration without reading a Claude/Codex page; every product page links to the canonical matrix.

### 11. Prove engine/instance upgrades and package the result

Touch:

- `tests/upgrade_test.go`
- `tests/engine_test.go`
- `.github/workflows/ci.yml`
- `CHANGELOG.md`

Extend the standalone CI test to run `regesto install --dry-run`, install against a temporary HOME, call `doctor`, and exercise the MCP handshake. Add a fixture representing a v0.3.1 instance and upgrade it without modifying its config.

Verification:

```bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
git diff --check
```

Standalone proof:

```bash
tmp="$(mktemp -d)"
go build -o "$tmp/bin/regesto" ./cmd/regesto
PATH="$tmp/bin:$PATH" HOME="$tmp/home" regesto init --dir "$tmp/kb" --machine ci --examples
PATH="$tmp/bin:$PATH" HOME="$tmp/home" regesto --config "$tmp/kb/config.toml" install --dry-run
PATH="$tmp/bin:$PATH" HOME="$tmp/home" regesto --config "$tmp/kb/config.toml" doctor --json
PATH="$tmp/bin:$PATH" HOME="$tmp/home" regesto --config "$tmp/kb/config.toml" lint
```

Do not delete the temporary directory from the implementation command itself until its files have been inspected when a failure occurs.

### 12. Live regression matrix

Run only after automated gates pass. Back up every host file the installer may edit and use the installer's dry-run first.

Claude Code:

- Skills appear and load.
- Instruction section is present exactly once.
- A fresh project session receives the correct project context before the first prompt.
- A broken context command does not block session startup.
- Native-memory baseline and one subsequent Markdown change harvest correctly.

Codex:

- Skills appear and the portable search procedure executes the real query.
- Instruction section is present exactly once.
- No hook guarantee is claimed or installed.
- Native-memory baseline and one subsequent change harvest correctly.

Hermes:

- Skills and `SOUL.md` instructions are present exactly once.
- The registered `pre_llm_call` hook injects context on the first turn and not the second.
- Hook failures return `{}` and do not break the conversation.
- The private Telegram source remains trusted through explicit configuration; an undeclared source remains quarantined.
- Native-memory baseline and one subsequent change harvest correctly.

Generic fixture:

- A temporary custom profile installs portable skills/instructions into configured paths.
- No hook is guessed.
- Markdown memory works when declared.
- Trust defaults to quarantine.

Record the commands and observed versions in the implementation task before declaring completion.

## Risks and decision gates

- **MCP dependency:** the repository currently has no module dependencies. Do not add an SDK implicitly. Attempt the small stdlib boundary only if protocol conformance tests are sufficient; otherwise pause for an explicit dependency decision.
- **Hermes YAML preservation:** never rewrite unfamiliar YAML with a lossy parser. Fall back to an exact manual recipe and a failing `doctor` capability until a preservation-safe registrar exists.
- **Upgrade ownership:** profile and hook retirement must use manifest hashes; locally edited adapter files remain untouched and reported.
- **Trust regression:** arbitrary integrations must not become trusted merely because they have filesystem paths. Generalize trust before enabling custom profiles.
- **Terminology churn:** retain `agents` and existing ids indefinitely as compatibility vocabulary even after docs prefer integrations.
- **Scope control:** additional product presets are follow-up work. The generic profile and contract tests prove extensibility in this goal.
- **Closed hosts:** no documentation may imply automatic integration when the product exposes no local files, hooks, CLI, plugin API, or MCP.

## Definition of done

All facts in `facts.md` hold; every automated fact selected in `facts.meta.json` has a passing check; every numbered milestone has completed two iterative codebase-review passes with accepted findings remediated and verified; every milestone ends in recorded cohesive commits rather than one final bag commit; the full and race gates pass; a v0.3.1-style instance upgrades without editing `config.toml`; live Claude, Codex, and Hermes regressions pass; documentation calls Hermes tested and describes support by capability; no remote service is introduced; and the working tree contains only reviewed goal changes.
