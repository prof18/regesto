# Setup — other agents

Three built-in adapters ship today: `claude`, `codex`, and `hermes`. Hermes has a tested
protocol boundary and installer, while validation against a live Hermes host remains on
the release checklist. See [setup-hermes.md](setup-hermes.md) for its exact registration
and probe commands. Anything else can use the generic profile with configured paths; a
reusable product preset is a small declarative profile.

## Adding one

An integration profile declares four capability groups:

| | |
|---|---|
| skills directory | where `<name>/SKILL.md` is discovered |
| instructions file | the always-loaded file the pointer section is appended to |
| settings file | where a hook is registered, if the platform has hooks |
| native memory | the files `regesto harvest` diffs, if it has any |

For a reusable preset, add `adapters/profiles/<id>.json`. Use the `portable` skills variant
unless the host has a tested optimization. An optional append-only variant belongs under
`adapters/variants/<variant>/`, declares every hook protocol it requires, and is rendered
only for profiles that select it. No Go table or knowledge-format change is needed. See
[CONTRIBUTING.md](../CONTRIBUTING.md#adding-an-agent).

For a one-off host, configure the generic profile and its actual local paths:

```toml
integrations = ["someagent"]

[integrations.someagent]
profile = "generic"
skills_dir = "~/.someagent/skills"
instructions_file = "~/.someagent/AGENTS.md"
```

Create the instructions file first if the host has not done so; the generic profile does
not invent a host-owned file. Installation then links a separate portable render tree and
merges the shared instruction section without a source change. If the host has a hook,
describe it in an instance-owned profile with a
supported protocol plus `manual` registrar; installation prints the exact event/command
recipe instead of guessing a settings format. Legacy `agents` and override tables remain
accepted for existing configurations.

## Agents with no local files at all

Chat apps — the Claude and ChatGPT desktop and mobile apps — cannot participate
automatically. There is nothing on disk to read and no way to inject context.

The path for those is manual and deliberate: export the conversation, then

```
/regesto-promote ~/Downloads/conversation.json
```

The `regesto-promote` skill extracts durable facts, writes them through the same
`regesto-write` procedure, and moves the transcript into `archive/chat-exports/`. Its
portable trigger and procedure require an explicit user request because it is a batch
operation with side effects.

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
