# SCHEMA — the knowledge base contract

This file defines how knowledge is stored. It is hand-written and authoritative: agents
follow it, generated artifacts conform to it, and `regesto` implements it.

It ships in two places — in the engine as the spec, and in every instance `regesto init`
creates, as the document the write skill points agents at. So it names no sibling file:
the reasoning behind these choices, and the rule that a schema change needs a migration
note, both live in the engine repository.

## The rule that governs everything

**Nothing is ever deleted.** A claim that stops being true is marked `status: superseded`,
not removed. History is part of the knowledge.

## Layout

```
INDEX.md                           GENERATED manifest + controlled vocabulary
SCHEMA.md                          this file
knowledge/facts/global/            applies everywhere
knowledge/facts/projects/<name>/   scoped to one project, by canonical name
knowledge/topics/                  GENERATED entity pages linking related facts
inbox/<agent>@<machine>/           raw captures awaiting normalization
adapters/                          per-provider glue: skills, hooks, instructions
archive/                           raw sources. Immutable. Grepped, never loaded.
bin/                               scripts: regesto-search, lint, index generation
.state/<machine>/                  per-machine diff baselines. NOT synced.
.state/write-locks/                local validated-write coordination. NOT synced.
docs/                              design documents. NOT knowledge — ignore when consulting.
```

Project names are **canonical**, not paths: `aurora`, not `apps/aurora` or
`/Users/you/work/aurora`. Every checkout on every machine maps to the same name.

## A fact

One claim per file, at `knowledge/facts/<scope-path>/<id>.md`.

```markdown
---
schema_version: 1
id: dec-kmp-expect-actual-boundary
title: Platform services cross the KMP boundary as interfaces
type: decision
scope: project:aurora
subject: expect/actual
relation: boundary-rule
topics: [kmp-conventions, aurora]
status: active
supersedes: dec-kmp-interface-boundary
source: codex@macbook
created:  2026-07-27T10:00:00Z
modified: 2026-07-27T10:00:00Z
---

Platform services cross the boundary as interfaces, not expect/actual — expect/actual is
reserved for compile-time constants and type aliases.

**Why:** SKIE generates cleaner Swift interop for interfaces, and expect/actual forced
duplicate implementations per platform.
```

### Fields

| Field | Required | Values |
|---|---|---|
| `id` | yes | `<prefix>-<kebab-slug>`. Stable once written — never renamed by hand; the only exception is lint's collision rule below, which also fixes the references. |
| `title` | yes | One line, ≤80 chars. What the claim asserts. Used to build `INDEX.md`. |
| `type` | yes | `decision` · `preference` · `fact` · `pattern` |
| `scope` | yes | `global` or `project:<name>` — exactly one; it determines the file's path. A claim true for several projects is `global`, with the projects in `topics` |
| `subject` | yes | what the claim is about — **reuse existing terms** |
| `relation` | yes | which property of the subject is asserted — **reuse existing terms** |
| `topics` | no | list of topic-page slugs this fact belongs on |
| `status` | yes | `active` · `proposed` · `superseded` |
| `supersedes` | no | `id` of the claim this replaces |
| `source` | yes | `<agent>@<machine>`, or `human` |
| `created` | yes | ISO 8601 UTC. Never changes. |
| `modified` | yes | ISO 8601 UTC. Updated on every edit. |
| `review_after` | no | ISO date. For time-bound claims. Lint flags these when due. |
| `schema_version` | yes | Integer, currently `1`. The schema revision this file conforms to — lets a future lint migrate old files mechanically instead of guessing. |

Id prefixes by type: `dec-`, `pref-`, `fact-`, `pat-`.

**Id collisions.** Ids must be unique across the whole store. Check `INDEX.md` before
assigning one. If two machines produce the same id for different claims, lint renames the
newer by `modified` with a `-2` suffix, fixes any `supersedes:` references that pointed at
the old spelling, and reports it.

### `subject` + `relation` — the identity of a claim

These two fields together identify *what is being asserted about what*. Two files sharing
the same pair make the same claim, which is what turns contradiction handling into a lookup
instead of a judgement call.

