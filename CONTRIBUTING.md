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

## Adding an agent

An adapter is a vendor's four locations plus whatever glue it needs, and it should touch
nothing else:

- `internal/adapters/adapters.go` — where the agent keeps skills, instructions and
  settings, as defaults an instance's config can override.
- `adapters/<vendor>/` — a hook if the platform has one.
- A test in `tests/adapters_test.go` asserting the defaults are under `$HOME` and hardcode
  no personal path element.

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
