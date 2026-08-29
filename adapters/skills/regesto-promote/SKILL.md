---
name: regesto-promote
description: Promote an exported chat transcript from a host that cannot access local files into durable Regesto facts, then archive the raw export. Use only when the user explicitly requests this side-effecting batch operation.
---

# Promote a chat export

This is a deliberate batch operation. Never look for exports or run it automatically.
The knowledge-base root is `{{kb_root}}`.

1. Locate only the export the user named, commonly a `.zip` or `.json` file in a download
   directory or under `{{kb_root}}/archive/`.
2. Extract durable decisions and their reasons, conventions, repeated corrections,
   preferences, and non-obvious environment or project facts. Skip transient state and
   facts derivable from source material already at hand. Most transcripts yielding no
   facts is normal.
3. Write each fact through the `regesto-write` procedure: check controlled vocabulary,
   keep one claim per file, preserve provenance, and obey supersession rules. Use the
   original conversation integration as `<integration>@<machine>` when known; otherwise
   use the documented human-promotion source.
4. Move the raw transcript, without editing it, to
   `{{kb_root}}/archive/chat-exports/<YYYY-MM-DD>-<short-name>.<ext>`. Do not leave a
   duplicate in its original location and never delete archived exports.
5. Report every fact written as id, title, and path, plus the archive destination. Call
   out any proposed claim that still awaits human review.
