# Agent integration matrix

This is the advanced capability reference. For normal setup, use the
[main quick start](../README.md#quick-start) or
[custom-agent guide](setup-other-agents.md#custom-local-integration).

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

## Configure an unknown local host

The guided onboarding procedure is in
[Connect any agent to Regesto](setup-other-agents.md#custom-local-integration). For a
one-off local host, you do not need a built-in preset or any Go changes. The standalone
reference configuration is:

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
format. This `config.toml`-only setup is the normal path.

For a reusable preset or a compatible host hook, copy the instance file
`~/regesto-kb/adapters/profiles/generic.json` to
`~/regesto-kb/adapters/profiles/<id>.json`, then change its `id` to the filename ID.
Replace the `none` hook with a supported `protocol`, the `manual` registrar, and the host
settings path. Use an automatic registrar only when Regesto has a preservation-safe
implementation for that settings format. The install plan then prints the exact event
and command instead of rewriting an unfamiliar file.

## Hosts without local integration files

Use the local stdio MCP server when a client supports MCP. MCP addresses the instance
directly, so an MCP-only client does not need to appear in `integrations`:

```json
{
  "mcpServers": {
    "regesto": {
      "command": "regesto",
      "args": ["--config", "/Users/you/regesto-kb/config.toml", "mcp"]
    }
  }
}
```

If the client does not inherit your shell `PATH`, replace `regesto` with the output of
`command -v regesto`. The knowledge-base path must be absolute because MCP clients may not
expand `~`. The equivalent shell command is
`regesto --config ~/regesto-kb/config.toml mcp`. It exposes search,
exact fact reads, project resolution, validated writes, the generated index, and fact
resources over stdin/stdout only. Diagnostics go to stderr and it opens no network
listener. MCP makes the tools available; persistent client instructions are still needed
if consultation must be habitual rather than requested.

If a host supports neither local skills/instructions nor local MCP, export the
conversation and run `regesto promote <export>` from the knowledge-base directory. No
silently skipped capability is treated as an installation success; `regesto doctor`
reports `unsupported` for an intentionally absent capability, `manual` for a safe human
step, and `warning` or `error` when remediation is required.
