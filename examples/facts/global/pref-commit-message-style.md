---
schema_version: 1
id: pref-commit-message-style
title: Commit subjects are imperative and under 60 characters
type: preference
scope: global
subject: git-commits
relation: message-style
topics: [conventions]
status: active
source: human
created:  2026-03-04T09:12:00Z
modified: 2026-03-04T09:12:00Z
---

Commit subjects are written in the imperative ("add the retry budget", not "added" or
"adds") and kept under 60 characters. The body explains why, not what.

**Why:** the subject line is read in `git log --oneline` far more often than the diff, and
a consistent mood makes a long log skimmable. Sixty characters is what fits before most
tools truncate.
