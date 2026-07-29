---
schema_version: 1
id: fact-build-flag
title: Aurora release builds require the neon build tag
type: fact
scope: project:aurora
subject: build
relation: required-flags
topics: [aurora, tooling]
status: active
source: human
created: 2026-07-10T16:00:00Z
modified: 2026-07-10T16:00:00Z
---

Release builds must pass `-tags neon` or the SIMD path silently falls back to
the scalar implementation.

**Why:** took a full afternoon to rediscover; exactly the kind of non-obvious
tooling fact the KB exists for.
