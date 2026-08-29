---
name: regesto-promote
description: Promote a chat export (Claude/ChatGPT app conversation) into knowledge-base facts, then archive the raw transcript. Deliberate batch operation with side effects — invoked by the user, never automatically.
when_to_use: Only when the user explicitly asks to promote an export or a transcript into the KB.
disable-model-invocation: true
allowed-tools: Read, Grep, Write, Bash
---

# regesto-promote — chat export → facts → archive

The only ingestion path for the Claude and ChatGPT mobile/desktop apps, which
cannot participate in the KB automatically. KB root: `{{kb_root}}`.

## Procedure

1. **Locate the export.** The user names a file (often a `.zip` or `.json`
   in `~/Downloads`, or already filed under `{{kb_root}}/archive/`). Do not
   go looking for exports unprompted.

2. **Read the transcript and extract durable facts** — apply SCHEMA.md's
   "What to record": decisions and why, conventions, repeated corrections,
   preferences, non-obvious facts about the environment, the tooling or the
   user's own life and admin — not only code. Skip anything derivable from a
   source already at hand, and transient state.
   Expect most of a transcript to yield nothing; that is normal.

3. **Write each fact via the `regesto-write` procedure** (its skill body is the
   authoritative template): vocabulary check against `INDEX.md` first, one
   claim per file, supersession rules respected. `source` is the agent that
   held the conversation if known (e.g. `claude@<machine>`), else `human`.
   A promoted claim enters `facts/` as `active` and keeps its source for
   provenance.

4. **Archive the raw transcript** to
   `{{kb_root}}/archive/chat-exports/<YYYY-MM-DD>-<short-name>.<ext>` —
   move, don't copy, so nothing lingers in Downloads. The archive is
   immutable: never edit files under `archive/`, and never delete them.

5. **Report**: list every fact written (id — title — path) and where the
   transcript was archived. If a candidate fact contradicted a `human` claim
   and landed as `proposed`, say so — it waits for review.
