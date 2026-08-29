# Setup — other agents

The canonical [agent integration matrix](agent-integration.md) lists every built-in profile,
declared capability, consultation guarantee, and evidence boundary. Three product presets
ship today: `claude`, `codex`, and `hermes`. Anything else can use the generic profile with
configured paths; a reusable product preset is a small declarative profile.

## Adding one

An integration profile declares these capabilities:

| | |
|---|---|
| detection | path signals used by init; path and command signals used by diagnostics |
| skills | one or more Agent Skills targets and a portable or tested host variant |
| instructions | one or more always-loaded targets, plus whether Regesto may create them |
| hooks | a host payload protocol, preservation-safe registrar, and settings target |
| native memory | one or more typed sources that `regesto harvest` diffs |
| trust and exclusions | the default capture policy and files that must not be harvested |

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

Run `regesto doctor --integration someagent` after installation. It reports configured and
detected state separately, every planned or current artifact, unsupported capabilities,
memory availability, effective default trust inputs, and an actionable repair command. It
does not write host files or harvest snapshots.

## Agents with no local files at all

Some desktop, mobile, or web chat clients expose neither local integration files nor MCP.
Those clients cannot participate automatically: there is nothing local to install and no
supported way to inject context.

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
