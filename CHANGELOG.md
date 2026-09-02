# Changelog

What changed, for the people using it. Each release's section is what the release
publishes as its notes — `release.yml` reads it from here and refuses to publish a tag
that has no section, so this file cannot fall behind.

## 0.4.0

**Integrations are now capability-driven instead of hardcoded to Claude Code and
Codex.** A declarative profile describes detection, skills, instructions, hooks,
native-memory sources, and default trust. Built-in profiles cover Claude Code,
Codex, Hermes, and a generic local integration; additional local profiles can be
added without changing the engine. Configuration now uses `integrations = [...]` and
per-integration override sections; old single-user instances are updated manually rather
than carrying an automatic migration layer.

`regesto install` now plans and applies those profiles through the Go engine. Its
versioned JSON dry-run reports canonical targets, ownership, current and intended
state, backups, and the exact action before anything is written. Shared skill or
instruction targets are reconciled once, generated artifacts carry ownership
markers, hooks are registered through declared protocols, and symlink or path
escapes are refused.

**Every integration gets portable Regesto skills.** Claude keeps its tailored
variant while Codex, Hermes, and generic integrations receive the same search,
write, and promote workflows through stable instance shims. Instruction blocks
use bounded markers, preserve host-owned text, and can be installed into more than
one declared target without duplicating content.

Native Markdown memory can now be harvested from every declared source with a
separate persisted baseline. Overlapping declarations deduplicate publication,
untrusted sources remain quarantined, and all reads
and writes stay within descriptor-verified roots. No proprietary cloud-memory
connector or network transport was added.

**A local stdio MCP server exposes Regesto resources and tools.** `regesto mcp`
implements the 2025-06-18 initialize lifecycle, tools, resources, bounded JSON-RPC
framing, and clean EOF shutdown using the same validation and trust rules as the
CLI. It listens on no socket and writes only protocol messages to stdout.

`regesto doctor [--integration ID] [--json]` provides deterministic, read-only
diagnostics for detection, installed artifacts, capabilities, memory, and trust,
with concrete remediation. The documentation now includes a profile-derived
capability matrix and setup recipes for Claude Code, Codex, Hermes, generic local
tools, and MCP clients.

The release gate now builds the engine away from its knowledge-base instance and
drives init, inert install dry-run, install, doctor, lint, upgrade, and an MCP
handshake inside a disposable HOME.

## 0.3.1

**Scheduled cycles can now find normalisers installed outside macOS's system paths.**
LaunchAgents do not read shell startup files and launchd gives them only
`/usr/bin:/bin:/usr/sbin:/sbin` by default. A normal terminal could therefore run
`claude` and `codex` while the hourly cycle could find neither—common on Apple Silicon,
where Homebrew installs them under `/opt/homebrew/bin`. Harvesting and linting kept
running, but raw captures stayed in the inbox instead of becoming facts.

`regesto schedule install` now gives both jobs a deterministic `PATH`. It includes the
directories of the configured normaliser and notifier commands found during installation,
then stable user, Homebrew and system locations. It deliberately does not copy the entire
interactive `PATH`, which can contain temporary agent-session or version-manager directories
that disappear while the LaunchAgent keeps running. The same environment also covers
interpreters such as `node`; hardcoding only the path to `claude` or `codex` would not.

Installations with an uncommon tool location can add it without replacing the safe defaults:

```toml
[schedule]
extra_path = "~/.local/share/mise/shims"
```

New schedules get the fix automatically. After upgrading an existing installation, run
`regesto --config /path/to/regesto-kb/config.toml schedule install` once to rewrite and
reload its current jobs. No fact format or schema changed.

## 0.3.0

**The cycle now tells you when it has stopped working.** It aborts on the first validation
error and commits nothing, which is the right call — applying half a reconciliation is worse
than applying none. But it runs unattended, so until now the only trace of a failure was an
exit code and a line in `.state/<machine>/logs/`, and nothing about the knowledge base looks
wrong from the outside while it is broken. Meanwhile every hour adds facts that are written
but never committed, reconciliations that never apply, and an `INDEX.md` that agents keep
reading as current.

A failing cycle now sends a notification naming the file that broke it, and another when it
recovers. While it stays broken you get one reminder a day, not one an hour — an alert that
fires every hour is an alert you mute, and a muted channel loses the next real failure too.
A working pass says nothing at all.

