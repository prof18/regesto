# Set up Claude Code

Use the main [quick start](../README.md#quick-start) first. This page only covers what is
specific to Claude Code. The [integration matrix](agent-integration.md) contains the exact
profile metadata and protocol details.

Claude Code is Regesto's most automatic integration: a `SessionStart` hook injects
relevant knowledge before the first prompt.

## Install

Make sure `claude` is listed in your knowledge base's `config.toml`:

```toml
integrations = ["claude"]
```

Then preview, install, and verify the integration:

```bash
cd ~/regesto-kb
regesto install --dry-run
regesto install
regesto doctor --integration claude
```

Installation connects three things:

- Regesto's search, write, and promote skills;
- a short knowledge-base section in `~/.claude/CLAUDE.md`;
- the session-start hook in `~/.claude/settings.json`.

Regesto preserves unrelated content and backs up host files before editing them. If
`CLAUDE.md` does not exist, create it and run `regesto install` again; Regesto does not
invent host-owned instruction files.

Open a new Claude Code session after installation.

## Verify it

Run the same context lookup used by the hook:

```bash
~/regesto-kb/bin/regesto-context --dir ~/work/aurora
```

If the project has facts, the output contains its canonical project name and matching
entries. `regesto doctor --integration claude` should report the skills, instructions,
and hook as current.

## Use non-default Claude paths

The built-in defaults are normally enough. Override them only when your Claude files live
somewhere else:

```toml
[integrations.claude]
skills_dir = "~/.agents/skills"
instructions_file = "~/.dotfiles/CLAUDE.md"
settings_file = "~/.claude/settings.json"
```

Run `regesto config` to see the resolved paths, then rerun the install commands.

## If context does not appear

Check these in order:

```bash
regesto doctor --integration claude
~/regesto-kb/bin/regesto-context
python3 -m json.tool ~/.claude/settings.json
```

The hook deliberately fails open so a Regesto problem never blocks Claude from starting.
That also means a broken hook can be silent. `doctor` shows the missing or stale artifact;
`regesto-context` confirms whether Regesto itself can resolve facts; the final command
checks that Claude's settings file is valid JSON.

Two normal cases are easy to misread as failures:

- The first `regesto harvest` only records a baseline. It captures changes from later
  runs.
- If two clones resolve to different project names, add the desired mapping under
  `[projects]` in `config.toml`, then run `regesto project --scope` inside the repository
  to verify it.
