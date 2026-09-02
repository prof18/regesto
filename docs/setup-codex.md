# Setup — Codex CLI

Not sure whether this is the right integration path? Start with
[Connect any agent to Regesto](setup-other-agents.md).

See the canonical [agent integration matrix](agent-integration.md) for profile metadata,
capability status, and consultation guarantees.

The built-in `codex` profile declares no hook that injects text before the first prompt,
so consultation through that profile rests on **skills plus instructions**: likely, not
guaranteed. Everything else — the same facts, schema, and `regesto-write` procedure — is
shared with the other integrations.

## Install

`regesto init` already added `codex` to `integrations` in `config.toml` if `~/.codex` existed on
this machine at the time. Add it yourself otherwise, or if this instance predates that
detection:

```toml
# config.toml
integrations = ["claude", "codex"]
```

```bash
~/regesto-kb/bin/regesto-install
```

This links the shipped portable skills into `~/.codex/skills/` and appends the
knowledge-base section to `~/.codex/AGENTS.md`. No hook is registered because the built-in
profile declares none; install treats that capability as unsupported rather than guessing
a settings format.

If both agents share one instructions file (a symlink into a dotfiles repo, say), install
notices they resolve to the same path and appends the section once.

## What consultation looks like here

The always-loaded section in `AGENTS.md` tells the agent that canonical knowledge exists,
where it is, and how to search it. The `regesto-search` skill carries the procedure. In
practice the agent reaches for it on questions about past decisions and conventions, less
reliably on everything else it holds, and sometimes not at all.

Two things make that better:

- **Ask for it once.** "Check the knowledge base first" at the top of a session is usually
  enough for the rest of it.
- **Keep the section short.** It competes with everything else in `AGENTS.md`. The shipped
  template is deliberately about thirty lines.

---

## Troubleshooting

### The skill reports a command without running it

Codex receives the portable `regesto-search` body, with no host-specific pre-execution
syntax. That body tells the agent to call `bin/regesto-search` with the user's actual query
and then read matching claims. If it merely repeats the command, ask it to execute the
documented procedure and check that the instance shims are executable. Re-run
`bin/regesto-install` if the rendered skill was truncated or edited.

Explicit skill invocation syntax varies by host. The portable `regesto-write` procedure
therefore takes the claim from the user's visible request and never relies on argument
substitution.

### The skills are not listed at all

```bash
ls -l ~/.codex/skills/
regesto --config ~/regesto-kb/config.toml config | grep codex
```

Install creates a configured skills directory when it is missing. If the links are not
listed, inspect the complete resolved plan with
`regesto --config ~/regesto-kb/config.toml install --dry-run`; for a custom location,
configure the resolved integration directly:

```toml
[integrations.codex]
skills_dir = "~/.my-codex/skills"
```

### Facts written by Codex have the wrong project

Codex's native memory is global rather than per-project, so a capture harvested from it has
no repo attached. Normalisation offers the model the canonical project names in use and
asks it to pick, which is why the vocabulary in `INDEX.md` matters: a model that cannot see
the names invents a spelling and the project's knowledge splits in two.

If it picks wrong, fix the `scope:` in the fact file and re-run `regesto index`. Facts are
plain markdown; editing one by hand is a supported operation, not a workaround.

### Everything else

The general failure modes — silent hooks, machine names drifting, harvest windows, project
name collisions — are the same across agents and are written up in
[setup-claude-code.md](setup-claude-code.md#troubleshooting).
