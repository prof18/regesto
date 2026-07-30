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

Any client that replicates a directory works. Syncthing is the obvious choice (no third
party holds your knowledge) but nothing here depends on it.

## Two things must never sync

Both are already in the `.stignore` that `regesto init` writes. Do not tidy either away.

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

## Per machine

Each machine needs its own engine binary, its own identity, and its own adapters installed:

```bash
echo laptop > ~/regesto-kb/.state/machine   # or let it derive one, and check `regesto config`
~/regesto-kb/bin/regesto-install
regesto schedule install
```

The knowledge base itself arrives through sync — do not run `regesto init` a second time
against a folder that is already replicating.

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

### The machine name keeps changing

macOS increments a counter on the hostname when rejoining networks. Regesto strips the
suffix, but if `regesto config` reports `machine_source=hostname` you are relying on a
guess — and the machine name is in `inbox/<agent>@<machine>/` and in every fact's
`source:`. Pin it in `.state/machine`, which is per-machine and never synced.
