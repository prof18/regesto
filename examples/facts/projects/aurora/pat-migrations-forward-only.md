---
schema_version: 1
id: pat-migrations-forward-only
title: Migrations are forward-only; a mistake is fixed by a new migration
type: pattern
scope: project:aurora
subject: database-migrations
relation: direction
topics: [aurora]
status: active
source: claude@laptop
created:  2026-06-09T10:00:00Z
modified: 2026-06-09T10:00:00Z
---

Migrations only go forward. There are no `down` scripts; a migration that turned out to be
wrong is corrected by writing another one.

**Why:** a `down` script is written when the schema is fresh in mind and run, if ever,
months later against data it was never tested on. Rolling forward is the path that is
actually exercised.
