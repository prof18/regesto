# Setup — other agents

Three built-in adapters ship today: `claude`, `codex`, and `hermes`. Hermes has a tested
protocol boundary and installer, while validation against a live Hermes host remains on
the release checklist. See [setup-hermes.md](setup-hermes.md) for its exact registration
and probe commands. Anything else needs an adapter, which is small.

## Adding one

An agent is four locations and nothing else:

| | |
|---|---|
| skills directory | where `<name>/SKILL.md` is discovered |
| instructions file | the always-loaded file the pointer section is appended to |
| settings file | where a hook is registered, if the platform has hooks |
| native memory | the files `regesto harvest` diffs, if it has any |

Add the defaults to `internal/adapters/adapters.go`, add a test asserting they sit under
`$HOME` and hardcode no personal path element, and you are done. Nothing under
`knowledge/` changes and `SCHEMA.md` does not move. See
[CONTRIBUTING.md](../CONTRIBUTING.md#adding-an-agent).

Until then, an unknown agent named in `config.toml` is **reported rather than skipped**, so
you get a warning instead of silence:

```toml
agents = ["claude", "someagent"]
```

```
! someagent: no skills dir known — set [skills_dirs].someagent in config.toml
```

Set the three locations yourself and the generic install path works with no code change at
all:

```toml
[skills_dirs]
someagent = "~/.someagent/skills"
[instructions]
someagent = "~/.someagent/AGENTS.md"
[settings_files]
someagent = "~/.someagent/settings.json"
```

The settings entry is only useful if the agent's hook configuration happens to have the
same shape as Claude Code's; leave it unset otherwise and rely on skills and instructions.

## Agents with no local files at all

Chat apps — the Claude and ChatGPT desktop and mobile apps — cannot participate
automatically. There is nothing on disk to read and no way to inject context.

The path for those is manual and deliberate: export the conversation, then

```
/regesto-promote ~/Downloads/conversation.json
```

The `regesto-promote` skill extracts durable facts, writes them through the same
`regesto-write` procedure, and moves the transcript into `archive/chat-exports/`. It has
`disable-model-invocation: true` — it is a batch operation with side effects and runs only
when you ask.

## What not to build

**Do not point a vendor's auto-memory directory at the knowledge base.** It points at one
directory, which collapses per-project isolation, and it makes the vendor's pruning your
problem. Harvest-by-diff needs no cooperation from the vendor and cannot be broken by a
change on their side.

**Do not expose the knowledge base over the network to reach a phone.** That is a
permanently listening service holding everything you know, and there is a better shape:
run a chat-surface agent on your own machine, making outbound connections only. Trust is
then an explicit source policy: private status alone does not grant it; unknown or shared
surfaces quarantine until a human records a supervised rule or promotes the capture.
[DESIGN.md §8](../DESIGN.md#8-phones).
