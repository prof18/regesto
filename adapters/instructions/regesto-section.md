<!-- regesto:section:start -->
## Knowledge base

Canonical knowledge lives at `{{kb_root}}` — decisions, conventions, preferences and
facts of every kind, one claim per file. Not only code: how this machine is set up, how
the user wants to be worked with, and matters of their life and admin that live in no
repo. It is the record, not optional background.

**Consult it** before any answer or decision a recorded claim could settle, technical or
not. Integrations with deterministic context injection may provide the relevant index
automatically. Otherwise start from `{{kb_root}}/INDEX.md` and search:

```
{{kb_root}}/bin/regesto-search [--subject S] [--relation R] [--scope SC] [terms...]
```

Ignore `status: superseded` unless the question is about history. A `[proposed]` claim is
awaiting human review — you may rely on it, but say that you are.

**Record to it, unprompted**, via the `regesto-write` skill, at the moment any of these happens
— not at the end of the session:

- the user asks to remember, store, or note something. This one is absolute: never reply
  "noted" or "I'll keep that in mind" without writing the file.
- a decision is agreed ("let's go with X because Y")
- a preference is stated, or a correction is given for the second time
- a non-obvious fact surfaces about the environment, the tooling, a project, or the
  user's own life and admin
- a recurring approach is settled on

Neither behaviour is a per-request favour; both are standing. Before minting a new
`(subject, relation)` pair, check the controlled vocabulary in `INDEX.md` — reusing terms is
what lets a later claim supersede an earlier one instead of silently contradicting it.

If what you have is half-formed, capture it to `{{kb_root}}/inbox/human@<machine>/`
rather than forcing a wrong pair — `<machine>` is this machine's short name, which
`{{kb_root}}/bin/regesto-config` prints. Contract: `{{kb_root}}/SCHEMA.md`.
<!-- regesto:section:end -->
