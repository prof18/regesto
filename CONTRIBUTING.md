# Contributing

Bug reports, adapters for new agents and documentation fixes are all welcome. Read the
sections below before you start.

## The one rule: schema changes require a migration note

`SCHEMA.md` is a contract with data that already exists on other people's disks. Every
instance out there holds facts written against some revision of it, and nobody is going to
hand-edit a thousand markdown files.

So a pull request that changes `SCHEMA.md` must also:

1. **Bump `schema_version`** and say what revision the change lands in.
2. **Add a migration note** — a short section stating what changed, what an older file
   looks like, and what it should become. Make it mechanical if at all possible: "field
   `x` is now required, default it to `y`" is a migration; "rethink your subjects" is not.
3. **Say what old files do in the meantime.** Older facts must keep parsing and keep being
   searchable. A change must not break reads; to retire a field, deprecate it over a
   revision.

The same applies to renaming a field, changing what a value means, or tightening a
validation rule. Adding an optional field is the one case that needs only the version bump.

If your change cannot be migrated mechanically, it probably belongs in the instance's own
vocabulary rather than in the schema.

The same reasoning covers `adapters/`, `bin/` and `SCHEMA.md` generally: those files are
copied *into* people's knowledge bases, and `regesto upgrade` will offer to replace them.
It refuses to touch any copy that differs from what the engine recorded writing, so a
change of yours reaches an instance only where the user never edited that file. Write them
as if the user has already forked their copy, because some users have.

## Adding an agent

Prefer a declarative integration profile under `adapters/profiles/<id>.json`. It can
declare detection, skills, instructions, hooks, memory, exclusion, and trust capabilities
without adding a Go vendor table. Use the portable skills variant unless the host has a
tested optimization.

Put the other parts of an integration in these locations:

- host-specific append-only skill variants go in `adapters/variants/<variant>/`;
- a host-facing hook wrapper goes in `adapters/<profile>/hooks/`;
- protocol translation and registrar implementations go in `internal/install`.

A new integration also needs:

- adapter and render tests asserting that defaults stay under `$HOME`, contain no personal
  path element, and do not leak a host optimization into another integration;
- an update to the generated-checked profile table and evidence notes in
  `docs/agent-integration.md`, with that matrix linked from any new product setup page;
- doctor coverage for missing, current, manual, and unsupported capabilities. Diagnostics
  must stay read-only.

A one-off host needs no engine change: configure the generic profile and its actual paths
in the instance's `config.toml`.

Adding an agent changes nothing under `knowledge/` and does not change `SCHEMA.md`. If your
adapter needs a schema change, say so in the issue first; it is reviewed separately.

## Engine and instance

The engine is this repository. An **instance** is a knowledge base created by `regesto
init` — someone's actual facts. They never share a repository or a history, and the engine
must contain nothing true of only one person or one machine.

The code in `cmd/`, `internal/`, `adapters/` and `bin/` must contain no path, machine
name, user name or project name. Every location comes from the instance's `config.toml`,
with vendor defaults in `internal/adapters`. Examples in docs and tests use the invented
*aurora* and *beacon* projects — please keep it that way rather than reaching for a real
one.

The shipped skills carry a `{{kb_root}}` placeholder rather than a resolved path;
`bin/regesto-install` renders them per instance.

## Releases

Every release publishes what `CHANGELOG.md` says about it. Add a `## <version>` section
before you prepare the release. The local release script reads the notes from that section
and **refuses to build a version with no section**, so the changelog cannot drift behind
the releases.

Write the section for people who use regesto rather than for its maintainers: what
changed, what they have to do about it, and what they can ignore. Do not paste a generated
list of commit subjects as release notes.

Releases are prepared on macOS so the Darwin binaries can use the maintainer's local
Developer ID identity and ASC authentication without copying either credential into CI:

```bash
scripts/release.sh vX.Y.Z
```

That command runs the local gate, builds all four target archives, signs and notarizes the
two macOS binaries, and writes reviewable output under `dist/vX.Y.Z/`. It publishes nothing.
After reviewing those files, publish them explicitly:

```bash
scripts/publish-release.sh vX.Y.Z
```

Publication requires clean `main` to match `origin/main`. It creates the GitHub release and
then starts the tap repository's secret-free updater, which rewrites and install-tests the
Homebrew formula from the public archives. The publish command creates external state; do
not run it speculatively.

## Before you open a pull request

```bash
gofmt -l . && go vet ./... && go test ./...
```

All three must come back clean. Tests live in `tests/` and run against temporary
instances. No test touches a real knowledge base or a real agent's configuration, and new
tests must not either.

If you changed anything about how an instance is scaffolded or installed, also prove it
from nothing:

```bash
go build -o /tmp/regesto ./cmd/regesto
/tmp/regesto init --dir /tmp/kb --examples
/tmp/kb/bin/regesto-install --dry-run
```

This check has caught more real bugs than the unit tests. It is the only check that
exercises the engine and the instance as two separate things.

## Style

Comments explain *why*, not *what*. Write one only when it records the reasoning behind a
non-obvious choice or the failure mode a guard exists for. Match the surrounding code
rather than any general rule.
