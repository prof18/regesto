# Changelog

What changed, for the people using it. Each release's section is what the release
publishes as its notes — `release.yml` reads it from here and refuses to publish a tag
that has no section, so this file cannot fall behind.

## 0.2.1

**Skills pointed agents at a binary that was not there.** The `regesto-write` skill told
agents to run `<instance>/bin/regesto` to look up a fact's scope and the machine's name.
That path only exists where the instance is also an engine checkout — so wherever regesto
was installed from a release, agents could not run it and fell back to guessing those two
values. A guessed scope files a fact where the session hook will never find it; a guessed
source crosses the human/agent trust boundary. Neither shows up as an error.

Fixed by giving the skills the stable entry points the other subcommands already had:

- **New:** `bin/regesto-project` and `bin/regesto-config`. Existing instances get them on
  their next `regesto upgrade`.

**Your sync client's conflict naming is now a setting.** `regesto cycle` resolves the copies
a sync client leaves when two machines edit one fact. It only recognised Syncthing's naming;
now it takes a pattern, so a knowledge base replicated some other way still gets its
conflicts resolved automatically.

```toml
[sync]
conflict_pattern = " \(.*conflicted copy.*\)"
```

The default is unchanged, so Syncthing users need do nothing.

## 0.2.0

**`regesto upgrade` finishes the job.** It used to refresh the files inside an instance and
then tell you to run `bin/regesto-install` yourself. Since agents load *rendered* copies of
the skills, an upgrade had until then changed nothing any agent could see — a new skill in a
release sat in the instance, invisible, and the symptom was a feature appearing not to
exist.

It now re-renders the skills, relinks them into every agent, refreshes the instructions
section and the hook, and repoints the scheduled jobs when they name an engine that no
longer serves the instance.

**Files a release retires are removed** — but only where the copy on disk is byte for byte
what the engine recorded writing. Edit one and it becomes yours: kept, reported, and no
longer tracked. Anything with no recorded hash is never touched. `--force` removes an edited
one, backing it up beside itself first.

Note the ordering: this behaviour lives in the binary, so `regesto upgrade` on an older
engine still behaves the old way. Update the binary first, then upgrade the instance.

## 0.1.1

First release with downloadable binaries, for macOS and Linux on both architectures, plus
`checksums.txt`. The engine is identical to 0.1.0.

## 0.1.0

First release. `regesto` with `search`, `index`, `context`, `config`, `project`, `harvest`,
`normalize`, `lint`, `promote`, `cycle`, `schedule`, `init` and `upgrade`.

Consultation is hook-enforced on Claude Code and skill-driven elsewhere. Facts are plain
markdown, one claim per file, and nothing is ever deleted.

Installable with `go install` only — the release carries no binaries, because the workflow
that builds them failed after tagging. Use 0.1.1 or later.
