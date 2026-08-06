# Setup — Codex CLI

Codex has no hook that can inject text before the first prompt, so consultation here rests
on **skills plus instructions**: likely, not guaranteed. Everything else — the same facts,
the same schema, the same `regesto-write` procedure — is identical to Claude Code.

## Install

`regesto init` already added `codex` to `agents` in `config.toml` if `~/.codex` existed on
this machine at the time. Add it yourself otherwise, or if this instance predates that
detection:

```toml
# config.toml
agents = ["claude", "codex"]
```

```bash
~/regesto-kb/bin/regesto-install
```

This links the three skills into `~/.codex/skills/` and appends the knowledge-base section
to `~/.codex/AGENTS.md`. No hook is registered — Codex has no settings file for one, and
install skips that step rather than warning about it.

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

### The skill prints a command instead of results

Expected, and handled. `regesto-search` uses Claude Code's `` !`command` `` dynamic context
injection, which runs the search *before* the skill body reaches the model. That preamble
is a Claude Code feature, not part of the agentskills.io standard: **Codex receives the
line literally, unexecuted, and does not expand `$ARGUMENTS` either.**

The skill body opens by telling the agent which of the two cases it is in, and in the
literal case to run the command itself with the query substituted. Codex does that and
returns the right fact. If you see it print the command and stop, the skill body was
truncated or edited — re-run `bin/regesto-install`.

The same limitation means **slash invocation with arguments is not portable**.
`/regesto-write some claim` substitutes the claim on Claude Code and does not on Codex, so
the skill also tells the agent to take the claim from the message you actually sent.

### The skills are not listed at all

```bash
ls -l ~/.codex/skills/
regesto config | grep codex
```

Install skips an agent whose parent directory does not exist on this machine, on the
assumption it is not installed here. Create `~/.codex/` first, or point
`[skills_dirs].codex` at wherever yours lives.

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
