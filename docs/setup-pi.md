# Set up the Pi coding agent

Use the main [quick start](../README.md#quick-start) first. This page only covers what is
specific to the pi coding agent (`pi`). The
[integration matrix](agent-integration.md) contains the exact profile metadata and
capability details.

pi loads extensions that can inject a message into the agent run, so its integration is
deterministic like Claude Code's: on the first prompt of each session, the Regesto
extension retrieves the index and the facts scoped to the working directory, and the agent
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

1. Links Regesto's skills into `~/.pi/agent/skills/`, one of the global skills directories
   pi scans in every session. pi also auto-discovers the shared `~/.agents/skills`
   directory: if another integration already links Regesto skills there, override
   `skills_dir` so pi sees a single copy instead of colliding names.
2. Adds a marked knowledge-base section to `~/.pi/agent/AGENTS.md`, the global context
   file pi loads in every session; it stacks with a repository's own `AGENTS.md` and
   `CLAUDE.md`. Regesto creates the file when it does not exist and preserves foreign
   content when it does.
3. Links the Regesto extension into `~/.pi/agent/extensions/`, the directory pi
   auto-discovers global extensions from. No settings file is edited; the linked file is
   the whole registration.

Open a new pi session after installation.

## What to expect

The first prompt of every session carries the knowledge-base index and the facts scoped to
the working directory, injected as a persistent custom message the model sees but the
transcript does not print; the session file records it under the `regesto-context` custom
type. The agent should still use the search skill for anything the index only summarizes,
and the write skill when you explicitly ask it to remember something.

A broken extension degrades to injecting nothing — the prompt always proceeds.

## Use non-default directories

Override the built-in paths only when your setup stores them somewhere else:

```toml
[integrations.pi]
skills_dir = "~/.agents/skills"
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
changing a path. pi's startup header lists loaded extensions; start it with `--verbose`
to see them, and `/reload` picks up a repaired link without restarting.

## Harvest

pi's sessions are structured files under `~/.pi/agent/sessions/`, so there is no native
memory to harvest. Anything worth keeping should be written through the write skill or
promoted from an exported conversation with `regesto promote`.