macOS uses `osascript` and Linux `notify-send`, so this works without installing anything.
To send alerts somewhere else — a phone, a chat channel — point `[notify].command` at any
program and it receives the title and message as its last two arguments, plus
`$REGESTO_NOTIFY_TITLE`, `$REGESTO_NOTIFY_MESSAGE`, `$REGESTO_NOTIFY_STATE` and
`$REGESTO_NOTIFY_KEY` in its environment:

```toml
[notify]
on = "off"                     # default is on wherever a notifier exists
command = "~/bin/my-notifier"  # receives: <title> <message>
renag_hours = "24"             # 0 to report each transition once and never nag
```

**`regesto schedule status` now reports whether the cycle is working, not just whether it is
installed.** It prints how long ago the last clean pass was, or how long it has been failing.
This is the half the cycle cannot report on itself: a job that never fires — unloaded, or
holding a path to an engine that has moved — never reports a failure either, and a stale
last-clean-pass is the only evidence of it. It also warns when notifications are turned off,
so a silent instance is never silently silent.

**The failure message now names a file.** `regesto cycle` used to end with `2 validation
error(s); nothing applied, nothing committed`; it now appends the first error and the path
it came from. A count alone cannot be acted on from a log line or a notification.

**A quarantined capture now says why it was quarantined.** The note read `quarantined —
reachable by third parties`, which states a conclusion the engine has not reached: all it
knows is that the source is not listed in `[trusted_sources]`. Whether the channel really is
reachable by anyone else is a question only you can answer, and asserting it sends people
looking for a breach instead of at the one line of config that resolves it. It now names the
source and the setting: `quarantined — hermes@studio is not in [trusted_sources], so its
captures are left raw for a human to promote`.

Nothing about the file format changed, and no existing configuration becomes invalid.
Notifications are on by default — if you would rather they were not, set `[notify].on =
"off"`. `regesto upgrade` refreshes the affected files as usual.

## 0.2.3

**The instructions agents load described a knowledge base for code.** Every surface that
tells an agent what belongs here framed the subject as architecture, conventions and
tooling: the section installed into `CLAUDE.md` / `AGENTS.md`, the `SessionStart` payload,
the `regesto-search` and `regesto-write` skills, `SCHEMA.md`, and the prompts that extract
facts from native memory and from transcripts.

An agent reading that literally never searches for a tax record, a device inventory or a
travel claim, and never files one either — however plainly the knowledge base itself says
those belong. The payload was the clearest case: it printed "consult it before decisions
about architecture, conventions, preferences, or past work" directly above a list of facts
about residency and medical history.

All of it now says what it always meant. Consult the knowledge base before any answer or
decision a recorded claim could settle, technical or not. Record decisions and why,
conventions, repeated corrections, preferences, and non-obvious facts about the
environment, the tooling, or your own life and admin.

The bar for recording is unchanged — durable, non-obvious, costly to rediscover. What
changed is that "is this about code?" is no longer part of it. The old "don't record
anything derivable from reading the code" test became "derivable from a source already at
hand", which is a question you can actually answer about a fact that has no code.

Nothing about behaviour, configuration or the file format changed. `regesto upgrade`
refreshes the affected files and reinstalls the adapters in the same run, so the new text
reaches every agent without a separate step.

## 0.2.2

**`regesto init` no longer hardcodes `agents = ["claude", "codex"]`.** It now detects which
known agents are actually installed on this machine — the same check `bin/regesto-install`
already made before installing — and writes only those. A machine with just Claude Code
gets `agents = ["claude"]`; one with neither gets an empty list and a printed reason,
instead of a config file asserting two agents that were never there.

`regesto upgrade` uses the same detection to flag an agent that showed up on the machine
after the instance was created but is still missing from `agents` — a note only, since
config.toml is the one file the engine never writes to on its own.

**Several install-flow docs bugs, found by running the README against a real, empty
`$HOME`.** Most subcommands beyond the six with a `bin/regesto-*` shim resolve their
instance from the working directory or from beside the engine binary — neither of which
helps once the engine is installed via Homebrew and invoked from an arbitrary shell. The
Quickstart and setup docs now say so, and their examples `cd` into the instance first. Also
fixed: the hourly scheduled job was called "lint" in one place when it actually runs
`cycle`; there was no Uninstalling section; and `regesto-kb` now reads as this guide's
suggested folder name, not a required one.

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
