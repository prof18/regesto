# Regesto

**A knowledge base your agents consult — not a memory they carry.**

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
go install github.com/prof18/regesto/cmd/regesto@latest
```

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

Multi-machine is a file-sync client over the folder and nothing else — no server, no
backend, no daemon of ours. Every machine holds a full replica and reads it locally.
[docs/setup-sync.md](docs/setup-sync.md) is the whole of it, including the two directories
that must never sync.

---

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
| `regesto schedule` | run harvest and cycle automatically |

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
- **[CONTRIBUTING.md](CONTRIBUTING.md)** — one rule that matters: schema changes need a
  migration note.

## Requirements

Go 1.26 to build. `jq` for hook registration. macOS for `regesto schedule` (launchd); the
rest is portable, and the jobs are two commands you can run from any scheduler.

## License

MIT.
