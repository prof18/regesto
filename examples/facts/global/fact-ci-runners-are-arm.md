---
schema_version: 1
id: fact-ci-runners-are-arm
title: CI runners are arm64, so amd64-only images fail there and not locally
type: fact
scope: global
subject: ci-runners
relation: architecture
topics: [ci, conventions]
status: active
source: claude@laptop
created:  2026-04-18T14:40:00Z
modified: 2026-04-18T14:40:00Z
---

The CI runners are arm64. An image or a prebuilt binary published for amd64 only will
build on a developer machine and fail in CI with an exec-format error that names no
architecture.

**Why:** the error message is unhelpful and the failure is machine-dependent, so this costs
an hour every time someone rediscovers it. Publish multi-arch, or pin the platform
explicitly.
