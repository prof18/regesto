# Agent integration matrix

Regesto integrates by capability, not by product identity. A profile declares what a host
can expose: detection signals, Agent Skills targets and variant, always-loaded instructions,
hook protocol and safe registrar, native-memory sources, exclusions, and default trust. A
host gets deterministic consultation only when its declared hook injects Regesto context;
installing the same skills under a different product name does not create that guarantee.

## Built-in profile metadata

This table is checked against `adapters/profiles/*.json` in the test suite. Paths remain
home-relative here because profiles are machine-independent; `regesto config` and `regesto
doctor` show their resolved local values.

<!-- regesto:profile-matrix:start -->
| Profile | Display name | Detection | Skills | Instructions | Hooks | Memory | Exclusions | Default trust |
|---|---|---|---|---|---|---|---|---|
| `claude` | Claude Code | paths=`~/.claude`; commands=`claude` | variant=`claude`; targets=`~/.claude/skills` | targets=`~/.claude/CLAUDE.md`; create=`false` | `claude-session-start-v1` / `claude-settings-json-v1` / `~/.claude/settings.json` | `markdown-glob-v1` / `~/.claude/projects/*/memory` | none | `supervised` |
| `codex` | Codex | paths=`~/.codex`; commands=`codex` | variant=`portable`; targets=`~/.codex/skills` | targets=`~/.codex/AGENTS.md`; create=`false` | none | `markdown-glob-v1` / `~/.codex/memories` | `raw_memories.md` | `supervised` |
| `generic` | Generic integration | paths=none; commands=none | variant=`portable`; targets=none | targets=none; create=`false` | `none` / `none` | `none` | none | `quarantine` |
| `hermes` | Hermes | paths=`~/.hermes`; commands=`hermes` | variant=`portable`; targets=`~/.hermes/skills` | targets=`~/.hermes/SOUL.md`; create=`false` | `hermes-pre-llm-v1` / `hermes-config-yaml-v1` / `~/.hermes/config.yaml` | `markdown-glob-v1` / `~/.hermes/memories` | none | `quarantine` |
<!-- regesto:profile-matrix:end -->

`codex` has no hook entry in its profile. `generic` explicitly declares `none` hook and
memory capabilities so absence remains visible. Codex also excludes `raw_memories.md` from
harvest; exclusions and all raw profile fields are available in the profile JSON.

## Behavior and evidence

| Integration | Consultation path | Guarantee | Evidence in this repository |
|---|---|---|---|
| Claude Code | `SessionStart` injects context | deterministic at the declared hook event | protocol, merge, fail-open, and installer contract tests; live-host verification is release evidence |
| Hermes | first `pre_llm_call` per session injects JSON context | deterministic on the first turn when registration is current | protocol, session-bound, YAML recipe, allowlist, and installer contract tests; live-host verification is release evidence |
| Codex | always-loaded instructions plus portable skills | advisory, not enforced | renderer and installer contract tests |
| Generic/custom | whichever declared targets and supported protocol the instance profile supplies | determined by declared capabilities | profile validation, renderer, and installer contract tests |

“Tested” here means the protocol and registration boundary is covered by automated fixtures.
It is intentionally separate from a dated live-host result; those release records belong in
the validation ledger rather than being implied forever by this matrix.

## Configure an unknown host

You do not need a built-in preset. Find the host's Agent Skills directory and its
always-loaded instructions file, create the instructions file if the host has not, then add:

```toml
integrations = ["my-agent"]

[integrations.my-agent]
profile = "generic"
skills_dir = "~/.my-agent/skills"
instructions_file = "~/.my-agent/AGENTS.md"
trust = "quarantine"
```

If the host has a markdown memory directory, declare it explicitly:

```toml
memory_kind = "markdown-glob-v1"
memory_location = "~/.my-agent/memory"
```

Then inspect, install, and diagnose without opening a product-specific page:

```bash
regesto --config ~/regesto-kb/config.toml install --dry-run
regesto --config ~/regesto-kb/config.toml install
regesto --config ~/regesto-kb/config.toml doctor --integration my-agent
```

The generic profile never creates a host-owned instructions file and never guesses a hook
format. For a host hook, copy `generic.json` into the instance as
`adapters/profiles/<id>.json`, then change its `id` to the filename ID and replace the
`none` hook with a supported `protocol`, the `manual` registrar, and the host settings
path. The filename and JSON `id` must match. Use an automatic registrar only when Regesto
has a preservation-safe implementation for that settings format. The install plan then
prints the exact event and command instead of rewriting an unfamiliar file.

## Hosts without local integration files

Use the local stdio MCP server when a client supports MCP:

```bash
regesto --config ~/regesto-kb/config.toml mcp
```

It exposes search, exact fact reads, project resolution, validated writes, the generated
index, and fact resources over stdin/stdout only. It opens no network listener. If a host
supports neither local skills/instructions nor local MCP, export the conversation and run
the portable `regesto-promote` procedure. No silently skipped capability is treated as an
installation success; `regesto doctor --json` reports `unsupported`, `warning`, `manual`,
or `error` explicitly.
