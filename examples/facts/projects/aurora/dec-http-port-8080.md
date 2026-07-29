---
schema_version: 1
id: dec-http-port-8080
title: The API listens on 8080 in development
type: decision
scope: project:aurora
subject: http-server
relation: port
topics: [aurora]
status: active
supersedes: dec-http-port-8000
source: codex@laptop
created:  2026-05-02T08:30:00Z
modified: 2026-05-02T08:30:00Z
---

The development server listens on port 8080.

**Why:** 8000 collided with the docs preview server once both were running, and the
collision surfaced as a confusing 404 rather than a bind error.

Note what this pair demonstrates: `8000` → `8080` is a value flip, which is *more* similar
to the original than an honest rephrase would be. Similarity search cannot tell the two
apart. The matching `(subject, relation)` can.
