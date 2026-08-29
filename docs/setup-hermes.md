# Setup — Hermes Agent

Hermes consultation is **hook-enforced on the first turn**. Regesto registers a
`pre_llm_call` hook that returns `{"context":"..."}` once per session; later calls return
the host-valid no-op object `{}`.

## Install

Include `hermes` in the instance configuration, then inspect and apply the plan:

```toml
agents = ["hermes"]
```

```bash
regesto --config ~/regesto-kb/config.toml install --dry-run
regesto --config ~/regesto-kb/config.toml install
```

For a missing `~/.hermes/config.yaml`, the installer can safely create the minimal hook
configuration. If that YAML file already contains anything else, Regesto does not parse
and rewrite it: the plan reports a manual step with the exact command to merge. This
preserves comments, anchors, aliases, tags, ordering, and unfamiliar settings.

The requested YAML has this shape, with the command replaced by the absolute path printed
by the install plan:

```yaml
hooks:
  pre_llm_call:
    - command: "'/absolute/path/to/regesto-kb/adapters/hermes/hooks/pre-llm.sh'"
      timeout: 10
```

The inner single quotes are part of the command, not decoration. Keep the exact quoted
command printed by the installer so Hermes' shell-style argument parser treats an
instance path containing spaces or metacharacters as one executable path.

Hermes also requires the exact event and command in
`~/.hermes/shell-hooks-allowlist.json`. The installer validates and merges that JSON,
preserves unrelated approvals, creates an adjacent backup before changing an existing
file, and refuses duplicate or malformed JSON keys. Re-running install is idempotent.

## Protocol probes

These three probes exercise the exact host framing without starting either host. The
instance shim supplies the matching configuration even when the current directory is
elsewhere. The example directory must exist if you want context rather than a fail-open
result.

```bash
mkdir -p /tmp/project
printf '%s' '{"workspace":{"current_dir":"/tmp/project"}}' \
  | ~/regesto-kb/bin/regesto-hook claude-session-start-v1
printf '%s' '{"cwd":"/tmp/project","session_id":"s1","extra":{"is_first_turn":true}}' \
  | ~/regesto-kb/bin/regesto-hook hermes-pre-llm-v1
printf '%s' '{"cwd":"/tmp/project","session_id":"s1","extra":{"is_first_turn":false}}' \
  | ~/regesto-kb/bin/regesto-hook hermes-pre-llm-v1
```

The Claude probe emits plain context. The first Hermes probe emits either a compact
`{"context":"..."}` object or `{}` when no context is available; the repeated session
probe emits exactly `{}`. Malformed input and operational failures also exit zero with
host-valid empty output. Diagnostics go to stderr and never contaminate protocol stdout.

The payload, framing, session-bound, registrar, and allowlist behavior are covered by
automated fixtures and contract tests. A live Hermes-host validation remains a separate
release-checklist step.
