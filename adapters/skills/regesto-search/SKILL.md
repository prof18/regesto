---
name: regesto-search
description: Search the personal Regesto knowledge base for prior decisions, conventions, preferences, environment details, and project facts before answering or deciding anything that recorded knowledge could settle.
---

# Search the knowledge base

The knowledge-base root is `{{kb_root}}`.

Use any result lines already supplied by an integration-specific section below. Otherwise
run the stable search command with the user's actual query terms:

```sh
{{kb_root}}/bin/regesto-search <query terms>
```

Results are tab-separated `id`, `title`, and relative `path` fields. Read a matching claim
in full before relying on it. A claim tagged `[proposed]` awaits human review; it may be
used only with that caveat. Superseded claims are hidden unless history was requested.

Useful exact filters are available when free text is too broad:

```sh
{{kb_root}}/bin/regesto-search [--subject S] [--relation R] [--scope SC] [--history] [terms...]
```

`--scope` accepts `global` or a project name. If the search returns no lines, say that
nothing is recorded on the topic rather than inventing an answer.
