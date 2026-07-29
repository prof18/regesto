# Regesto — design

Why this is built the way it is. The contract itself is `SCHEMA.md`; this document is the
reasoning behind it, distilled from the architecture decision the tool grew out of.

Section numbers are stable, because code comments cite them.

---

## 1. What this is

**This is not a memory system. It is a knowledge base that agents consult.**

The distinction is load-bearing. A native agent memory file — Claude Code's, Codex's,
Hermes's — is a *working-set cache*. It is bounded (a couple of hundred lines, or a hard
character cap), and the agent is actively instructed to prune stale entries to stay under
it. It is lossy **by design**. That is correct behaviour for a cache and wrong behaviour
for knowledge.

A knowledge base is the opposite: it accumulates, nothing is ever deleted, and superseded
claims stay on disk marked `status: superseded` so the history of a decision survives.

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
| Query | **Consult** — before acting | The agent, enforced by a hook |
| Lint | `regesto cycle`: normalize, supersede, rebuild | One machine |

Everything else is in service of those three.

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

One claim per file. The frontmatter is what makes lint mechanical rather than a guess —
the full field list is in `SCHEMA.md`.

`subject` + `relation` together are the identity of a claim. Two files sharing that pair
assert the same thing, so contradiction becomes a lookup: newer `modified` wins, older
flips to `superseded` and **stays on disk**. No embedding threshold, no similarity guess —
which matters, because embeddings provably cannot do this. A value flip `8000`→`8080` is
*more* embedding-similar to the original than a genuine rephrase is. The fix has to be
structural.

`source` is what buys provider independence in practice: you can always see which agent
asserted what, and revoke a provider's contributions wholesale if it proves noisy.

### 2.2 Topic pages

Atomic facts are excellent for supersession and terrible at *"tell me everything about
this project's architecture."* Karpathy's wiki has entity pages and cross-references, and
you want them too.

`knowledge/topics/` holds **generated** pages that wikilink the atomic facts they
summarise. They are rebuilt by lint, so they never drift from the facts. They are also
the layer that makes the whole thing browsable for a human in any markdown vault — graph
view, backlinks, the lot. The same artifact serves the agent and you.

### 2.3 INDEX.md

A generated manifest: what exists, organised by scope and subject, with one-line
descriptions and file paths, plus the controlled vocabulary of `(subject, relation)` pairs
in use. Small enough to inject wholesale into a session.

This is what makes consultation cheap. Without it an agent greps blind; with it, it knows
what is there before it looks.

---

## 3. Consultation, and how to actually enforce it

This is the crux. "The agent consults the knowledge base before acting" is easy to write
and hard to guarantee, because **instructions get ignored**. Without explicit guidance a
model acknowledges information and never calls the retrieval tool; the instructions are
the product, and the tools alone are not enough. Even with guidance, compliance is
probabilistic.

For Claude Code you can do better than instructions. Two hooks inject their stdout
directly into the model's context:

| Hook | Fires | Output behaviour |
|---|---|---|
| `SessionStart` | startup, resume, clear, compact, fork | stdout added as context before the first prompt |
| `UserPromptSubmit` | every prompt, before the model processes it | stdout added as context alongside the prompt |

That makes retrieval **deterministic**. The hook does the lookup; the model does not get a
choice about whether to consult.

- `SessionStart` → inject the index plus the facts scoped to this repo. Cheap, once per
  session, and the agent starts already knowing what exists. This is what `regesto
  context` prints.
- `UserPromptSubmit` → a *cheap* keyword match against the index, injecting only matching
  fact ids and one-line summaries. Not a full search on every keystroke. Not implemented:
  build it once you have seen what you actually query.

Note the inversion: **the hook retrieves, the agent reads.** That sidesteps the compliance
problem entirely for the surface where most of the work happens.

Agents without a hook mechanism fall back to instructions plus skills. Weaker, and worth
being honest about: consultation is *guaranteed* on Claude Code and *likely* elsewhere.

**One caveat on scope.** "Consult before doing anything" is too broad. Firing retrieval on
every trivial prompt is expensive and trains you to ignore the noise. Scope it to tasks
touching a project, a decision, or a convention. Over-instructing is how you get an agent
that ignores the rule entirely.

---

## 3.5 Skills: the portable half of the adapter

