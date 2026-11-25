# Set up the Pi coding agent

Use the main [quick start](../README.md#quick-start) first. This page only covers what is
specific to the pi coding agent (`pi`). The
[integration matrix](agent-integration.md) contains the exact profile metadata and
capability details.

pi loads extensions that can inject a message into the agent run, so its integration is
deterministic like Claude Code's. On the first prompt of each session, the Regesto
extension retrieves the index and the facts scoped to the working directory, so the agent
starts already knowing what exists. Portable skills provide the search and write
procedures on demand.

## Install

Make sure `pi` is listed in your knowledge base's `config.toml`:

```toml
integrations = ["pi"]
```

Then preview, install, and verify the integration:

```bash
cd ~/regesto-kb
regesto --config ~/regesto-kb/config.toml install --dry-run
regesto --config ~/regesto-kb/config.toml install
regesto doctor --integration pi
```

Installation does three things:

1. Links Regesto's skills into the shared `~/.agents/skills/`, which pi auto-discovers in
   every session alongside `~/.pi/agent/skills/`. Regesto removes any links an earlier
   version left in `~/.pi/agent/skills/`.
2. Adds a marked knowledge-base section to `~/.pi/agent/AGENTS.md`, the global context
   file pi loads in every session. It stacks with a repository's own `AGENTS.md` and
   `CLAUDE.md`. Regesto creates the file when it does not exist and preserves foreign
   content when it does.
3. Links the Regesto extension into `~/.pi/agent/extensions/`, the directory pi
   auto-discovers global extensions from. Regesto edits no settings file, because the
   linked file is the whole registration.

Open a new pi session after installation.

## What to expect

The first prompt of every session carries the knowledge-base index and the facts scoped to
the working directory. The extension injects them as a persistent custom message that the
model sees but the transcript does not print. The session file records it under the
`regesto-context` custom type.

The agent should still use the search skill for anything the index only summarizes, and the
write skill when you explicitly ask it to remember something.

If the extension breaks, it injects nothing and the prompt still proceeds.

## Use non-default directories

Override the built-in paths only when your setup stores them somewhere else:

```toml
[integrations.pi]
skills_dir = "~/.pi/agent/skills"
instructions_file = "~/.pi/agent/AGENTS.md"
```

Inspect the resolved configuration and installation plan:

```bash
regesto --config ~/regesto-kb/config.toml config
regesto --config ~/regesto-kb/config.toml install --dry-run
```

## If the extension, skills, or instructions are missing

```bash
regesto doctor --integration pi
ls -l ~/.pi/agent/extensions/
cat ~/.pi/agent/AGENTS.md
```

`doctor` reports the canonical target and remediation. Rerun `regesto install` after
changing a path. pi's startup header lists loaded extensions when you start pi with
`--verbose`. Run `/reload` to pick up a repaired link without restarting.

## Harvest

pi's sessions are structured files under `~/.pi/agent/sessions/`, so there is no native
memory to harvest. Save anything worth keeping with the write skill, or promote an
exported conversation with `regesto promote`.
