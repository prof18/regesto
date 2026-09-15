# Set up OpenCode

Use the main [quick start](../README.md#quick-start) first. This page only covers what is
specific to OpenCode. The [integration matrix](agent-integration.md) contains the exact
profile metadata and capability details.

OpenCode auto-discovers plugins from `~/.config/opencode/plugins/` and calls their
`chat.message` hook on every user message, so its integration is deterministic like Claude
Code's: on the first message of each session, Regesto's plugin retrieves the index and the
facts scoped to the current repository and injects them as a context part. Portable skills
provide the search and write procedures on demand.

## Install

Make sure `opencode` is listed in your knowledge base's `config.toml`:

```toml
integrations = ["opencode"]
```

Then preview, install, and verify the integration:

```bash
cd ~/regesto-kb
regesto --config ~/regesto-kb/config.toml install --dry-run
regesto --config ~/regesto-kb/config.toml install
regesto doctor --integration opencode
```

Installation does three things:

1. Links Regesto's skills into `~/.config/opencode/skills/`, the global skills directory
   OpenCode discovers.
2. Creates or updates `~/.config/opencode/AGENTS.md` with a marked knowledge-base
   section. OpenCode always loads this file, and it stacks additively with any
   repository-level `AGENTS.md`.
3. Links the plugin launcher into `~/.config/opencode/plugins/regesto.ts`. OpenCode
   auto-discovers that directory, so no settings file is edited; the linked launcher
   locates your knowledge base on its own, the same way Regesto's other hook wrappers do.
   Foreign `*.ts`/`*.js` files next to it are untouched.

Quit and restart OpenCode after installation — plugins and skills load once at startup.

## What to expect

The first message of every session carries the knowledge-base index and the facts scoped
to the current repository. The agent should still use the search skill for anything the
index only summarizes, and the write skill when you explicitly ask it to remember
something.

A broken plugin degrades to injecting nothing — the message always goes through.

## Claude-compatible directories

OpenCode also auto-loads skills from `~/.claude/skills` and `~/.agents/skills` for Claude
Code compatibility. Skill names must be unique across all locations, so on a machine that
also runs the Claude integration the same skill would appear twice with different
variants. Keep Regesto's skills in `~/.config/opencode/skills` and silence the duplicate
scan:

```bash
export OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1
```

The plugin and instructions surfaces are unaffected by that variable.

## Use non-default directories

Override the built-in paths only when your setup stores them somewhere else — for
example, when `XDG_CONFIG_HOME` moves OpenCode's configuration away from `~/.config`:

```toml
[integrations.opencode]
skills_dir = "~/.config/opencode/skills"
instructions_file = "~/.config/opencode/AGENTS.md"
settings_file = "~/.config/opencode/plugins/regesto.ts"
```

Inspect the resolved configuration and installation plan:

```bash
regesto --config ~/regesto-kb/config.toml config
regesto --config ~/regesto-kb/config.toml install --dry-run
```

## If the plugin, skills, or instructions are missing

```bash
regesto doctor --integration opencode
ls -l ~/.config/opencode/plugins/regesto.ts
head ~/.config/opencode/AGENTS.md
```

`doctor` reports the canonical target and remediation. Rerun `regesto install` after
changing a path, then restart OpenCode.

## Harvest

OpenCode's sessions are structured JSON under `~/.local/share/opencode`, so there is no
native markdown memory to harvest. Anything worth keeping should be written through the
write skill or promoted from an exported session with `regesto promote`.
