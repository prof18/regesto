---
schema_version: 1
id: dec-http-port-8000
title: The API listens on 8000 in development
type: decision
scope: project:aurora
subject: http-server
relation: port
topics: [aurora]
status: superseded
source: codex@laptop
created:  2026-02-11T11:05:00Z
modified: 2026-05-02T08:30:00Z
---

The development server listens on port 8000.

**Why:** it was free on every machine in the team at the time.

*This claim is kept as an example of what supersession looks like. It is no longer true —
see `dec-http-port-8080`, which names the same `(subject, relation)` pair. The body is
never edited to "fix" a superseded claim: the record of what was believed is the point.*
