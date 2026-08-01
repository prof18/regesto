# Setup — more than one machine

Regesto works on one machine with nothing else installed. This page is only for
replicating a knowledge base across several, and it is deliberately short: there is no
server to configure, because there is no server.

Read [DESIGN.md §9](../DESIGN.md#9-sync-and-transport) for why it is shaped this way. The
short version: **a file-sync client for transport, git for history.** Git-as-transport
fails open — miss a pull and the agent confidently reads stale knowledge. Sync fails
closed: the worst case is a visible conflict file that lint resolves.

## The shape

The knowledge base is a folder. A sync client keeps a **full replica on every machine**,
and every read is a local file read — an agent on a laptop greps the laptop's copy,
offline if need be. No machine ever queries another to consult knowledge.

## What the transport has to do

Regesto has no opinion about how the folder gets from one machine to another. The contract
is three lines long:

1. **Replicate a directory of plain files.** No database, no API, no daemon of ours.
2. **Let you exclude two paths** — `.state/` and `.git`. Both are directories, so per-folder
   exclusion is enough; pattern matching is not needed.
3. **Keep the files real and local.** An agent greps them directly, so anything that stores
   placeholders and fetches on demand will not do.

Anything meeting that works. Some shapes it can take:

| | |
|---|---|
| **Continuous peer-to-peer** (Syncthing, Resilio) | No third party holds your knowledge. `regesto init` writes a `.stignore` for exactly this case |
| **A hosted drive** (Dropbox, Google Drive, OneDrive) | Selective sync excludes the two directories. Your knowledge sits in plaintext on someone else's disk — decide that deliberately |
| **Scheduled `rsync` or `unison`** | Total control, no client to run. Prefer `unison`: plain `rsync` in one direction will happily overwrite the newer copy |
| **A mounted share** (NFS, SMB) | Works, but there is no replica — reads stop when the network does, and the offline-on-a-train property goes with it |
| **iCloud Drive** | Not recommended. "Optimize Mac Storage" evicts file contents and leaves stubs, which breaks requirement 3 silently |
| **git** | Deliberately not — [DESIGN §9](../DESIGN.md#9-sync-and-transport). It fails open: miss a pull and the agent confidently reads stale knowledge |

### Conflict naming

When two machines edit the same fact, the client writes a second copy alongside the first.
`regesto cycle` resolves those: the newer `modified` wins in place, the other is archived,
nothing is deleted. Until then the copy is invisible to search, the index and the hook —
an unresolved conflict must not reach a session's context.

Finding those copies is the one thing that depends on your client, because each names them
differently. It is a setting, not a constant:

```toml
[sync]
conflict_pattern = " \(.*conflicted copy.*\)"
```

Config values are taken literally — backslashes are not escape characters here — so write
the expression exactly as a regex, with single backslashes.

A regular expression, and the part it matches is **cut out** of the name to get back to the
original — which is what makes it work across clients whose shapes differ, not merely their
wording. Syncthing appends before the extension:

```
dec-a.sync-conflict-20260729-101500-ABCDEF.md   →   dec-a.md
```

while others bracket the insertion mid-name:

```
dec-a (someone's conflicted copy 2026-07-30).md   →   dec-a.md
```

The default matches Syncthing, so leave it alone if that is what you run. A pattern that is
not valid regex is refused at startup rather than quietly ignored: a conflict copy that
stops being recognised would load as a real fact, and two claims would share one id.

## Two things must never sync

`regesto init` writes a `.stignore` covering both, which Syncthing and Resilio read
directly. With any other transport you configure the same two exclusions its own way —
selective sync, an `--exclude` flag, an ignore list. Do not tidy either away.

**`.state/`** — each machine diffs its agents' native memory against **its own** baseline
snapshot. Share those snapshots and machines overwrite each other's baselines, and
captures are lost silently, with no error anywhere. This is the single most damaging
misconfiguration available to you.

It also holds the machine's identity and its rendered skills, both of which are local
facts by definition.

**`.git`** — keep the repository on one machine. Syncing it is unenforceable: any agent
session on a second machine runs `git status` and becomes a second writer to the same
index. The markdown is the knowledge; git is convenience and offsite history.

> `.stignore` is **per-device** in Syncthing. Adding a pattern on one machine does not
> protect the others. Set them everywhere, and re-check after adding a device.

## Which machine does what

Every machine runs `regesto harvest` — native memory is local, so nothing else can see it.

**One machine runs `regesto cycle`.** Capture happens everywhere; deciding what becomes a
fact happens in one place, or two machines mint competing vocabulary for the same claim and
neither ever detects the contradiction. Name it in the instance config:

```toml
[roles]
lint = "workstation"
```

`regesto schedule install` then installs both jobs on that machine and only the harvest job
elsewhere. Machines with no `[roles].lint` entry assume they are it, which is correct for
the single-machine case.

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

**5. Check it.** Open a session in a project and confirm its facts appear.

```bash
regesto --config ~/regesto-kb/config.toml upgrade --dry-run   # expect: 0 changed
regesto --config ~/regesto-kb/config.toml harvest --dry-run   # first run records a baseline
```

**Home directories differ between machines**, and that is handled: everything resolves
relative to `config.toml`'s location, and the instructions section renders `~/`-relative
paths rather than absolute ones. Do not hardcode a path anywhere yourself.

**Home directories differ between machines**, and that is handled: everything resolves
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
and a backup target. It runs nothing else — no regesto, no agents, no scheduled jobs. It is
a third copy, and that is the whole job.

Filesystem snapshots there protect against a bad sync propagating. They do not protect
against the building burning down, so if every replica lives in one place, add an offsite
git remote and let `regesto cycle --push` use it. Be deliberate: that remote holds your
accumulated personal knowledge in plaintext.

---

## Troubleshooting

### The first harvest on a new machine captures nothing

By design. It records a baseline instead, so you do not get that machine's entire existing
memory store dumped into the inbox at once. Captures start from the *next* change.

### Conflict files

Two machines editing one fact produces `<name>.sync-conflict-<timestamp>.md`. `regesto
cycle` resolves these: it keeps the copy with the newer `modified` and moves the loser into
`archive/inbox/<date>/`. Nothing is deleted, so a wrong call is recoverable.

If you resolve one by hand, follow the same rule, and never delete the loser.

### `regesto cycle` refuses to run on a machine

Expected if that machine has no `.git` — which is deliberate, since git lives on one
machine only. Either it is not the lint host (check `[roles].lint`), or you are on the
wrong machine.

### Facts appear twice with slightly different subjects

Two machines normalised the same capture independently before either had seen the other's.
`regesto lint` reports near-duplicate `(subject, relation)` pairs for you to merge. If it
keeps happening, `[roles].lint` is unset or names a machine that is not running the job —
check `regesto schedule status` there.

### A machine kept using its old engine after you installed a new one

Two leftovers shadow a freshly installed engine, and neither announces itself — everything
keeps working, on the old one:

- `<instance>/bin/regesto`, which the shims check **before** PATH. It does not sync, so a
  machine that once built its own engine still has it.
- `~/.local/bin/regesto`, the symlink `regesto-install` creates, which sits earlier in PATH
  than Homebrew.

Delete both, then `bin/regesto-install` and `regesto schedule install` — or just
`regesto upgrade`, which does all of it. `regesto version` reports whichever binary won, so
compare it against what you installed.

### The machine name keeps changing

macOS increments a counter on the hostname when rejoining networks. Regesto strips the
suffix, but if `regesto config` reports `machine_source=hostname` you are relying on a
guess — and the machine name is in `inbox/<agent>@<machine>/` and in every fact's
`source:`. Pin it in `.state/machine`, which is per-machine and never synced.
