---
schema_version: 1
id: dec-http-port-8080
title: Aurora dev server listens on port 8080
type: decision
scope: project:aurora
subject: http-server
relation: port
topics: [aurora]
status: active
supersedes: dec-http-port
source: claude@testbox
created: 2026-07-22T08:30:00Z
modified: 2026-07-22T08:30:00Z
---

The aurora dev server listens on port 8080.

**Why:** 8000 collided with the metrics exporter; moved once, recorded so it
never gets "fixed" back.
