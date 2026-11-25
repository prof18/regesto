# Regesto — design

Why this is built the way it is. The contract itself is `SCHEMA.md`; this document is the
reasoning behind it, distilled from the architecture decision the tool grew out of.

Section numbers are stable, because code comments cite them.

---

## 1. What this is

**Regesto is a knowledge base that agents consult before they act. It is not a memory
system: it keeps the history that agent memory files lose.**

A native agent memory file is a *working-set cache*. Hosts may bound, rewrite, or prune
it, and none expose the append-only history contract Regesto requires, so it is lossy by
design. That suits a cache, but knowledge has to survive.

This knowledge base accumulates instead. Nothing is ever deleted, and superseded claims
stay on disk marked `status: superseded`, so the history of a decision survives.

This follows [Karpathy's wiki gist](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f)
directly. Its three layers map one to one:

| Karpathy | Here | Property |
|---|---|---|
| Raw sources | `archive/` | Immutable. Never edited, only appended. |
| The wiki | `knowledge/` | Agent-maintained. Facts plus linked topic pages. **The product.** |
| The schema | `SCHEMA.md` | Hand-written by you. Defines the contract. |

And its three operations:

| Karpathy | Here | Who runs it |
|---|---|---|
| Ingest | `regesto harvest`, `regesto promote` | A job on every machine |
| Query | **Consult** — before acting | The declared hook when available; otherwise the agent follows instructions |
| Lint | `regesto cycle`: normalize, supersede, rebuild | One machine |

Everything else serves those three operations.

---

## 2. The knowledge base

```
<instance>/
├── SCHEMA.md                 # the contract
├── INDEX.md                  # ★ GENERATED manifest — the agent's entry point
├── knowledge/
│   ├── facts/                #   ★ THE PRODUCT. One claim per file. Nothing deleted.
│   │   ├── global/
│   │   └── projects/<name>/
│   └── topics/               #   GENERATED entity pages that wikilink the facts
├── inbox/<agent>@<machine>/  # harvested agent writes, pre-merge
├── adapters/                 # per-provider glue: skills, hooks, instructions
├── archive/                  # raw sources. Immutable. Grepped, never loaded.
└── .state/                   # last-seen snapshots, per machine. Never synced.
```

### 2.1 Atomic facts

One claim per file. The frontmatter lets lint work mechanically instead of guessing.
`SCHEMA.md` lists every field.

The `subject` and `relation` fields together identify a claim. Two files that share that
pair assert the same thing, so finding a contradiction is a lookup: the newer `modified`
wins, and the older claim flips to `superseded` and **stays on disk**.

Reconciliation uses no embedding threshold or similarity score, because embeddings provably
cannot detect this case. Changing a value from `8000` to `8080` produces text that is
*more* embedding-similar to the original than a genuine rephrase is. Only a structural key
tells the two apart.

The `source` field gives provider independence in practice: you can always see which agent
asserted a claim, and revoke all of one provider's contributions if it proves noisy.

### 2.2 Topic pages

Atomic facts are excellent for supersession and poor at *"tell me everything about this
project's architecture."* Karpathy's wiki answers that with entity pages and
cross-references.

`knowledge/topics/` holds **generated** pages that wikilink the atomic facts they
summarise. Lint rebuilds them, so they never drift from the facts, and they make the base
browsable for a human in any markdown vault.

### 2.3 INDEX.md

`INDEX.md` is a generated manifest. It lists what exists, organised by scope and subject,
with one-line descriptions and file paths, plus the controlled vocabulary of
`(subject, relation)` pairs in use. It is small enough to inject whole into a session.

It makes consultation cheap: an agent knows what exists before it searches.

---

## 3. Consultation, and how to actually enforce it

"The agent consults the knowledge base before acting" is easy to write and hard to
guarantee, because **instructions get ignored**. Without explicit guidance a model
acknowledges information and never calls the retrieval tool, so the tools do nothing
without instructions that tell the model to use them. Even with guidance, compliance is
probabilistic.

For a host with a tested pre-model hook, Regesto can do better than instructions. The
profile declares a protocol and registrar; the protocol adapter translates the host's
payload and emits host-valid context framing. The built-in deterministic paths are:

| Hook | Fires | Output behaviour |
|---|---|---|
| Antigravity `PreInvocation` | before every model call; injection on the first call of a conversation | JSON `injectSteps` ephemeral message added before the first model turn |
| Claude `SessionStart` | startup, resume, clear, compact, fork | stdout added as context before the first prompt |
| Copilot `SessionStart` | first prompt of a new session | JSON `additionalContext` added before the first turn |
| Hermes `pre_llm_call` | first call for a session | JSON `context` added before the first model turn |

That makes retrieval **deterministic**. The hook does the lookup; the model does not get a
choice about whether to consult.

- `SessionStart`, the first Antigravity `PreInvocation`, and the first Hermes `pre_llm_call`
  inject the index plus the facts scoped to this repo. This runs once per session and costs
  little, and the agent starts already knowing what exists. It is what `regesto context`
  prints before protocol framing.
- `UserPromptSubmit` could run a *cheap* keyword match against the index and inject only the
  matching fact ids and one-line summaries, rather than a full search on every prompt. It is
  not implemented: build it once you have seen what you actually query.

With a hook, **the hook retrieves and the agent only reads**. On the surface where most of
the work happens, the agent no longer has to comply with anything for consultation to occur.

Integrations without a declared hook mechanism use declared skills and instructions where
those targets exist; otherwise they need configured generic targets, local MCP, or manual
promotion. Those paths are weaker. Consultation is deterministic only where Regesto has
successfully registered a hook; recognizing the host's product name does not make it so.
[docs/agent-integration.md](docs/agent-integration.md) documents the profile-driven matrix
and evidence boundary.

**Scope consultation narrowly.** "Consult before doing anything" is too broad. Firing
retrieval on every trivial prompt is expensive and trains you to ignore the noise. Scope it
to tasks touching a project, a decision, or a convention. An agent given too broad an
instruction tends to ignore the rule entirely.

---

## 3.5 Skills: the portable half of the adapter

Where a host exposes the right event, hooks guarantee consultation. Skills are the
complement and the most portable piece of the design: the common files follow the
[agentskills.io](https://agentskills.io) open standard, and every built-in integration
renders them byte for byte. A profile may still declare an append-only variant for a
tested host optimization; no built-in profile does.

A skill is on-demand instructions in `<dir>/skills/<name>/SKILL.md` with YAML frontmatter.
Only its `description` sits in context; the body loads when invoked. That fits the central
rule of this design — keep the always-loaded layer tiny — better than any other mechanism.

Regesto deliberately uses the standard's host-neutral subset: `name`, `description`,
`license`, `compatibility`, and string-valued `metadata`. The experimental
`allowed-tools` field is excluded because its command grammar is host-specific; a tested
host optimization belongs in that integration's declared variant instead. The installer
validates a conservative YAML string subset (quoted or safe plain scalars, plus ordinary
literal/folded blocks); unusual YAML encodings should be normalized during upgrade.

**`regesto-write`** teaches how to record a fact correctly: id naming, how to choose
`subject`/`relation`, and when to supersede rather than create. It submits those semantic
fields to the validated writer; the engine owns provenance, timestamps, schema metadata,
path selection, atomic publication, and reconciliation preview. The skill turns `SCHEMA.md`
into an executable procedure without asking an agent to hand-author authority fields.

Its `description` is worded as **triggers, not a summary** — "use immediately when the user
asks to remember something, a decision has been agreed, a preference stated, a correction
repeated, a gotcha discovered". The description is the only part of a skill that is always
in context, and the model matches against it when deciding to invoke the skill unprompted.
A bland description such as "records facts" means the skill fires only when you ask. The
trigger wording is what makes §5's fast path work.

The same skill is the **manual store procedure**. Invoke it explicitly through the host's
skill UI or ask naturally ("remember this"). The first trigger is absolute: the agent
never replies "noted" without writing the file.

**`regesto-search`** runs a deep query. The body tells the host to call the stable search
CLI with the user's actual query. An earlier Claude-only variant pre-ran that search through
Claude Code's dynamic context injection; it was dropped because hosts sharing a skills
directory with Claude received syntax they never execute, and an invocation without
arguments dumped the whole index.

**`regesto-promote`** turns an exported chat into durable facts and then archives the
export. It is for any conversation host that cannot participate automatically. Its portable
description and body make explicit that it is a user-requested batch operation with side
effects.

### Where skills fit, and where they don't

| Mechanism | Guarantees? | Portable? | Job |
|---|---|---|---|
| Hook | **yes** — deterministic | host capability | make sure consultation *happens* |
| Skill | no — the model decides | **yes** | teach the *procedure*; run deep retrieval |
| Pointer in the memory file | no | yes | last-resort breadcrumb |

Skills have three limits:

- **Skill content persists but is never re-read.** Once invoked, the rendered body stays in
  context for the session and is not reloaded on later turns. A search invoked at turn 1
  has stale results by turn 20, so a per-prompt hook is still useful.
- **Descriptions drive discovery.** Only portable `name` and `description` metadata is
  required at startup, so the description leads with the trigger and concrete use case.
- **Dynamic pre-execution skill injection is a profile variant, and no built-in uses it.**
  Variants live under `adapters/variants/`; render tests prevent cross-profile leakage, and
  a shared skills directory refuses integrations whose variants differ.

---

## 4. Agent memory files are pointers, not digests

An earlier revision of this design tried to project a *content digest* of the knowledge
base into each agent's native memory file, fighting each vendor's size cap. Under a
consult-first model that is unnecessary. If the agent reads the knowledge base, the memory
file does not need to contain the knowledge. It needs to say **where the knowledge is and
how to search it** — which is what `adapters/instructions/regesto-section.md` installs.

Each consequence simplifies the design:

- **Size caps stop mattering.** The tightest vendor cap was the binding constraint in the
  digest design. A pointer fits in a few hundred characters, so the cap no longer binds.
- **The hard part disappears.** Generating a mechanical manifest replaces summarisation
  into a byte budget, which is both more tractable and more reliable.
- **Native memory keeps its real job** of holding short-lived working-set notes. Harvest
  anything durable out of it, and let the rest expire.

---

## 5. Ingest: two write paths

**Direct writes — the fast path.** When a decision is agreed mid-session, a preference is
stated, or a correction lands for the second time, the agent writes the fact straight into
`knowledge/facts/` via the `regesto-write` skill. The fact is schema-correct, the write
works on any machine, and it does not wait for lint. The agent writes **unprompted**: the
triggers live in `SCHEMA.md`, in the always-loaded instructions, and in the skill's own
description. You should never have to say "write that down."

Unprompted writing is a judgement call — no hook can force it — so compliance is
probabilistic, like all instructions. That is acceptable *because the second path exists*:

**Harvest — the safety net.** Agents write to their native memory automatically. That
capture costs nothing and already happens; it is the one hard problem the vendors already
solved for you. Anything the agent noticed but did not record arrives this way one cycle
later.

Harvest has one caveat. Native memory is a cache the agent actively prunes (§1), so a note
written *and pruned* between two harvest snapshots never appears in any diff and is gone.
**The harvest interval is the loss window**: run it every few minutes, not hourly, and
treat the direct write as the real guarantee for anything that matters. If the agent did
both, reconcile collapses the duplicate, so the two paths can overlap safely.

Collection needs no cooperation from the agent. Harvest keeps a snapshot in `.state/` of
each vendor file as last seen, and the new capture is the current file minus that
snapshot.

**Harvest is a per-machine job.** Native memory lives in each machine's home directory,
outside the synced folder, so each machine harvests its own agents into
`inbox/<agent>@<machine>/` inside the synced folder. That is also why `.state/` is
per-machine and unsynced: each machine diffs against its own baseline. If `.state/` is ever
shared, machines overwrite each other's baselines and captures are lost silently.

**Order matters: harvest before writing.** Nothing in this pipeline rewrites vendor files —
memory files are pointers (§4). The rule exists for the one exception, installing or
re-asserting the pointer block: if that runs before the diff, everything written since the
last snapshot is destroyed silently.

---

## 6. Lint

The periodic pass is `regesto cycle`.

1. **Collect** — pick up new captures from `inbox/`
2. **Normalize** — convert raw captures into canonical frontmatter; assign `id`, `subject`,
   `relation`, `source`
3. **Validate** — schema-lint all of `knowledge/facts/`, including facts written directly,
   which never pass through `inbox/`
4. **Reconcile** — when two `active` or `proposed` claims share a `(subject, relation)`, the
   newer `modified` wins, but never across the boundary between agent and human sources. The
   older claim flips to `status: superseded` with a backlink. **Never delete.**
5. **Rebuild** — regenerate `knowledge/topics/` and `INDEX.md`
6. **Resolve** — settle sync-conflict copies
7. **Commit** — commit the pass to git

Normalisation uses the configured source policy (§8), never an inferred channel label. A
private single-user surface may be explicitly trusted, while unknown, unattended, or
unconfigured sources quarantine by default: they stay raw in `inbox/`, invisible to search
and hooks, until a human promotes them.

**Lint runs on one machine.** Capture happens everywhere, but deciding what becomes a fact
happens in one place; otherwise two machines mint competing vocabulary for the same claim.

---

## 7. The agents

| Agent | Native store | Scope | Consultation |
|---|---|---|---|
| Antigravity CLI | JSONL transcripts + SQLite session store (not harvestable) | global | deterministic when hook registration is current |
| Claude Code | per-project memory directory | per git repo | deterministic when startup hook is current |
| Codex CLI | global memory directory | global | instructions + skills |
| Copilot CLI | SQLite session store (not harvestable) | global | deterministic when startup hook is current |
| Hermes | declared markdown memory directory | global | deterministic when hook and allowlist are current |
| Host without local files or MCP | vendor-internal | — | manual promotion only |

Of the built-in profiles, Claude Code's is the only per-project store, and its directory
name derives from the absolute repo path, so the same project fragments across clones and
across machines. The adapter normalises all of them onto one canonical project name,
derived from the git remote's basename, with a hand-kept map in the instance config for
remote-less repos and for basename collisions.

Three vendors independently converging on a markdown memory file is good evidence the
format is right, but each of those stores stays under its vendor's control.

**Adding another agent** normally requires only a configured integration using the
portable generic profile: its pointer block, instruction, optional hook, paths, and trust
default are data rather than product-specific code. A pull request is reserved for a new
optimised built-in profile whose platform semantics cannot be expressed by the generic
contract. Nothing in `knowledge/` changes.

---

## 8. Phones

An obvious design is a public HTTPS endpoint so a phone app's cloud can reach your
machine. That means a permanently listening service holding your entire personal knowledge
base, reachable from the internet — and it still leaves some vendors out.

**A chat-surface agent running on your own machine replaces it.** The machine makes
outbound connections only: no tunnel, no open port, no public attack surface.

**Trust attaches to a configured source surface. An agent name or an asserted channel
status does not grant it.** A supervised integration may trust its own source; an
unattended or shared surface should use a distinct integration ID and quarantine default.
Exact source policies can approve or quarantine a known source, while unknown and
malformed sources quarantine. A private-channel claim is not self-authenticating: a human
records the policy.

Quarantined captures are never normalised, remain raw in `inbox/`, and stay invisible to
search, hooks, and the index until a human promotes them. A planted claim that reached a
session's context before review would defeat the quarantine, so quarantined must mean
*invisible*, not merely flagged.

An always-on agent also means no quiet window for lint, which is why writes are atomic.

---

## 9. Sync and transport

**Regesto uses a sync client for transport and git for history.** Git-as-transport fails
open: it needs a pull before every session and a push after, and missing one means the
agent confidently reads stale knowledge. Hooks do not rescue it — an end-of-session hook
cannot block, does not fire on a crash or a closed lid, and its output reaches no one.

A file-sync client fails closed: in the worst case you get a visible conflict file, which
lint reconciles.

### 9.0 How the machines fit together

There is no server and no backend. The knowledge base is a folder, and sync keeps a **full
replica on every machine**. Reading is always a local file read — an agent on a laptop
greps the laptop's copy, offline if need be. No machine ever queries another to consult
knowledge.

Symlinks into the agents live *outside* the synced folder and point into it, so the sync
client only ever sees real files.

**The infrastructure comes in tiers, and you pick the one you need:**

| Tier | What you run | What you get |
|---|---|---|
| One machine | nothing | the whole design, minus replication |
| Several machines | any file-sync client | a replica everywhere, offline reads |
| Plus an always-on node | the same client on a NAS or small server | a third replica that is always up, and a backup target |

On the single-machine tier, consultation, writing and lint all work. Sync only adds
replication, and nothing else depends on it.

### 9.1 Git lives on one machine

The repository is intentionally excluded from sync, and exists on the machine that runs
lint. Syncing `.git` is unenforceable: any agent session on a second machine runs
`git status` and becomes a second writer. The markdown holds the knowledge; git adds
convenience and offsite history.

A remote holds your accumulated personal knowledge in plaintext. Choose it deliberately —
a private hosted repo, a bare repo on a machine you own, or an encrypted remote.

---

## 10. Research verdicts

**Validated:**

- Markdown in git is the source of truth, and every index is disposable.
- A workstation is the active host, and a NAS provides durability.
- Some phone apps structurally cannot reach a custom MCP server. Others call connectors
  from the vendor's cloud, so they need a public endpoint.
- A graph database is the wrong place to start.

**Memory-palace vector products are rejected.** Independent reproduction of the
best-marketed one confirms the "memory palace" architecture it sells as its differentiator
*makes retrieval worse*, and its headline recall number is essentially unmodified
off-the-shelf vector search. They also store knowledge as opaque vectors rather than
readable markdown, which rules them out on its own.

**Regesto has no vector database yet.** With structured frontmatter, scope and subject
filters and a generated index, grep is strong for a long time. Revisit it once you can name
real queries grep failed.

**Avoid pointing an agent's auto-memory directory at the knowledge base.** It points at
one directory, collapsing per-project isolation. The vendor issue requesting repo-relative
paths is open; until it ships, harvest-by-diff is the better mechanism, and it needs no
cooperation from the vendor at all.

**Cross-agent message relays are orthogonal.** A relay could later carry requests such as
"the laptop asks the workstation to run lint", but it stores no knowledge, so it is not a
memory system.

---

## 11. Build order

Build consultation before automation.

- **Foundation** — write `SCHEMA.md` for your own vocabulary if the shipped one does not
  fit. The later phases build on it.
- **Consultation** — build index generation, search, the `SessionStart` hook, and the
  skills. **This alone makes the knowledge base real**, before any automation exists.
- **The lint loop** — harvest by diff, normalize, reconcile, rebuild, commit.
- **Phones** — add them if you want them.

A knowledge base that nobody writes to automatically is still useful, because you can write
to it by hand. A knowledge base that agents cannot read is useless. So if you stop after
consultation, what you have already works.

Code comments cite the phases of the original build log as `PLAN <phase>.<step>`; that
document is a private instance's history and is not published. The mapping is: **0**
foundation, **1** consultation, **2** the lint loop, **3** phones, **4** this extraction.

---

## 12. Honest trade-offs

**The fully automatic path is not zero-infrastructure.** Deterministic consultation needs
a supported hook, and automatic capture needs a periodic job. When a layer breaks, the one
below it still works. If the hook breaks, you can still grep. If lint breaks, facts still
accumulate in `inbox/`. Nothing is ever *only* in a generated artifact.

**Generated pages and the index can drift** if lint fails silently. They are regenerable
from `facts/`, so treat them as caches and rebuild rather than repair.

**Consultation is guaranteed only where a hook exists.** Elsewhere it relies on
instructions, which are probabilistic, so the two are not equivalent.

**Hook latency is real.** A per-session hook is free; a per-prompt hook runs on every
message. Keep it to a cheap index lookup, or you will feel it constantly and disable it.
The engine is a single static binary rather than a script for this reason: a cold start of
milliseconds is what makes the hook safe to run.

**You own a format now.** `SCHEMA.md` has to stay disciplined, or the knowledge base rots
into free-form notes with inconsistent frontmatter and lint stops working.

**Accumulation is unbounded.** Never deleting is correct for knowledge but means growth is
monotonic. `status: superseded` keeps it navigable; revisit if `facts/` reaches five
figures.

---

## 13. Deliberately not doing

- No vector or graph database until a real query fails against grep.
- No memory engine. `SCHEMA.md` and the lint pass are the product.
- No content digests fighting vendor size caps.
- No deletion. Supersede instead.
- No public inbound endpoint unless a specific app earns it.
