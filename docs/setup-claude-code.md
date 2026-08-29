# Setup — Claude Code

Claude Code is the only supported agent where consultation is **guaranteed** rather than
likely, because it exposes a hook that injects text into the model's context before the
first prompt. The hook does the lookup; the model does not get a choice.

## Install

```bash
regesto init --dir ~/regesto-kb --examples
~/regesto-kb/bin/regesto-install
```

`regesto-install` is a compatibility shim over `regesto install`. Installation is
idempotent, backs up every host file it edits, and takes `--dry-run` (plus `--json` for a
versioned machine-readable plan). The Go installer does three things:

1. **Renders and links the skills** into `~/.claude/skills/`: `regesto-search`,
   `regesto-write`, `regesto-promote`.
2. **Registers the `SessionStart` hook** in `~/.claude/settings.json`, appending to any
   hooks already there.
3. **Appends the knowledge-base section** to `~/.claude/CLAUDE.md`, between
   `regesto:section:start` / `:end` markers so it can be updated later without touching
   the rest of the file.

In an engine source checkout, the compatibility shim also builds the real binary before a
non-dry install. Release-backed instances already have an engine and simply forward to it.

Then open a session in a project. Its facts should be in context before you type anything.

## What you should see

```
$ ~/regesto-kb/bin/regesto-context --dir ~/work/aurora
# Knowledge base
...
## Project: aurora (3)
- `dec-http-port-8080` — The API listens on 8080 in development
```

That exact text is what the hook feeds Claude Code at session start.

## Configuration

Everything lives in your instance's `config.toml`; the engine hardcodes nothing.

```toml
agents = ["claude"]

# Only if this machine differs from the vendor defaults.
[skills_dirs]
claude = "~/.agents/skills"
[instructions]
claude = "~/.dotfiles/AGENTS.md"
[settings_files]
claude = "~/.claude/settings.json"
```

`regesto config` prints what these resolve to. If something installed in the wrong place,
look there first.

---

## Troubleshooting

These are the things that actually went wrong while building this, not a list of things
that theoretically could.

### The session starts but no facts appear

Check, in order:

```bash
~/regesto-kb/bin/regesto-config         # is the instance the one you think it is?
~/regesto-kb/bin/regesto-context        # does it print anything at all?
python3 -m json.tool ~/.claude/settings.json
```

The hook is written to **never fail a session start**: if anything goes wrong it exits 0
and injects nothing. That is deliberate — a non-zero exit would surface as an error on
every single session — but it does mean a broken hook is silent. Run
`bin/regesto-context` by hand to see the error it is swallowing.

To prove stdout really lands in context, put a sentinel string in the hook's output and ask
the model to repeat it, in an isolated session that does not touch your global config:

```bash
claude -p --settings /tmp/probe-settings.json 'repeat the sentinel you were given'
```

### Settings JSON is invalid

The installer parses and merges `settings.json` in Go; it has no `jq` dependency. If the
file is malformed, planning fails before making any change. Repair the JSON and re-run
`regesto install --dry-run` to inspect the exact canonical target and backup action.

### Facts land under two different project names

Claude Code's native memory directory is derived from the **absolute repo path**, so the
same project fragments across clones and across machines. Regesto resolves the canonical
name from the git remote's basename instead, which every clone shares.

For a repo with no remote, or two different projects whose basenames collide, map them in
the instance config:

```toml
[projects]
"aurora-fork" = "aurora"
```

`regesto project --scope` run inside the repo prints what the hook will use. `regesto
cycle` also moves already-filed facts onto the canonical name when you add a mapping —
names that merely *resemble* each other are left alone, because `beacon` and
`beacon-mobile` are two different projects.

### The machine name keeps changing

macOS appends and increments a counter to the hostname when rejoining networks — the same
box comes back as `-2`, then `-3`. Regesto strips the suffix, but if `regesto config` says
`machine_source=hostname` you are relying on a guess, and the machine name appears in
`inbox/<agent>@<machine>/` and in every fact's `source:`.

Pin it:

```bash
echo laptop > ~/regesto-kb/.state/machine
```

`.state/` is per-machine, never committed and never synced.

### I edited a skill and nothing changed

Shipped skills carry a `{{kb_root}}` placeholder, so they are rendered into
`.state/skills/` and linked from there. Re-run `bin/regesto-install` after editing one.
The same applies to the instructions section: install compares the installed copy against
the template and updates it when it has drifted, because a stale always-loaded section
actively misdirects every session.

### Something else already owns a skill link

If `~/.claude/skills/regesto-search` exists and points somewhere other than this instance,
install warns and leaves it alone rather than stealing it. That is what you want when two
instances are on one machine; delete the link yourself if it is stale.

### Harvest captured nothing

`regesto harvest` diffs each agent's memory files against a snapshot in `.state/`. The
first run takes the baseline, so it legitimately reports nothing. After that, an empty run
means the agent has not written since the last one.

The window matters: native memory is a cache the agent prunes, so a note written **and
pruned** between two harvest runs never appears in any diff. Run harvest every few minutes
(`regesto schedule install` does), and treat the `regesto-write` skill as the real
guarantee for anything you care about.

### Everything harvested since the last run vanished

Something rewrote a memory file before harvest diffed it. Nothing in regesto does this —
memory files hold a pointer, not content — but if you ever install or re-assert the pointer
block by hand, harvest first.
