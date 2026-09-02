# Regesto

**A knowledge base your agents consult — not a memory they carry.**

> *[Regèsto](https://it.wikipedia.org/wiki/Regesto)* (Latin *regĕsta*, "carried back"): an
> archivist's register of an archive too large to read end to end — one short entry per
> document, giving its date, its source and what it is about. Nineteenth-century historians
> compiled them for medieval collections that could never be published in full.

Every coding agent now ships a memory file. All of them are working-set caches: bounded,
pruned by the agent to stay under a cap, lossy by design. That is right for a cache and
wrong for the decisions you spent a week arriving at.

Regesto keeps those decisions in plain markdown you own, one claim per file, and makes the
agent read them *before* it acts. Nothing is ever deleted — a claim that stops being true
is marked `superseded` and stays on disk, because how you used to think is part of the
knowledge.

```
knowledge/facts/projects/aurora/dec-sessions-not-jwt.md
```

```markdown
---
id: dec-sessions-not-jwt
title: Sessions are server-side; the cookie carries only an opaque id
type: decision
scope: project:aurora
subject: auth
relation: session-transport
status: active
source: human
---

Session state lives server-side. The cookie carries an opaque identifier and nothing else.

**Why:** revocation has to be immediate.
```

`subject` + `relation` are the identity of a claim. Two files sharing that pair assert the
same thing, so contradiction is a lookup rather than a guess — which matters, because
similarity search cannot do this job. A value flip `8000` → `8080` is *more* embedding-similar
to the original than an honest rephrase is.

---

## Quickstart

Ten minutes, one machine, one built-in agent. For a custom agent, MCP client, or hosted
chat, start with [Connect any agent to Regesto](docs/setup-other-agents.md).

**1. Install the engine.**

```bash
brew install prof18/tap/regesto
```

Or `go install github.com/prof18/regesto/cmd/regesto@latest` if you have Go, or grab a
binary from [releases](https://github.com/prof18/regesto/releases).

**2. Create your instance.** It is a folder of markdown; put it wherever you keep things
you would miss.

```bash
regesto init --dir ~/regesto-kb --examples
```

`regesto-kb` is just this guide's name for that folder, not a required one — name it
whatever you like, and read it back into every later example here as whatever you chose.

That writes the tree, a commented `config.toml`, both ignore files, this machine's
identity, the `bin/` shims, the agent adapters, `SCHEMA.md`, and a handful of example
facts to imitate. Drop `--examples` for an empty one.

**3. Choose which built-in agents Regesto should manage.**

`regesto init` detects installed Claude Code, Codex, and Hermes directories once and uses
that result to create the initial `integrations` list. Detection is only a starting guess:
open `~/regesto-kb/config.toml` and add or remove names until it matches what you want.

```toml
integrations = ["claude"]
```

You can list more than one:

```toml
integrations = ["claude", "codex", "hermes"]
```

**4. Preview and install the agent adapters.**

```bash
~/regesto-kb/bin/regesto-install --dry-run   # see what it would touch
~/regesto-kb/bin/regesto-install
# Equivalent once the engine can resolve this instance:
regesto --config ~/regesto-kb/config.toml install --dry-run --json
```

For each configured integration, install links Regesto's skills, updates its existing
instructions file, and registers a startup hook when the profile supports one. The dry run
shows every target before anything changes. Installation backs up files it edits and is
safe to run again.

Run `regesto --config ~/regesto-kb/config.toml doctor` to see detected integrations,
installed and pending artifacts, hook registration, memory availability, trust defaults,
and exact remediation without changing either the instance or host files.

**5. Open a new agent session in a project.** Claude's startup hook injects project
context automatically. Hermes does the same on the first turn after its reported YAML
step is complete. Codex has no startup hook; its installed instructions tell it to search
Regesto when recorded knowledge could answer the question.

**6. Record something.** Tell the agent "remember that release builds run on CI",
or run `/regesto-write`. The skill submits it through Regesto's validated write command, so
the fact lands in `knowledge/facts/` with its schema metadata, path, and timestamps assigned
by the tool, plus a `**Why:**` line.

That is the whole loop. Everything below is optional.

### Which setup path should I use?

| Your client | What to do |
|---|---|
| Claude Code, Codex, or Hermes | Put its built-in ID in `integrations`, then run `regesto install`. |
| Another local agent with skills or persistent instructions | Configure its actual paths with the `generic` profile. |
| A client that can launch local MCP servers | Point it at `regesto --config /absolute/path/config.toml mcp`; no integration entry is required for MCP alone. |
| A client with none of those capabilities | Export the conversation and run `regesto promote`. |

The complete procedures and copyable configuration are in
[Connect any agent to Regesto](docs/setup-other-agents.md). The
[integration matrix](docs/agent-integration.md) is the exact capability reference.

---

## Optional: the automatic half

```bash
cd ~/regesto-kb && regesto schedule install    # harvest every 15 min, cycle hourly (launchd, macOS)
```

LaunchAgents do not read shell startup files. At install time Regesto gives them a
deterministic `PATH`: the directories of the configured normaliser and notifier commands
it can resolve, followed by stable user, Homebrew and system locations. For a CLI installed
somewhere uncommon, add its directory with `[schedule].extra_path` and run `schedule install`
again. Keeping this at the scheduler boundary also covers interpreters such as `node`; using
absolute paths for `claude` or `codex` alone would not.

Commands other than `init` and the seven with a `bin/regesto-*` shim (`install`, `search`,
`context`, `config`, `index`, `project`, `write`) resolve their instance from the working directory,
so run them from inside `~/regesto-kb` — or pass `--config ~/regesto-kb/config.toml` from
anywhere else.

- **`regesto harvest`** diffs each agent's native memory against a per-machine snapshot and
  files anything new in `inbox/`. It needs no cooperation from the agent — anything it
  wrote down gets picked up whether or not it used the skill.
- **`regesto cycle`** normalises those captures into facts, reconciles contradictions,
  rebuilds `INDEX.md` and the topic pages, and commits.

The cycle stops at the first validation error and commits nothing, which is correct — half
a reconciliation is worse than none — but it means one malformed fact halts the whole thing
while facts keep being written. Since it runs unattended, it tells you: a notification when
it starts failing, another when it recovers, and one reminder a day in between. A working
pass says nothing. `regesto schedule status` prints when the last clean pass was, which is
the one thing a failing cycle cannot tell you itself — a job that never fires never reports
anything at all.

macOS uses `osascript`, Linux `notify-send`. To send them somewhere else — a phone, a chat
channel — point `[notify].command` at any program; it gets the title and message as its
last two arguments. `[notify].on = "off"` disables it.

---

## More than one machine

Get the folder onto both machines. That is the entire mechanism — no server, no backend, no
daemon of ours. Every machine holds a **full replica** and reads it locally, so an agent on a
laptop greps the laptop's copy, offline on a train if need be. No machine ever queries
another to consult knowledge.

**How you move it is your choice.** Syncthing, a hosted drive, `unison` on a timer — regesto
asks only that the transport replicate plain files and let you exclude two directories.
Nothing here is built against one vendor, and swapping later costs you nothing, because the
knowledge was never in the tool.

Each machine gets its own engine, its own name, and its own harvest job, because native
agent memory is local and nothing else can see it. One machine runs the lint pass, because
deciding what becomes a fact has to happen in one place or two machines mint competing
vocabulary for the same claim.

Two directories must **never** sync — `.state/`, whose per-machine baselines are what make
harvesting work, and `.git`. Both are already in the ignore files `regesto init` writes.

**[docs/setup-sync.md](docs/setup-sync.md)** — adding a machine, step by step, and what goes
wrong.

---

## Upgrading

Replacing the binary is half of it. `regesto init` also wrote files *into* your
knowledge base — the `bin/` shims, `adapters/`, `SCHEMA.md` — and a new engine does not
touch them on its own:

```bash
cd ~/regesto-kb
regesto upgrade --dry-run   # what would change
regesto upgrade
```

It knows the difference between a file it wrote and one you changed, because `init`
recorded a hash of each in `.regesto-manifest`. Files you edited are **left alone and
reported**, never overwritten; `--force` overwrites them and backs each one up first. A
file it cannot attribute — an instance older than the manifest — is treated as yours.

A file a release **retires** is removed, but only where it is byte for byte what the engine
recorded writing. Edit it and it becomes yours: kept, reported, and no longer tracked.
Anything with no recorded hash is never touched at all.

Regesto does not carry automatic migrations for old instance formats. For a pre-0.4
instance, manually rename `agents` to `integrations`, move location overrides into
`[integrations.<id>]`, then run `regesto install`. After confirming the new links and one
current harvest baseline, old `.state/skills` and per-integration `last/` blob directories
can be removed.

Then it finishes the job for each declared capability: re-renders and relinks configured
skills, drops retired owned skills, refreshes configured instructions and hook
registrations, and repoints scheduled jobs if they name an engine that is no longer the
one serving this instance. A skill added or withdrawn by a release reaches integrations
with skills targets from this one command — nothing else to run.

## Uninstalling

There is no single command, because `regesto-install` only ever *adds* to files it does not
own outright. Reverse it by hand:

```bash
cd ~/regesto-kb && regesto schedule uninstall   # stops the launchd jobs, if you ran `schedule install`
```

Then inspect `regesto install --dry-run --json` for the declared host targets. Remove the
Regesto hook registration and any separate host allowlist entry, the block between the
`regesto:section:start` / `:end` markers in each instructions file, and the Regesto skill
links in each skills directory. Preserve unrelated settings and links. Delete the instance
directory itself (`~/regesto-kb` by default) to remove the knowledge base.

## Commands

| | |
|---|---|
| `regesto search` | query facts by subject, relation, scope or free text |
| `regesto context` | the payload the `SessionStart` hook injects |
| `regesto install` | plan or apply skills, instructions and hook registration |
| `regesto hook <protocol>` | translate one Claude/Hermes hook payload with exact host framing |
| `regesto doctor` | read-only integration, artifact, capability, memory, and trust diagnostics |
| `regesto mcp` | serve local resources and validated tools over MCP stdio; no network listener |
| `regesto index` | rebuild `INDEX.md` and `knowledge/topics/` |
| `regesto lint` | validate against `SCHEMA.md`, reconcile contradictions |
| `regesto harvest` | capture native-memory writes into `inbox/` |
| `regesto normalize` | turn captures into canonical facts |
| `regesto cycle` | the whole downstream pass, then commit |
| `regesto promote` | a chat export → facts → archive |
| `regesto project` | the canonical project name for a directory |
| `regesto config` | the resolved instance config |
| `regesto write` | validate and atomically record one fact from structured input |
| `regesto init` | scaffold a new instance |
| `regesto upgrade` | refresh an instance's engine-owned files after the engine changed |
| `regesto schedule` | run harvest and cycle automatically |
| `regesto version` | which engine this is |

`search`, `context`, `project`, `config`, and `doctor` accept `--json` for stable
machine-facing results. `write --source … --json-input --json` is the validated
structured write boundary; its default output remains a concise relative path.

---

## Agents

| Agent | Consultation | Setup |
|---|---|---|
| Claude Code | deterministic when the declared startup hook is current | [docs/setup-claude-code.md](docs/setup-claude-code.md) |
| Hermes Agent | deterministic on the first turn when registration and allowlist are current | [docs/setup-hermes.md](docs/setup-hermes.md) |
| Codex CLI | skills + instructions | [docs/setup-codex.md](docs/setup-codex.md) |
| Other local agents | portable skills and/or instructions configured with their actual paths | [docs/setup-other-agents.md](docs/setup-other-agents.md#custom-local-integration) |
| MCP clients | local search, reads, project resolution, and validated writes; consultation remains client-driven | [docs/setup-other-agents.md](docs/setup-other-agents.md#mcp-client) |
| Hosted clients with no local interface | export a conversation and promote it manually | [docs/setup-other-agents.md](docs/setup-other-agents.md#no-local-integration-surface) |

The declared Claude `SessionStart` and Hermes `pre_llm_call` protocols inject context
before the model can respond, so consultation is guaranteed when their registrations are
current. The built-in Codex profile, and generic integrations configured with skills and
instructions targets, use those advisory paths; consultation is likely rather than
enforced. That difference is real and this project would rather state it than paper over
it.

The canonical [agent integration matrix](docs/agent-integration.md) is mechanically checked
against the profile metadata. It also gives a standalone generic-host recipe and separates
automated protocol/installer evidence from dated live-host validation.

Adding a reusable agent preset is normally a declarative profile under
`adapters/profiles/`; a one-off host can be configured entirely in `config.toml`. Tested
hooks or host-specific skill optimizations add protocol or variant files under
`adapters/`, but nothing changes in `knowledge/`.

---

## Read next

- **[SCHEMA.md](SCHEMA.md)** — the contract. What a fact is, how supersession works, what
  to record and what not to. Read this before writing facts by hand.
- **[DESIGN.md](DESIGN.md)** — why it is built this way, including the research verdicts
  that saved a week.
- **[docs/setup-sync.md](docs/setup-sync.md)** — running across several machines: the order
  to do it in, what must never sync, and the failure modes.
- **[CONTRIBUTING.md](CONTRIBUTING.md)** — one rule that matters: schema changes need a
  migration note.

## Requirements

Go 1.26 to build. Hook payloads and registration use the Go engine and require no `jq`.
macOS is required for `regesto schedule` (launchd); the rest is portable, and the jobs are
two commands you can run from any scheduler.

## License

MIT.
