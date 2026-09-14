# Set up Antigravity CLI

Use the main [quick start](../README.md#quick-start) first. This page only covers what is
specific to Google Antigravity CLI (`agy`). The
[integration matrix](agent-integration.md) contains the exact profile metadata and
capability details.

Antigravity supports `PreInvocation` hooks that run before every model call and can inject
steps into the conversation, so its integration is deterministic like Claude Code's: on the
first invocation of each conversation, the hook retrieves the index and the facts scoped to
the mounted workspace, and the agent starts already knowing what exists. Portable skills
provide the search and write procedures on demand.

## Install

Make sure `antigravity` is listed in your knowledge base's `config.toml`:

```toml
integrations = ["antigravity"]
```

Then preview, install, and verify the integration:

```bash
cd ~/regesto-kb
regesto --config ~/regesto-kb/config.toml install --dry-run
regesto --config ~/regesto-kb/config.toml install
regesto doctor --integration antigravity
```

Installation does three things:

1. Links Regesto's skills into `~/.gemini/antigravity-cli/skills/` — the global skills
   directory `agy` imports as slash commands in every session. A repository's own
   `.agents/skills/` is a separate, per-workspace surface Regesto does not touch: these
   integrations are machine-global, and per-repo scoping happens in the hook at runtime.
2. Adds a marked knowledge-base section to `~/.gemini/GEMINI.md`, the global context file
   Antigravity consults in every session; it stacks with a repository's own `AGENTS.md`
   and `GEMINI.md`. Regesto creates the file when it does not exist and preserves foreign
   content when it does. The file is also Gemini CLI's global context file: on a machine
   with both, both products read the section, but Gemini CLI loads skills from
   `~/.gemini/skills`, not `~/.gemini/antigravity-cli/skills` — override `instructions_file`
   if you want the surfaces kept apart.
3. Registers the PreInvocation hook under the `regesto` name in
   `~/.gemini/config/hooks.json`, the customization directory shared by the Antigravity
   harness. Foreign hook names in that file are untouched.

Open a new Antigravity session after installation.

## What to expect

The first model call of every conversation carries the knowledge-base index and the facts
scoped to the first mounted workspace. The agent should still use the search skill for
anything the index only summarizes, and the write skill when you explicitly ask it to
remember something.

A broken hook degrades to injecting nothing — the model call always proceeds. A session
without a mounted workspace (for example a bare `agy --print` outside any project) also
starts without injection: Antigravity reports `workspacePaths` only for mounted
workspaces, and Regesto does not guess a scope the host did not declare.

## Use non-default directories

Override the built-in paths only when your setup stores them somewhere else:

```toml
[integrations.antigravity]
skills_dir = "~/.agents/skills"
instructions_file = "~/.gemini/GEMINI.md"
```

Inspect the resolved configuration and installation plan:

```bash
regesto --config ~/regesto-kb/config.toml config
regesto --config ~/regesto-kb/config.toml install --dry-run
```

## If the hook, skills, or instructions are missing

```bash
regesto doctor --integration antigravity
ls -l ~/.gemini/antigravity-cli/skills/
cat ~/.gemini/config/hooks.json
```

`doctor` reports the canonical target and remediation. Rerun `regesto install` after
changing a path. Inspect loaded hooks inside a session with `/hooks`.

## Harvest

Antigravity's transcripts are JSONL plus SQLite under `~/.gemini/antigravity-cli/brain/`,
so there is no native memory to harvest. Anything worth keeping should be written through
the write skill or promoted from an exported conversation with `regesto promote`.
