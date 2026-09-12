# Set up GitHub Copilot CLI

Use the main [quick start](../README.md#quick-start) first. This page only covers what is
specific to GitHub Copilot CLI (and Copilot in VS Code). The
[integration matrix](agent-integration.md) contains the exact profile metadata and
capability details.

Copilot CLI supports `SessionStart` hooks that inject context before the first turn, so
its integration is deterministic like Claude Code's: the hook retrieves the index and the
facts scoped to the current repository, and the agent starts already knowing what exists.
Portable skills provide the search and write procedures on demand.

## Install

Make sure `copilot` is listed in your knowledge base's `config.toml`:

```toml
integrations = ["copilot"]
```

Then preview, install, and verify the integration:

```bash
cd ~/regesto-kb
regesto --config ~/regesto-kb/config.toml install --dry-run
regesto --config ~/regesto-kb/config.toml install
regesto doctor --integration copilot
```

Installation does three things:

1. Links Regesto's skills into `~/.copilot/skills/` — a personal skills directory that
   Copilot CLI and Copilot in VS Code both discover. On machines where `~/.claude/skills`
   symlinks to the shared `~/.agents/skills`, keep this separate target: one directory can
   only carry one skill variant, and Regesto's Claude variant carries Claude-specific
   syntax. To share one directory deliberately, override
   `skills_dir = "~/.agents/skills"`.
2. Creates `~/.copilot/instructions/regesto.instructions.md` with a marked
   knowledge-base section. Copilot has no host-owned user instructions file, so this one
   is Regesto-created; it is always loaded, and in VS Code it stacks additively with any
   repository-level `.github/copilot-instructions.md` or `AGENTS.md`.
3. Registers the SessionStart hook in `~/.copilot/hooks/regesto.json`, a Regesto-owned
   file in the user hooks directory Copilot reads. Foreign entries in other `*.json`
   files in that directory are untouched.

Open a new Copilot session after installation.

## What to expect

Every new session starts with the knowledge-base index and the facts scoped to the
current repository already in context. The agent should still use the search skill for
anything the index only summarizes, and the write skill when you explicitly ask it to
remember something.

A broken hook degrades to injecting nothing — the session always starts.

## Copilot in VS Code

No separate integration is needed. Copilot in VS Code reads the same three surfaces:
`~/.copilot/skills`, `~/.copilot/instructions`, and `~/.copilot/hooks`. Installing the
`copilot` integration covers both hosts.

If user instructions do not appear in VS Code, check that the Copilot Chat extension is
current and look for the file in the instructions menu (`/instructions` in Copilot CLI
lists it too).

## Use non-default directories

Override the built-in paths only when your setup stores them somewhere else:

```toml
[integrations.copilot]
skills_dir = "~/.agents/skills"
instructions_file = "~/.copilot/instructions/regesto.instructions.md"
```

Inspect the resolved configuration and installation plan:

```bash
regesto --config ~/regesto-kb/config.toml config
regesto --config ~/regesto-kb/config.toml install --dry-run
```

## If the hook, skills, or instructions are missing

```bash
regesto doctor --integration copilot
ls -l ~/.copilot/skills/
cat ~/.copilot/hooks/regesto.json
```

`doctor` reports the canonical target and remediation. Rerun `regesto install` after
changing a path. Note that Copilot stops loading `~/.agents/skills` when the
`COPILOT_HOME` environment variable or `--config-dir` overrides its config home.

## Harvest

Copilot's session store is SQLite, so there is no native memory to harvest. Anything worth
keeping should be written through the write skill or promoted from an exported session
with `regesto promote`.
