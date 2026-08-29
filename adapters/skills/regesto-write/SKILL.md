---
name: regesto-write
description: Record one durable fact in the Regesto knowledge base immediately when the user asks to remember something, a decision or preference is settled, or a non-obvious recurring fact or gotcha surfaces.
---

# Record one durable fact

The knowledge-base root is `{{kb_root}}`. Use this procedure immediately when its trigger
fires; do not defer the write until the end of the session.

## Resolve values instead of guessing

Run these commands from the relevant project:

```sh
{{kb_root}}/bin/regesto-project --scope
{{kb_root}}/bin/regesto-config
```

Use the current integration id plus the returned machine name as `--source`, in the form
`<integration>@<machine>`. Never label an agent assertion `human`; that source is reserved
for explicit human promotion or the documented human inbox workflow.

## Procedure

1. State one claim in one sentence. If the available information is too fragmentary for a
   reliable `(subject, relation)` pair, put the verbatim note in
   `{{kb_root}}/inbox/human@<machine>/` instead of forcing a fact.
2. Skip transient state, restatements of source material already at hand, and anything a
   competent reader can derive easily from the code.
3. Choose `type` and id prefix: `decision`/`dec-`, `preference`/`pref-`, `fact`/`fact-`,
   or `pattern`/`pat-`.
4. Read `{{kb_root}}/INDEX.md` and reuse its controlled `(subject, relation)` vocabulary
   exactly. A synonym creates a separate claim stream and can hide contradictions.
5. Choose `global` scope only when the claim is true beyond one project. Otherwise pass
   `"scope":"project"` with `--dir <repo>` and let Regesto resolve the canonical name.
6. Search the chosen pair for an existing contradictory claim. When superseding one, add
   its id as `supersedes`; never delete or hand-edit the incumbent.
7. Submit only the semantic fields to the validated writer. Regesto supplies timestamps,
   schema version, path, validation, and reconciliation state.

```sh
{{kb_root}}/bin/regesto-write --source <integration>@<machine> --dir <repo> --json-input --json <<'JSON'
{
  "id": "<prefix>-<kebab-slug>",
  "title": "<one line, at most 80 characters>",
  "type": "fact",
  "scope": "project",
  "subject": "<controlled-subject>",
  "relation": "<controlled-relation>",
  "topics": ["<optional-topic>"],
  "status": "active",
  "body": "<claim>",
  "why": "<why it is worth retaining>"
}
JSON
```

The accepted input fields are `id`, `title`, `type`, `scope`, `subject`, `relation`,
optional `topics`, optional `status` and `supersedes`, `body`, and `why`. Do not supply
`source`, `schema_version`, `created`, or `modified` inside the object. Report the returned
id and relative path after the write succeeds.
