# Contributing

Bug reports, adapters for new agents and documentation fixes are all welcome. A few things
are worth knowing before you start.

## The one rule: schema changes require a migration note

`SCHEMA.md` is a contract with data that already exists on other people's disks. Every
instance out there holds facts written against some revision of it, and nobody is going to
hand-edit a thousand markdown files.

So a pull request that changes `SCHEMA.md` must also:

1. **Bump `schema_version`** and say what revision the change lands in.
2. **Add a migration note** — a short section stating what changed, what an older file
   looks like, and what it should become. Mechanical if at all possible: "field `x` is now
   required, default it to `y`" is a migration; "rethink your subjects" is not.
3. **Say what old files do in the meantime.** Older facts must keep parsing and keep being
   searchable. Breaking reads is not an option; deprecating a field over a revision is.

The same applies to renaming a field, changing what a value means, or tightening a
validation rule. Adding an optional field is the one case that needs only the version bump.

If your change cannot be migrated mechanically, it probably belongs in the instance's own
vocabulary rather than in the schema.

The same reasoning covers `adapters/`, `bin/` and `SCHEMA.md` generally: those files are
copied *into* people's knowledge bases, and `regesto upgrade` will offer to replace them.
It refuses to touch any copy that differs from what the engine recorded writing, so a
change of yours reaches an instance only where the user never edited that file. Write them
as if someone has forked their copy, because someone has.

## Adding an agent

Prefer a declarative integration profile under `adapters/profiles/<id>.json`; it can
declare skills, instructions, settings, hooks, and memory capabilities without adding a
Go vendor table. Use the portable skills variant unless the host has a tested optimization.
Host-specific append-only skill variants live under `adapters/variants/<variant>/`, and a
host-facing hook wrapper belongs under `adapters/<profile>/hooks/` while protocol
translation and registrar implementations live in `internal/install`. Add adapter and
render tests asserting that defaults remain under `$HOME`, contain no personal path
element, and do not leak a host optimization into another integration.

A one-off host needs no engine change: configure the generic profile and its actual paths
in the instance's `config.toml`.

Nothing under `knowledge/` changes, and neither does `SCHEMA.md`. If your adapter needs a
schema change, say so in the issue first — that is a different conversation.

## Engine and instance

The engine is this repository. An **instance** is a knowledge base created by `regesto
init` — someone's actual facts. They never share a repository or a history, and the engine
must contain nothing true of only one person or one machine.

Concretely: no path, machine name, user name or project name in `cmd/`, `internal/`,
`adapters/` or `bin/`. Every location comes from the instance's `config.toml`, with vendor
defaults in `internal/adapters`. Examples in docs and tests use the invented *aurora* and
*beacon* projects — please keep it that way rather than reaching for a real one.

The shipped skills carry a `{{kb_root}}` placeholder rather than a resolved path;
`bin/regesto-install` renders them per instance.

## Releases

Every release publishes what `CHANGELOG.md` says about it. Add a `## <version>` section
before tagging — the workflow reads it from there and **refuses to publish a tag with no
section**, so the changelog cannot drift behind the releases.

Write it for someone using regesto, not for someone maintaining it: what changed, what they
have to do about it, and what they can ignore. A generated list of commit subjects is not
release notes.

## Before you open a pull request

```bash
gofmt -l . && go vet ./... && go test ./...
```

All three clean. Tests live in `tests/` and run against temporary instances — no test
touches a real knowledge base or a real agent's configuration, and none should.

If you changed anything about how an instance is scaffolded or installed, also prove it
from nothing:

```bash
go build -o /tmp/regesto ./cmd/regesto
/tmp/regesto init --dir /tmp/kb --examples
/tmp/kb/bin/regesto-install --dry-run
```

That has caught more real bugs than the unit tests have — it is the only check that
exercises the engine and the instance as two separate things.

## Style

Comments explain *why*, not *what*, and they earn their place: the reasoning behind a
non-obvious choice, the failure mode a guard exists for. Match the surrounding code rather
than any general rule.
