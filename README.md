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

Ten minutes, one machine, one agent.

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

That writes the tree, a commented `config.toml`, both ignore files, this machine's
identity, the `bin/` shims, the agent adapters, `SCHEMA.md`, and a handful of example
facts to imitate. Drop `--examples` for an empty one.

**3. Install the adapters into your agents.**

```bash
~/regesto-kb/bin/regesto-install --dry-run   # see what it would touch
~/regesto-kb/bin/regesto-install
```

Skills are symlinked into each agent, the `SessionStart` hook is registered (Claude Code),
and a short pointer section is appended to your instructions file. It backs up anything it
edits and is safe to re-run.

**4. Open a session in a project.** Its facts are already in context — you did not ask for
them, and the agent did not have to decide to look.

**5. Record something.** Tell the agent "remember that migrations here are forward-only",
or run `/regesto-write`. The fact lands in `knowledge/facts/`, schema-correct, with a
`**Why:**` line.

That is the whole loop. Everything below is optional.

---

## Optional: the automatic half

```bash
regesto schedule install    # harvest every 15 minutes, lint hourly (launchd, macOS)
```

- **`regesto harvest`** diffs each agent's native memory against a per-machine snapshot and
  files anything new in `inbox/`. It needs no cooperation from the agent — anything it
  wrote down gets picked up whether or not it used the skill.
- **`regesto cycle`** normalises those captures into facts, reconciles contradictions,
  rebuilds `INDEX.md` and the topic pages, and commits.

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

Then it finishes the job: re-renders the skills, relinks them into every agent, drops the
ones that were retired, refreshes the instructions section and the hook, and repoints the
scheduled jobs if they name an engine that is no longer the one serving this instance. A
skill added or withdrawn by a release reaches your agents from this one command — nothing
else to run.

## Commands

| | |
|---|---|
| `regesto search` | query facts by subject, relation, scope or free text |
| `regesto context` | the payload the `SessionStart` hook injects |
| `regesto index` | rebuild `INDEX.md` and `knowledge/topics/` |
| `regesto lint` | validate against `SCHEMA.md`, reconcile contradictions |
| `regesto harvest` | capture native-memory writes into `inbox/` |
| `regesto normalize` | turn captures into canonical facts |
| `regesto cycle` | the whole downstream pass, then commit |
| `regesto promote` | a chat export → facts → archive |
| `regesto project` | the canonical project name for a directory |
| `regesto config` | the resolved instance config |
| `regesto init` | scaffold a new instance |
| `regesto upgrade` | refresh an instance's engine-owned files after the engine changed |
| `regesto schedule` | run harvest and cycle automatically |
| `regesto version` | which engine this is |

---

## Agents

| Agent | Consultation | Setup |
|---|---|---|
| Claude Code | **hook-enforced** — deterministic | [docs/setup-claude-code.md](docs/setup-claude-code.md) |
| Codex CLI | skills + instructions | [docs/setup-codex.md](docs/setup-codex.md) |
| Others | skills + instructions | [docs/setup-other-agents.md](docs/setup-other-agents.md) |

Only Claude Code exposes a hook that can inject context before the first prompt, so only
there is consultation *guaranteed*. Everywhere else it is likely. That difference is real
and this project would rather state it than paper over it.

Adding an agent is a pull request that touches `internal/adapters` and `adapters/`, and
nothing in `knowledge/`.

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

Go 1.26 to build. `jq` for hook registration. macOS for `regesto schedule` (launchd); the
rest is portable, and the jobs are two commands you can run from any scheduler.

## License

MIT.
