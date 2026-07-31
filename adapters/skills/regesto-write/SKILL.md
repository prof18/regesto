---
name: regesto-write
description: Record a fact in the knowledge base at {{kb_root}}. Use IMMEDIATELY when the user asks to remember/store/note something (this trigger is absolute — never reply "noted" without writing the file); when a decision is agreed in conversation ("let's go with X because Y"); when a preference is stated or a correction is given for the second time; when a non-obvious fact about the environment, tooling, or a project surfaces; when a recurring approach is settled on; when a gotcha is discovered. Do it at the moment it happens, unprompted — not at the end of the session. Also the manual store command, /regesto-write <statement> or bare /regesto-write.
when_to_use: The moment any trigger above occurs, mid-session, without being asked. Also on explicit invocation.
allowed-tools: Read, Grep, Write, Bash
---

# regesto-write — record a fact

This is `{{kb_root}}/SCHEMA.md` as an executable procedure. The KB root is
`{{kb_root}}`.

## Values to look up, never guess

Run these rather than inferring them — they must match what the rest of the
system uses for the same repo and machine:

```
{{kb_root}}/bin/regesto-project --scope    # scope:  project:<name>, run from the repo
{{kb_root}}/bin/regesto-config             # machine=<name>, for source: and the inbox path
date -u +%Y-%m-%dT%H:%M:%SZ                 # created: / modified:
```

Guessing the date is the most common error here: check it, never assume.

## Invocation shapes

Work out which one applies before you start:

- **Invoked with a statement after it** — that statement *is* the claim; record the
  user's own words. Claude Code substitutes them into this skill for you, while
  Codex and Hermes do not (confirmed 2026-07-28) — so in either case take the claim
  from wherever you can actually see the user's words: substituted here, or in the
  message they sent. Never record a placeholder.
- **`/regesto-write` with nothing after it** — record what was just agreed in the
  conversation. Say which claim you settled on before writing, so a wrong reading
  is caught cheaply.
- **Unprompted** — a trigger from the description fired. Record it now, mid-session,
  without waiting to be asked.

If you genuinely cannot tell what to record, ask — one short question beats a
wrong `(subject, relation)` pair, which is silent corruption.

## Procedure

1. **State the claim as one sentence.** One claim per file. If what you have
   is half-formed — a fragment, a hunch, no clear `(subject, relation)` —
   do NOT force it: write it verbatim to
   `{{kb_root}}/inbox/human@<machine>/<timestamp>-note.md` — `<machine>` from
   `bin/regesto-config` — and stop. Create the directory if absent. A rough note
   in the inbox beats a wrong pair in `facts/`.

2. **Check "Don't record".** Skip anything derivable from reading the code,
   transient state (branch names, in-flight work), restatements of official
   docs. Test: would a competent stranger need to be told this, and would
   they struggle to find it out?

3. **Choose `type`** and id prefix: `decision`/`dec-`, `preference`/`pref-`,
   `fact`/`fact-`, `pattern`/`pat-`.

4. **Choose `(subject, relation)` — check the vocabulary FIRST.** Open
   `{{kb_root}}/INDEX.md`, section "Controlled vocabulary". If an existing
   pair fits, reuse it **exactly**; only mint a new pair when nothing fits.
   Pick the pair so a future contradicting claim would land on the same pair:
   `subject: gradle` + `relation: console-flags`, never the whole sentence.
   A synonym pair (`cli-flags` vs `console-flags`) means a contradiction is
   never detected — silent corruption.

5. **Choose `scope`**: `global` if the claim is true beyond one project (put the
   projects in `topics:` instead), otherwise the `project:<name>` that
   `bin/regesto-project --scope` prints when run from the repo. Scope determines
   the path: `knowledge/facts/global/<id>.md` or
   `knowledge/facts/projects/<name>/<id>.md`.

6. **Assign `id`**: `<prefix>-<kebab-slug>`, unique across the whole store —
   check `INDEX.md` first. Stable once written; never rename by hand.

7. **Check for a contradicted claim.** Search for the same
   `(subject, relation)` (`bin/regesto-search --subject S --relation R`). If an
   existing claim is contradicted:
   - Add `supersedes: <old-id>` to the new file.
   - If the old claim's `source` is `human` and you are an agent: the new
     claim gets `status: proposed`; the old file is NOT touched — a human
     resolves it (SCHEMA, "Resolving a review").
   - Otherwise: new claim `status: active`; edit the old file's `status` to
     `superseded` and bump its `modified`. **Never edit the old claim's
     body** — the record of what was believed is the point.
   - **Never delete anything.**

8. **Write the file** with this frontmatter, then the claim body and a
   `**Why:**` line:

   ```markdown
   ---
   schema_version: 1
   id: <prefix>-<kebab-slug>
   title: <one line, ≤80 chars, what the claim asserts>
   type: decision | preference | fact | pattern
   scope: global | project:<name>
   subject: <reused-or-new-term>
   relation: <reused-or-new-term>
   topics: [<topic-slugs, optional>]
   status: active | proposed
   supersedes: <old-id, only if superseding>
   source: <agent>@<machine>   # you, e.g. claude@studio. `human` only when the
                               # user asserted it themselves — that source is
                               # never auto-superseded by an agent claim.
   created:  <date -u output>
   modified: <same as created for a new file>
   ---

   <The claim, stated plainly.>

   **Why:** <the reasoning — this is what makes the fact worth keeping.>
   ```

9. **Confirm** to the user in one line: the id and path you wrote.

Do not edit `INDEX.md` or `knowledge/topics/` — they are generated; the next
`bin/regesto-index` run picks the new fact up.