Hooks guarantee consultation but are Claude Code only. Skills are the complement, and they
are the most provider-portable piece of the design: Claude Code skills follow the
[agentskills.io](https://agentskills.io) open standard, Codex discovers the same files,
and Hermes documents compatibility with it. **One `SKILL.md` can serve all three.**

A skill is on-demand instructions in `<dir>/skills/<name>/SKILL.md` with YAML frontmatter.
Only its `description` sits in context; the body loads when invoked. That fits the central
rule of this design — keep the always-loaded layer tiny — better than any other mechanism.

**`regesto-write`** — how to record a fact correctly: the frontmatter template, id naming,
how to choose `subject`/`relation`, when to supersede rather than create, where the file
goes by scope. `SCHEMA.md` rendered as an executable procedure.

Its `description` is worded as **triggers, not a summary** — "use immediately when the user
asks to remember something, a decision has been agreed, a preference stated, a correction
repeated, a gotcha discovered" — because the description is the only part of a skill that
is always in context, and it is what the model matches against when deciding to invoke
unprompted. A bland "records facts" means the skill only ever fires when you ask; trigger
wording is the mechanism behind §5's fast path.

The same skill is the **manual store command**. Skills are user-invocable as slash
commands, so `/regesto-write <statement>` records that statement on the spot, and
`/regesto-write` with nothing after it records whatever was just agreed. Natural language
works too ("remember this") — it is the first trigger in the list, and it is absolute: the
agent never replies "noted" without writing the file.

**`regesto-search`** — deep query. On Claude Code it uses `` !`command` `` dynamic context
injection, which runs the search *before* the skill body reaches the model and substitutes
the output. **The skill runs the search itself**; the model receives results rather than an
instruction to go looking — the same inversion the hooks give you, in a portable package.

**`regesto-promote`** — chat export → durable facts → archive, for app conversations that
cannot participate automatically. `disable-model-invocation: true`: it is a deliberate
batch operation with side effects.

### Where skills fit, and where they don't

| Mechanism | Guarantees? | Portable? | Job |
|---|---|---|---|
| Hook | **yes** — deterministic | Claude Code only | make sure consultation *happens* |
| Skill | no — the model decides | **yes** | teach the *procedure*; run deep retrieval |
| Pointer in the memory file | no | yes | last-resort breadcrumb |

**Hooks for enforcement, skills for procedure.** They are not substitutes.

Three limits worth knowing:

- **Skill content persists but is never re-read.** Once invoked, the rendered body stays in
  context for the session and is not reloaded on later turns. A search invoked at turn 1
  has stale results by turn 20 — which is why a per-prompt hook still earns its place.
- **`description` + `when_to_use` are truncated** in the skill listing, around 1,536
  characters. Lead with the key use case.
- **Dynamic context injection is the non-portable part.** The `` !`command` `` preamble is a
  Claude Code feature, not part of the agentskills.io core standard. Skills themselves do
  port — Codex discovers all three and lists their descriptions — but Codex receives the
  `` !`command` `` line literally and unexecuted, and does not expand `$ARGUMENTS` either.
  So `regesto-search`'s body opens by telling the agent which of the two cases it is in
  and, in the literal case, to run the command itself with the query substituted. Weaker
  than hook-grade enforcement, but honest, and retrieval still happens on both.

---

## 4. Agent memory files are pointers, not digests

An earlier revision of this design tried to project a *content digest* of the knowledge
base into each agent's native memory file, fighting each vendor's size cap. Under a
consult-first model that is unnecessary. If the agent reads the knowledge base, the memory
file does not need to contain the knowledge. It needs to say **where the knowledge is and
how to search it** — which is what `adapters/instructions/regesto-section.md` installs.

Consequences, all simplifications:

- **Size caps stop mattering.** The tightest vendor cap was the binding constraint in the
  digest design. A pointer fits in a few hundred characters. The constraint evaporates.
- **The hard part disappears.** Generating a mechanical manifest replaces summarisation
  into a byte budget. Far more tractable, far more reliable.
- **Native memory keeps its real job**: short-lived working-set notes. Harvest anything
  durable out of it, and let the rest expire. You stop fighting the tool.

---

## 5. Ingest: two write paths

**Direct writes — the fast path.** When a decision is agreed mid-session, a preference is
stated, or a correction lands for the second time, the agent writes the fact straight into
`knowledge/facts/` via the `regesto-write` skill — schema-correct, on any machine, no
waiting for lint. And **unprompted**: the triggers live in `SCHEMA.md`, in the always-loaded
instructions, and in the skill's own description. You should never have to say "write that
down."

Unprompted writing is a judgement call — no hook can force it — so compliance is
probabilistic, like all instructions. That is acceptable *because the second path exists*:

**Harvest — the safety net.** Agents write to their native memory automatically. That
capture is free and already happening; it is the one genuinely hard problem the vendors
solved for you. Anything the agent noticed but did not record arrives this way one cycle
later: delayed, not lost — with one honest caveat. Native memory is a cache the agent
actively prunes (§1), so a note written *and pruned* between two harvest snapshots never
appears in any diff and is gone. **The harvest interval is the loss window**: run it every
few minutes, not hourly, and treat the direct write as the real guarantee for anything
that matters. If the agent did both, reconcile collapses the duplicate — the redundancy is
a feature.

Collection depends on no cooperation from the agent: keep a snapshot in `.state/` of each
vendor file as last seen; new capture = *current file − last snapshot*.

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

The periodic pass, `regesto cycle`. This is the only real engineering in the project, and
the part no product ships.

1. **Collect** — pick up new captures from `inbox/`
2. **Normalize** — convert raw captures into canonical frontmatter; assign `id`, `subject`,
   `relation`, `source`
3. **Validate** — schema-lint all of `knowledge/facts/`, including facts written directly,
   which never pass through `inbox/`
4. **Reconcile** — same `(subject, relation)` → newer `modified` wins, among
   `active`/`proposed` claims and never across the agent→human trust boundary; the older
   flips to `status: superseded` with a backlink. **Never delete.**
5. **Rebuild** — regenerate `knowledge/topics/` and `INDEX.md`
6. **Resolve** — sync-conflict copies
7. **Commit**

Normalisation treats sources per channel (§8): a private single-user channel normalises
like any other agent; captures from channels a third party can reach are skipped entirely —
quarantined in `inbox/`, invisible to search and hooks, until a human promotes them.

**Lint runs on one machine.** Capture happens everywhere; deciding what becomes a fact
happens in one place, or two machines mint competing vocabulary for the same claim.

---

## 7. The agents

| Agent | Native store | Scope | Consultation |
|---|---|---|---|
| Claude Code | per-project memory directory | per git repo | **hook-enforced** |
| Codex CLI | global memory directory | global | instructions + skills |
| Hermes | global memory directory | global, hard char cap | instructions + skills |
| Chat apps | vendor-internal | — | manual promotion only |

Claude Code's is the only per-project store, and its directory name derives from the
absolute repo path — so the same project fragments across clones and across machines. The
adapter normalises all of them onto one canonical project name, derived from the git
remote's basename, with a hand-kept map in the instance config for remote-less repos and
for basename collisions.

Three vendors independently converging on a markdown memory file is good evidence the
format is right. It is not a reason to let any of them own your knowledge.

**Adding a fourth agent** is an adapter: a pointer block, an instruction, and a hook if the
platform has one. Nothing in `knowledge/` changes. That is the entire point, and a new
agent should be a pull request that touches only `internal/adapters` and `adapters/`.

---

## 8. Phones

An obvious design is a public HTTPS endpoint so a phone app's cloud can reach your
machine. That means a permanently listening service holding your entire personal knowledge
base, reachable from the internet — and it still leaves some vendors out.

**A chat-surface agent running on your own machine replaces it.** The machine makes
outbound connections only: no tunnel, no open port, no public attack surface.

**Trust follows the channel, not the agent.** A private, single-user channel is you, so its
writes are trusted like any other agent's — with every such write listed in the run summary
as a compensating check, because the agent acts unattended and content it reads for you
(forwarded mail, fetched links) is still third-party. On a channel others can reach, anyone
who can message you can attempt to plant false knowledge, so writes from those channels are
**quarantined**: never normalised, raw in `inbox/`, invisible to search, hooks and the
index, until a human promotes them. A planted claim that reached a session's context before
review would defeat the quarantine, so quarantined must mean *invisible*, not merely
flagged.

An always-on agent also means no quiet window for lint, which is why writes are atomic.

---

## 9. Sync and transport

**A sync client for transport, git for history.** Git-as-transport fails open: it needs a
pull before every session and a push after, and missing one means the agent confidently
reads stale knowledge. Hooks do not rescue it — an end-of-session hook cannot block, does
not fire on a crash or a closed lid, and its output reaches no one. An unreliable push is
the same fail-open failure with extra steps.

A file-sync client fails closed: worst case is a visible conflict file that lint
reconciles.

### 9.0 How the machines fit together

There is no server and no backend. The knowledge base is a folder, and sync keeps a **full
replica on every machine**. Reading is always a local file read — an agent on a laptop
greps the laptop's copy, offline if need be. No machine ever queries another to consult
knowledge.

Symlinks into the agents live *outside* the synced folder and point into it, so the sync
client only ever sees real files.

**Infrastructure is tiered, not prescribed:**

| Tier | What you run | What you get |
|---|---|---|
| One machine | nothing | the whole design, minus replication |
| Several machines | any file-sync client | a replica everywhere, offline reads |
| Plus an always-on node | the same client on a NAS or small server | a third replica that is always up, and a backup target |

The single-machine tier is not a degraded mode: consultation, writing and lint all work.
Sync is what makes the knowledge follow you, and nothing else depends on it.

### 9.1 Git lives on one machine

The repository is intentionally excluded from sync, and exists on the machine that runs
lint. Syncing `.git` is unenforceable: any agent session on a second machine runs
`git status` and becomes a second writer. The markdown is the knowledge; git is
convenience and offsite history.

A remote holds your accumulated personal knowledge in plaintext. Choose it deliberately —
a private hosted repo, a bare repo on a machine you own, or an encrypted remote.

---

## 10. Research verdicts

Kept because they save someone else the same week of reading.

**Validated:** markdown-in-git as truth with disposable indexes; a workstation as active
host and a NAS as durability; some phone apps structurally cannot reach a custom MCP
server, and others call connectors from the vendor's cloud and so need a public endpoint;
don't start with a graph database.

**Memory-palace vector products — rejected.** Independent reproduction of the best-marketed
one confirms the "memory palace" architecture it sells as its differentiator *makes
retrieval worse*, and its headline recall number is essentially unmodified off-the-shelf
vector search. Also disqualifying: opaque vectors rather than readable markdown.

**No vector database yet.** With structured frontmatter, scope and subject filters and a
generated index, grep is strong for a long time. Revisit when you can name real queries it
failed — not before.

**Pointing an agent's auto-memory directory at the knowledge base — avoid.** It points at
one directory, collapsing per-project isolation. The vendor issue requesting repo-relative
paths is open; until it ships, harvest-by-diff is the better mechanism, and it needs no
cooperation from the vendor at all.

**Cross-agent message relays — orthogonal.** Useful later as the transport for "the laptop
asks the workstation to run lint". Not memory.

---

## 11. Build order

The order matters, and it is the opposite of the intuitive one.

- **Foundation** — write `SCHEMA.md` for your own vocabulary if the shipped one does not
  fit. Everything else is plumbing.
- **Consultation** — index generation, search, the `SessionStart` hook, the skills. **This
  alone makes the knowledge base real**, before any automation exists.
- **The lint loop** — harvest by diff, normalize, reconcile, rebuild, commit.
- **Phones**, if you want them.

Consultation comes first because a knowledge base you cannot read is worthless, while a
knowledge base nobody writes to automatically is merely manual. Stop after consultation and
it is still useful.

Code comments cite the phases of the original build log as `PLAN <phase>.<step>`; that
document is a private instance's history and is not published. The mapping is: **0**
foundation, **1** consultation, **2** the lint loop, **3** phones, **4** this extraction.

---

## 12. Honest trade-offs

**It is not zero-infrastructure.** It needs a hook and a periodic job. Mitigation: every
layer degrades gracefully. Hook breaks → you still grep. Lint breaks → facts still
accumulate in `inbox/`. Nothing is ever *only* in a generated artifact.

**Generated pages and the index can drift** if lint fails silently. They are regenerable
from `facts/`, so treat them as caches and rebuild rather than repair.

**Consultation is guaranteed only where a hook exists.** Elsewhere it relies on
instructions, which are probabilistic. Be honest about that rather than assuming parity.

**Hook latency is real.** A per-session hook is free; a per-prompt hook runs on every
message. Keep it to a cheap index lookup, or you will feel it constantly and disable it.
This is why the engine is a single static binary rather than a script: a cold start is
milliseconds, and that is what makes the hook safe to run.

**You own a format now.** That is the point, but `SCHEMA.md` has to stay disciplined or the
knowledge base rots into free-form notes with inconsistent frontmatter and lint stops
working.

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
