# Setup — more than one machine

Regesto works on one machine with nothing else installed. This page is only for
replicating a knowledge base across several. It is deliberately short, because Regesto has
no server to configure.

Read [DESIGN.md §9](../DESIGN.md#9-sync-and-transport) for why it is shaped this way. The
short version: **a file-sync client moves the files, and git keeps the history.** Git as a
transport fails open: if a machine misses a pull, its agent reads stale knowledge without
noticing. Sync fails closed: the worst case is a visible conflict file that lint resolves.

## The shape

The knowledge base is a folder. A sync client keeps a **full replica on every machine**,
and every read is a local file read — an agent on a laptop greps the laptop's copy,
offline if need be. No machine ever queries another to consult knowledge.

## What the transport has to do

Regesto has no opinion about how the folder gets from one machine to another. The transport
has to meet three requirements:

1. **Replicate a directory of plain files.** Regesto runs no database, API or daemon of its
   own.
2. **Let you exclude two paths** — `.state/` and `.git`. Both are directories, so per-folder
   exclusion is enough; pattern matching is not needed.
3. **Keep the files real and local.** An agent greps them directly, so anything that stores
   placeholders and fetches on demand will not do.

Any transport that meets these requirements works. Some options:

| | |
|---|---|
| **Continuous peer-to-peer** (Syncthing, Resilio) | No third party holds your knowledge. `regesto init` writes a `.stignore` for exactly this case |
| **A hosted drive** (Dropbox, Google Drive, OneDrive) | Selective sync excludes the two directories. Your knowledge sits in plaintext on someone else's disk — decide that deliberately |
| **Scheduled `rsync` or `unison`** | Total control, no client to run. Prefer `unison`: plain `rsync` in one direction will happily overwrite the newer copy |
| **A mounted share** (NFS, SMB) | Works for reading, writing and editing facts and for `regesto init`, `install` and `upgrade`, even when the share rejects hard links or rename. There, Regesto copies files instead. A copy still never overwrites another writer's file, but it is not atomic: an interrupted write can leave a stub that you delete before retrying. A move whose original cannot be removed deletes its copy again, and reports it if even that fails. There is no replica, so reads stop when the network does, and so does offline access |
| **iCloud Drive** | Not recommended. "Optimize Mac Storage" evicts file contents and leaves stubs, which breaks requirement 3 silently |
| **git** | Deliberately not — [DESIGN §9](../DESIGN.md#9-sync-and-transport). It fails open: miss a pull and the agent confidently reads stale knowledge |

### Conflict naming

When two machines edit the same fact, the client writes a second copy alongside the first.
`regesto cycle` resolves those: the newer `modified` wins in place, the other is archived,
nothing is deleted. Until then the copy is invisible to search, the index and the hook —
an unresolved conflict must not reach a session's context.

Each client names those copies differently, so finding them is the one thing that depends on
your client. The pattern is a config setting:

```toml
[sync]
conflict_pattern = " \(.*conflicted copy.*\)"
```

Config values are taken literally — backslashes are not escape characters here — so write
the expression exactly as a regex, with single backslashes.

The value is a regular expression. Regesto **cuts out** the part it matches to get back to
the original name, so one setting works for clients whose names differ in shape, not merely
in wording. Syncthing appends before the extension:

```
dec-a.sync-conflict-20260729-101500-ABCDEF.md   →   dec-a.md
```

while others bracket the insertion mid-name:

```
dec-a (someone's conflicted copy 2026-07-30).md   →   dec-a.md
```

The default matches Syncthing, so leave it alone if that is what you run. Regesto refuses a
pattern that is not valid regex at startup rather than ignoring it. Otherwise a conflict copy
that stops being recognised would load as a real fact, and two claims would share one id.

## Two things must never sync

`regesto init` writes a `.stignore` covering both, which Syncthing and Resilio read
directly. With any other transport you configure the same two exclusions its own way —
selective sync, an `--exclude` flag, an ignore list. Do not remove either exclusion.

**`.state/`** — each machine diffs its agents' native memory against **its own** baseline
snapshot. Share those snapshots and machines overwrite each other's baselines, and
captures are lost silently, with no error anywhere. This is the single most damaging
misconfiguration available to you.

It also holds the machine's identity and its rendered skills, both of which are local
facts by definition.

**`.git`** — keep the repository on one machine. Syncing it is unenforceable: any agent
session on a second machine runs `git status` and becomes a second writer to the same
index. The knowledge lives in the markdown files; git only adds convenience and offsite
history.

> `.stignore` is **per-device** in Syncthing. Adding a pattern on one machine does not
> protect the others. Set them everywhere, and re-check after adding a device.

## Which machine does what

Every machine runs `regesto harvest` — native memory is local, so nothing else can see it.

**One machine runs `regesto cycle`.** Every machine captures, but only one decides what
becomes a fact. Otherwise two machines mint competing vocabulary for the same claim, and
neither ever detects the contradiction. Name that machine in the instance config:

```toml
[roles]
lint = "workstation"
```

`regesto schedule install` then installs both jobs on that machine and only the harvest job
elsewhere. Machines with no `[roles].lint` entry assume they are the lint machine, which is
correct for the single-machine case.

## Adding a machine

The order matters. The knowledge base arrives through sync; everything else is per machine
and is set up afterwards.

**1. Install the engine.** Each machine needs its own binary — it is per-platform, and
`bin/regesto` is in the ignore files precisely so it never syncs.

```bash
brew install prof18/tap/regesto
```

**2. Set up the sync client** and let the folder replicate fully before touching anything.
Point it at the same directory path you use elsewhere, or a different one — nothing depends
on the path matching.

**Do not run `regesto init`.** That scaffolds a *new* instance; this machine is joining an
existing one. If you run it by mistake, it will keep every file that already exists, so the
damage is limited to files sync had not yet delivered.

**3. Give it a name**, before anything writes a fact from it.

```bash
echo laptop > ~/regesto-kb/.state/machine
regesto --config ~/regesto-kb/config.toml config | head -3
```

The name appears in `inbox/<agent>@<machine>/` and in every fact's `source:`. Left to a
hostname it can drift — see the troubleshooting entry below.

**4. Install the adapters and the jobs.**

```bash
~/regesto-kb/bin/regesto-install
regesto --config ~/regesto-kb/config.toml schedule install
```

`schedule install` gives this machine the harvest job, and the cycle job only if
`[roles].lint` names it or is unset.

The installed jobs get a deterministic `PATH` rather than launchd's system-only default.
That `PATH` includes the normaliser and notifier locations Regesto can resolve during
installation, plus the usual user, Homebrew and system directories. If this machine keeps a
required CLI elsewhere, add `[schedule].extra_path = "~/path/to/bin"` and reinstall the
schedule.

**5. Check it.** Open a session in a project and confirm its facts appear.

```bash
regesto --config ~/regesto-kb/config.toml upgrade --dry-run   # expect: 0 changed
regesto --config ~/regesto-kb/config.toml harvest --dry-run   # first run records a baseline
```

To bring this machine's existing agent memory into the shared knowledge base, run
`harvest --backfill` once. This machine collects the notes, and the lint host processes them
on its next cycle.

Expect some of them to repeat facts other machines already recorded; the normalizer skips
known facts, and `regesto lint` reports near-duplicates. The same applies on a machine that
was already harvesting before `--backfill` existed: the import can include notes an earlier
harvest collected.

**Home directories differ between machines**, and Regesto handles that: everything resolves
relative to `config.toml`'s location, and the instructions section renders `~/`-relative
paths rather than absolute ones. Do not hardcode a path anywhere yourself.

## Upgrading, with more than one machine

`.regesto-manifest` records which engine last wrote the instance's shared files, and it
replicates like everything else — so **upgrade on one machine, and update the engine on the
others before they run it**. `regesto upgrade` prints `engine <from> → <to>` before it does
anything; if that reads like a downgrade, it is, and you should stop.

The lint host is the natural place to do it. Every machine still needs its own binary and
its own `bin/regesto-install` run afterwards, because rendered skills live in `.state/`,
which never syncs.

## An always-on third node

A NAS or small server running the same sync client gives you a replica that is always up
and a backup target. It runs nothing else: no regesto, no agents and no scheduled jobs. Its
only job is to hold a third copy.

Filesystem snapshots there protect against a bad sync propagating. They do not protect
against the building burning down, so if every replica lives in one place, add an offsite
git remote and let `regesto cycle --push` use it. Be deliberate: that remote holds your
accumulated personal knowledge in plaintext.

---

## Troubleshooting

### The first harvest on a new machine captures nothing

This is by design. The first harvest records a baseline instead, so you do not get that
machine's entire existing memory store dumped into the inbox at once. Captures start from
the *next* change. Run
`regesto harvest --backfill` once if you do want that existing memory. A second backfill
imports nothing again; it only collects changes, like an ordinary harvest.

### Conflict files

When two machines edit one fact, the sync client writes a copy such as
`<name>.sync-conflict-<timestamp>.md`. `regesto cycle` resolves these: it keeps the copy
with the newer `modified` and moves the loser into `archive/inbox/<date>/`. Nothing is
deleted, so a wrong call is recoverable.

If you resolve one by hand, follow the same rule, and never delete the loser.

### `regesto cycle` refuses to run on a machine

This is expected if that machine has no `.git`. That is deliberate, because git lives on
one machine only. Either it is not the lint host (check `[roles].lint`), or you are on the
wrong machine.

### Facts appear twice with slightly different subjects

Two machines normalised the same capture independently before either had seen the other's.
`regesto lint` reports near-duplicate `(subject, relation)` pairs for you to merge. If it
keeps happening, `[roles].lint` is unset or names a machine that is not running the job —
check `regesto schedule status` there.

### A machine kept using its old engine after you installed a new one

Two leftovers take precedence over a freshly installed engine. Neither reports itself, and
everything keeps working on the old binary:

- `<instance>/bin/regesto`, which the shims check **before** PATH. It does not sync, so a
  machine that once built its own engine still has it.
- `~/.local/bin/regesto`, the symlink `regesto-install` creates, which sits earlier in PATH
  than Homebrew.

Delete both, then run `bin/regesto-install` and `regesto schedule install`. Or run
`regesto upgrade`, which does all of that. `regesto version` reports whichever binary won, so
compare it against what you installed.

### The machine name keeps changing

macOS increments a counter on the hostname when rejoining networks. Regesto strips the
suffix, but if `regesto config` reports `machine_source=hostname`, the name is still a
guess. The machine name appears in `inbox/<agent>@<machine>/` and in every fact's
`source:`, so pin it in `.state/machine`, which is per-machine and never synced.
