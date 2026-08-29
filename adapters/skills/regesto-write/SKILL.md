---
name: regesto-write
description: Record a fact in the knowledge base at {{kb_root}}. Use IMMEDIATELY when the user asks to remember/store/note something (this trigger is absolute — never reply "noted" without writing the file); when a decision is agreed in conversation ("let's go with X because Y"); when a preference is stated or a correction is given for the second time; when a non-obvious fact surfaces about the environment, tooling, a project, or the user's own life and admin; when a recurring approach is settled on; when a gotcha is discovered. Do it at the moment it happens, unprompted — not at the end of the session. Also the manual store command, /regesto-write <statement> or bare /regesto-write.
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
{{kb_root}}/bin/regesto-config             # machine=<name>, for source:
```

Use your integration id plus that machine for `--source` (for example
`claude@studio`). Never silently call an agent assertion `human`: that source is
reserved for explicit human promotion or the documented `inbox/human@<machine>/`
workflow.
The validated command stamps `schema_version`, `created`, and `modified`; do not
invent or supply them yourself.

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
   `bin/regesto-project --scope` prints when run from the repo. You may instead
   send `"scope": "project"` with `--dir <repo-dir>` and let Regesto resolve the
   canonical project name. Regesto determines the output path.

6. **Assign `id`**: `<prefix>-<kebab-slug>`, unique across the whole store —
   check `INDEX.md` first. Stable once written; never rename by hand.

7. **Check for a contradicted claim.** Search for the same
   `(subject, relation)` (`bin/regesto-search --subject S --relation R`). If an
   existing claim is contradicted:
   - Add `supersedes: <old-id>` to the new file.
   - Do not edit the incumbent claim. The write result reports pending
     reconciliation; an agent challenge to a human claim remains for human
     review as SCHEMA's "Resolving a review" describes.
   - **Never delete anything.**

8. **Submit the strict JSON object** to the validated writer. It accepts only
   `id`, `title`, `type`, `scope`, `subject`, `relation`, optional `topics`,
   optional `status`/`supersedes`, `body`, and `why`. It creates the fact
   atomically, chooses its canonical path, stamps protected metadata, and
   returns JSON containing the relative path and reconciliation state.

   ```sh
   {{kb_root}}/bin/regesto-write --source <integration>@<machine> --json-input --json <<'JSON'
   {
     "id": "<prefix>-<kebab-slug>",
     "title": "<one line, at most 80 chars>",
     "type": "decision",
     "scope": "global",
     "subject": "<reused-or-new-term>",
     "relation": "<reused-or-new-term>",
     "topics": ["<optional-topic>"],
     "status": "active",
     "body": "<the claim, stated plainly>",
     "why": "<the reasoning that makes it worth keeping>"
   }
   JSON
   ```

   For `"scope": "project"`, add `--dir <repo-dir>`; do not guess the
   canonical project name. Do not include `source`, `schema_version`,
   `created`, or `modified` in the JSON object.

9. **Confirm** to the user in one line: the id and relative path returned.

Do not edit `INDEX.md`, `knowledge/topics/`, or another claim by hand — they are
generated or reconciled by the normal downstream pass.
