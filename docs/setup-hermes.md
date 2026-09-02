# Set up Hermes

Use the main [quick start](../README.md#quick-start) first. This page only covers what is
specific to Hermes. The [integration matrix](agent-integration.md) contains the exact
profile metadata and protocol details.

Hermes can inject Regesto context on the first turn of a session. That behavior becomes
active when both its hook registration and hook allowlist are current.

## Install

Make sure `hermes` is listed in your knowledge base's `config.toml`:

```toml
integrations = ["hermes"]
```

Then preview, install, and verify the integration:

```bash
cd ~/regesto-kb
regesto install --dry-run
regesto install
regesto doctor --integration hermes
```

Installation connects three things:

- Regesto's portable search, write, and promote skills;
- a short knowledge-base section in `~/.hermes/SOUL.md`;
- the `pre_llm_call` hook and its allowlist entry.

If `SOUL.md` does not exist, create it and rerun `regesto install`; Regesto does not
invent host-owned instruction files. Open a new Hermes session after installation.

## Complete a reported manual step

Regesto can create a missing Hermes hook configuration. If `~/.hermes/config.yaml`
already contains other settings, it will not rewrite that YAML because doing so could
lose comments, anchors, ordering, or unfamiliar fields.

In that case, `regesto install` and `regesto doctor --integration hermes` print the exact
YAML entry to merge. Apply that recipe as shown, rerun `regesto doctor`, and confirm the
hook reports current. You do not need to construct or debug the hook command yourself.

The installer manages `~/.hermes/shell-hooks-allowlist.json` separately. It preserves
unrelated approvals and backs up the file before changing it.

## If context does not appear

Run:

```bash
regesto doctor --integration hermes
~/regesto-kb/bin/regesto-context
```

If context exists but the hook is `manual`, complete the printed YAML step. If the hook
or allowlist is stale, rerun `regesto install`. Hermes' hook fails open, so an integration
problem does not block the session; `doctor` is the reliable place to see it.