**This only works if terms are reused.** If one agent writes `relation: console-flags` and
another writes `cli-flags` for the same claim, the contradiction is never detected and both
stay `active` — silent corruption of the whole model.

So:

1. **Before inventing a pair, check the controlled vocabulary in `INDEX.md`.** If an existing
   `(subject, relation)` fits, use it exactly.
2. Only mint a new pair when nothing existing describes the claim.
3. Lint reports near-duplicate pairs for you to merge.

Pick them so a future contradicting claim would naturally land on the same pair:

- ✅ `subject: gradle`, `relation: console-flags`
- ❌ `subject: always run gradle with -q --console=plain` — the whole claim, so nothing can
  ever collide with it, so it can never be superseded

## What to record

**Record** — and not only about code: decisions and *why*; conventions that differ from
tool defaults; corrections you had to give more than once; non-obvious facts about the
environment, the tooling, or the user's own life and admin; preferences about how work
should be done; anything that would take real effort to rediscover.

**Don't record:** anything derivable from a source already at hand — the code, this repo's
own files, official documentation; transient state (branch names, current bugs, in-flight
work).

If in doubt: *would a competent stranger need to be told this, and would they struggle to
find it out?*

### Write unprompted

Agents record facts **at the moment they occur, without being asked**. The triggers:

- **the user asks to store something** — any phrasing ("remember this", "note that down",
  "store this in the kb", `/regesto-write …`) → record it *now*, whatever type fits. This
  trigger is absolute: never answer "I'll keep that in mind" without writing the file.
- a decision is agreed in conversation ("let's go with X because Y") → `dec-`
- a preference is stated, or a correction is given for the second time → `pref-`
- a non-obvious fact about the environment, tooling, or a project surfaces → `fact-`
- a recurring approach is settled on → `pat-`

Record it then and there via `regesto-write` — not at the end of the session, and never wait
for the user to say "note that down." The skill submits the claim through Regesto's validated
write interface, which owns the path, schema version, and timestamps and stamps the explicit
source. A missed trigger is not fatal (harvest picks up native memory later), but the direct
write is the fast path and the default.

If what the user hands over is half-formed — a fragment, a hunch, no clear
`(subject, relation)` yet — don't lose it and don't force it: capture it to
`inbox/human@<machine>/` as a raw note and let lint normalize it on the next pass.
That canonical namespace has an implicit supervised default, while an exact or
pattern source policy can still quarantine it. Facts produced from it are stamped
with the authoritative source `human`, not the inbox routing ID. A rough note in
the inbox beats a wrong pair in `facts/`.

## Superseding

When a new claim contradicts an existing one with the same `(subject, relation)`:

1. New file gets `supersedes: <old-id>`.
2. Old file gets `status: superseded` and a bumped `modified`. **Its content stays intact.**
3. Never edit the old claim's body to "fix" it. The record of what you used to believe is
   the point.

Resolution rules, in order:

1. **Only `active` and `proposed` claims enter reconciliation.** A `superseded` claim is
   out permanently — its bumped `modified` from step 2 above must never let it win a later
   pass.
2. **An agent-sourced claim never auto-supersedes a `human` claim.** It lands as
   `proposed` with `supersedes: <old-id>` recorded as *intent*; the human claim keeps
   `status: active`, and lint flags the pair for review. The human decides — see
   "Resolving a review" below. Recency only settles contests between equals.
3. Otherwise, newer `modified` wins. Never resolve by similarity or judgement.
4. **Lint reports every supersession it performs** in its run summary. Auto-resolution is
   mechanical, but it must never be silent — a wrongly-merged pair (two *complementary*
   claims that happened to share a `(subject, relation)`) is only catchable if you see it
   happen.

### Resolving a review

A `proposed` claim flagged against a `human` claim waits for exactly one human edit:

- **Accept:** set the proposed claim to `status: active`. Its `supersedes: <old-id>` is
  already in place, and lint's standing rule — *any claim named in an `active` claim's
  `supersedes:` is flipped to `superseded`* — retires the old claim on the next pass.
  Acceptance is a one-field edit; the human touching `status` is what authorizes crossing
  the trust boundary, so rule 2 no longer applies.
- **Reject:** set the proposed claim to `status: superseded` directly. It never becomes
  active, the human claim stands, and the record of the rejected assertion survives like
  any other superseded claim.

Until one of these happens, both claims stand — the incumbent `active`, the challenger
`proposed` — and lint re-reports the pending review on every pass rather than resolving
it itself.

## Status lifecycle

| Status | Meaning |
|---|---|
| `proposed` | In `facts/`, awaiting human review — created when an agent claim contradicts a `human` claim (Superseding, rule 2). Readable, but say so when relying on it. |
| `active` | Current and trusted. |
| `superseded` | Historical. Ignore unless asked about history. |

## Trust

`source` records who asserted a claim. Not all sources are equal:

- `human` — authoritative, lands as `active`. **Never auto-superseded by an agent claim**
  — see Superseding.
- Agent capture trust is resolved from the configured integration surface, not a
  product name or spelling of `source`. Before any rule applies, a capture must use
  exactly `<integration>@<machine>` with nonempty parts, and its integration and
  machine must equal the inbox directory's integration ID and machine. Malformed or
  mismatched captures are quarantined.
- Source policy resolves in this order: an exact `[source_policies]` entry; an exact
  legacy `[trusted_sources]` entry; the longest matching trailing-`*`
  `[source_policies]` prefix; the reserved `human@<machine>` inbox authority; the
  configured integration's `default_trust`; then quarantine. `human` cannot be a
  configured integration ID, and facts normalized from its inbox namespace are
  stamped with source `human`. `[source_policies]` values are exactly `supervised`
  or `quarantine`.
  Its exact keys are complete `<integration>@<machine>` source IDs; its patterns have
  one final `*`, such as `"hermes@studio-*" = "quarantine"`. Exact rules therefore
  can explicitly quarantine a stale legacy approval, while legacy entries retain their
  meaning whenever no exact source policy conflicts.
- A configured supervised local integration (currently Claude Code and Codex) may
  normalize its own captures. The legacy Hermes integration defaults to quarantine;
  its private, single-user surface keeps its existing trust through the exact
  `[trusted_sources]` entry. Unknown, custom, unattended, unconfigured, and empty
  sources default to quarantine unless an explicit policy or configured trust says
  otherwise. Give separate surfaces separate integration IDs (for example
  `hermes-private` and `hermes-public`) even if they use the same profile, then assign
  each its own default trust. This keeps provenance namespaces distinct without
  changing the `source` schema.
- A quarantined capture is never normalized: it stays raw in
  `inbox/<integration>@<machine>/`, invisible to `regesto-search`, hooks, and
  `INDEX.md`, until a human reviews and promotes it (by hand or via
  `regesto-promote`). A promoted claim enters `facts/` as `active` and keeps its
  source for provenance. Quarantined must mean *invisible*: a planted claim that
  reaches a session's context before review has already done its damage.

## Inbox lifecycle

The inbox path is only for **harvested** native-memory captures. Direct writes — an agent
invoking `regesto-write`, or a human editing — go straight into `knowledge/facts/` following
this schema, on any machine; lint validates them in place on its next pass.

1. Harvest writes raw captures to `inbox/<agent>@<machine>/`.
2. Lint normalizes each into a fact under `knowledge/facts/` — **except quarantined
   captures** (as resolved by the source policy in Trust), which it never touches:
   they wait in the inbox until a human promotes them.
3. The raw capture moves to `archive/inbox/<date>/` — not deleted, so a bad normalization
   can be re-run against the original.

## Generated artifacts

`INDEX.md` and everything under `knowledge/topics/` are **rebuilt from `facts/`**. Never
hand-edit them — edits are lost on the next lint pass. If a topic page is wrong, fix the
underlying fact.

`docs/` holds design documents *about* this system. It is **not knowledge** — skip it when
consulting the knowledge base.
