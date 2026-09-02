# Set up Codex

Use the main [quick start](../README.md#quick-start) first. This page only covers what is
specific to Codex. The [integration matrix](agent-integration.md) contains the exact
profile metadata and capability details.

Codex has no supported startup hook for injecting Regesto context. Its integration uses
persistent instructions to tell Codex when to consult the knowledge base and portable
skills to provide the search and write procedures.

## Install

Make sure `codex` is listed in your knowledge base's `config.toml`:

```toml
integrations = ["codex"]
```

Then preview, install, and verify the integration:

```bash
cd ~/regesto-kb
regesto --config ~/regesto-kb/config.toml install --dry-run
regesto --config ~/regesto-kb/config.toml install
regesto doctor --integration codex
```

Installation links Regesto's skills into `~/.codex/skills/` and adds a marked
knowledge-base section to `~/.codex/AGENTS.md`. It does not register a hook.

If `AGENTS.md` does not exist, create it and rerun `regesto install`; Regesto does not
invent host-owned instruction files. Open a new Codex session after installation.

## What to expect

Codex should search Regesto when a recorded decision, preference, convention, or
environment fact could settle the task. It should use the write skill when you explicitly
ask it to remember something.

This behavior is instruction-driven, not enforced. If a session overlooks relevant
knowledge, ask it to “check Regesto first.”

## Use a non-default skills directory

Override the built-in path only when Codex stores skills somewhere else:

```toml
[integrations.codex]
skills_dir = "~/.my-codex/skills"
```

Inspect the resolved configuration and installation plan:

```bash
regesto --config ~/regesto-kb/config.toml config
regesto --config ~/regesto-kb/config.toml install --dry-run
```

## If the skills or instructions are missing

```bash
regesto doctor --integration codex
ls -l ~/.codex/skills/
```

`doctor` reports the canonical target and remediation. Rerun `regesto install` after
creating a missing instructions file or changing a path.

Codex's native memory is global rather than tied to one repository. If a harvested fact
gets the wrong project scope, correct the fact's `scope:` field and run `regesto index`.
