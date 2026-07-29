---
name: regesto-search
description: Search the personal knowledge base at {{kb_root}} for prior decisions, conventions, preferences, and facts. Use before answering questions about past work, architecture, conventions, or the environment, and before making a decision a prior fact might settle.
when_to_use: Before decisions or answers that prior decisions, stated preferences, or recorded project facts could affect. Pass the query as arguments.
allowed-tools: Bash({{kb_root}}/bin/regesto-search:*), Read, Grep
---

# regesto-search

**Check which case you are in before answering.** Under "Matching knowledge" below
you will see one of two things:

1. **Result lines** — `id`, `title`, `path`, tab-separated, one per line. Claude Code
   runs the search before you see this, so the results are already there. Use them.
2. **An unexecuted command line starting with `` !` ``** — Codex and Hermes do not run
   it (confirmed 2026-07-28), and the query placeholder in it is left unexpanded too.
   Run that command yourself **now**, before answering, putting the user's actual
   search terms where the placeholder is. Then use its output.

Either way, consult the knowledge base before you answer rather than answering from
memory.

## Matching knowledge

!`{{kb_root}}/bin/regesto-search "$ARGUMENTS"`

---

To read a claim in full, open the path listed for it, relative to `{{kb_root}}`.
A claim tagged `[proposed]` is awaiting human review — you may rely on it, but say
that you are. No lines at all means nothing is recorded on the topic; say so rather
than inventing an answer.

Full usage, for narrowing a query or widening it to history:

```
{{kb_root}}/bin/regesto-search [--subject S] [--relation R] [--scope SC] [--history] [terms...]
```

Field filters match exactly and free-text terms AND together over frontmatter and
body. `--scope` takes `global` or a project name. Superseded claims are hidden
unless you pass `--history` — they are history, not current knowledge.
