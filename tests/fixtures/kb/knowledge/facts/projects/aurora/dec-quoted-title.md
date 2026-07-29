---
schema_version: 1
id: dec-quoted-title
title: "Aurora logging: structured JSON only"
type: decision
scope: project:aurora
subject: logging
relation: output-format
topics: [aurora]
status: active
source: human
created: 2026-07-15T11:00:00Z
modified: 2026-07-15T11:00:00Z
---

Aurora logs structured JSON only, never free-text lines.

**Why:** a title containing a colon must be YAML-quoted, so this fixture also
pins that the parser strips the quotes rather than treating them as content.
