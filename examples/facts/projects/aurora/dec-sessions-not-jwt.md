---
schema_version: 1
id: dec-sessions-not-jwt
title: Sessions are server-side; the cookie carries only an opaque id
type: decision
scope: project:aurora
subject: auth
relation: session-transport
topics: [aurora, security]
status: active
source: human
created:  2026-01-22T16:20:00Z
modified: 2026-01-22T16:20:00Z
---

Session state lives server-side. The cookie carries an opaque identifier and nothing else —
no claims, no JWT.

**Why:** revocation has to be immediate. A self-contained token is valid until it expires,
and every workaround for that (short expiry plus refresh, a denylist) rebuilds server-side
state anyway, with more moving parts.
