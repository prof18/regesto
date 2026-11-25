# Set up OpenCode

Use the main [quick start](../README.md#quick-start) first. This page only covers what is
specific to OpenCode. The [integration matrix](agent-integration.md) contains the exact
profile metadata and capability details.

OpenCode auto-discovers plugins from `~/.config/opencode/plugins/`, so its integration is
deterministic like Claude Code's. On the first message of each session, Regesto's plugin
retrieves the index and the facts scoped to the current repository and injects them into
that message.

OpenCode 2 intercepts prompt admission and appends the context to the message text, while
OpenCode 1 (1.18.29 or newer) uses the `chat.message` hook and appends a context part. The
same plugin file serves both. Portable skills provide the search and write procedures on
demand.

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

1. Links Regesto's skills into the shared `~/.agents/skills/`, which OpenCode loads for
   Claude Code compatibility. Regesto removes any links an earlier version left in
   `~/.config/opencode/skills/`.
2. Creates or updates `~/.config/opencode/AGENTS.md` with a marked knowledge-base
   section. OpenCode always loads this file, and it stacks additively with any
   repository-level `AGENTS.md`.
3. Links the plugin launcher into `~/.config/opencode/plugins/regesto.ts`. OpenCode
   auto-discovers that directory, so Regesto edits no settings file. The linked launcher
   locates your knowledge base on its own, the same way Regesto's other hook wrappers do.
   Foreign `*.ts`/`*.js` files next to it are untouched.

Restart OpenCode after installation, because plugins and skills load once at startup
(OpenCode 2 also reloads watched plugin directories automatically when the linked file
changes).

## What to expect

The first message of every session carries the knowledge-base index and the facts scoped
to the current repository. The agent should still use the search skill for anything the
index only summarizes, and the write skill when you explicitly ask it to remember
something.

If the plugin breaks, it injects nothing and the message still goes through.

## Claude-compatible directories

OpenCode loads skills from `~/.config/opencode/skills`, `~/.claude/skills`, and
`~/.agents/skills`. When the Claude integration links into a separate `~/.claude/skills`,
OpenCode sees the Regesto skill names twice. Every integration installs the same skill
text, so it does not matter which copy OpenCode serves, and nothing needs to be configured.

This duplication does not affect the plugin or the instructions file.

## Use non-default directories

Override the built-in paths only when your setup stores them somewhere else — for
example, when `XDG_CONFIG_HOME` moves OpenCode's configuration away from `~/.config`:

```toml
[integrations.opencode]
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
native markdown memory to harvest. Save anything worth keeping with the write skill, or
promote an exported session with `regesto promote`.
