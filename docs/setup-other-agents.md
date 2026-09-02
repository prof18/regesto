# Connect any agent to Regesto

This is the setup guide for both people and agents. Start here when you are not sure
which integration path applies. The [agent integration matrix](agent-integration.md) is
the reference for exact built-in capabilities; this page is the procedure.

Regesto can connect to a client in four ways. Choose the first row the client supports:

| Client capability | Setup path | What it provides |
|---|---|---|
| Built-in profile: Claude Code, Codex, or Hermes | [Built-in integration](#built-in-integrations) | Known skills, instructions, hooks, and memory locations |
| Local skills directory or always-loaded instructions file | [Custom local integration](#custom-local-integration) | Portable skills and/or persistent instructions |
| Local stdio MCP servers | [MCP client](#mcp-client) | Search, reads, project resolution, and validated writes |
| None of the above | [Manual promotion](#no-local-integration-surface) | Facts extracted from an exported conversation |

These paths can be combined. For example, a client may use MCP for live searches and a
configured memory source for harvesting.

## Before connecting a client

Install Regesto and create one knowledge-base instance:

```bash
brew install prof18/tap/regesto
regesto init --dir ~/regesto-kb --examples
```

Run later commands from `~/regesto-kb`, or add
`--config ~/regesto-kb/config.toml` immediately after `regesto` when running elsewhere.

Two terms matter:

- A **configured integration** is an entry in `integrations = [...]`. Regesto installs
  local artifacts for it and can harvest its declared memory.
- A **detected integration** is a built-in client Regesto happens to find on this
  machine. Detection during `init` only supplies the initial list; it does not enable or
  disable anything later.

An MCP-only client does not need an entry in `integrations` because MCP is an interface to
the knowledge-base instance itself. Add an integration entry only if Regesto should also
install local files for that client or harvest its memory.

## Built-in integrations

Regesto ships profiles for `claude`, `codex`, and `hermes`. `regesto init` adds whichever
ones it detects at that moment. Open `~/regesto-kb/config.toml` and make the list match the
clients you want Regesto to manage:

```toml
integrations = ["claude", "codex", "hermes"]
```

It is fine to configure a built-in integration that is not currently detected. This is
useful when the same knowledge base is used on multiple machines.

Preview, install, and verify:

```bash
cd ~/regesto-kb
regesto install --dry-run
regesto install
regesto doctor
```

`install` renders and links skills, updates a marker-delimited section in an existing
instructions file, and registers hooks where Regesto has a preservation-safe registrar.
It backs up host files before changing them and is safe to run again.

Expected consultation behavior differs by host:

- **Claude Code:** its `SessionStart` hook injects context automatically.
- **Hermes:** its first-turn hook injects context after any reported manual YAML step has
  been completed.
- **Codex:** instructions tell it to search and the skills provide the procedure, but no
  built-in hook forces consultation.

For host-specific details, use [Claude Code](setup-claude-code.md),
[Codex](setup-codex.md), or [Hermes](setup-hermes.md).

## Custom local integration

Use this path for an unrecognized local agent such as a new CLI or desktop client. First
find what the client actually supports; do not guess vendor paths or settings formats.

Look for:

1. An Agent Skills-compatible directory.
2. A Markdown file loaded into every session.
3. A Markdown memory directory that the client writes.
4. A documented startup or pre-model hook.

Add a unique ID to the top-level list and put its actual paths in the matching section:

```toml
# Keep any built-in IDs you already use and append the custom ID.
integrations = ["claude", "my-agent"]

[integrations.my-agent]
profile = "generic"
skills_dir = "~/.my-agent/skills"
instructions_file = "~/.my-agent/AGENTS.md"
trust = "quarantine"
```

Only include capabilities the client really has. A skills-only client needs only
`skills_dir`; an instructions-only client needs only `instructions_file`. Create the
instructions file yourself if it does not exist—Regesto will add its marked section to an
existing host-owned file, but the generic profile will not invent that file.

Then run:

```bash
cd ~/regesto-kb
regesto install --dry-run
regesto install
regesto doctor --integration my-agent
```

The generic profile uses portable skills, has no guessed hook or memory location, and
defaults new harvested material to quarantine. `doctor` should show each configured
capability as current and undeclared capabilities as unsupported.

### Add native-memory harvesting

If the client writes Markdown memory, extend the same section:

```toml
[integrations.my-agent]
profile = "generic"
skills_dir = "~/.my-agent/skills"
instructions_file = "~/.my-agent/AGENTS.md"
memory_kind = "markdown-glob-v1"
memory_location = "~/.my-agent/memory"
trust = "quarantine"
```

The first `regesto harvest` records a baseline and normally captures nothing. Later runs
write only new changes under `inbox/my-agent@<machine>/`. Keep an unfamiliar or shared
source quarantined until a human explicitly approves it.

### Add a custom hook

The generic profile never guesses a hook format. If the client has a compatible hook,
copy `~/regesto-kb/adapters/profiles/generic.json` to
`~/regesto-kb/adapters/profiles/<id>.json`, make its JSON `id` match the filename, and
declare a supported protocol with the `manual` registrar. `regesto install --dry-run`
will print the exact registration recipe without rewriting an unfamiliar settings file.

A genuinely new hook payload or output format requires a small protocol adapter and
contract tests. That is the uncommon case where Regesto source code must change.

## MCP client

Use MCP when the client can launch a local stdio MCP server. Configure the client with an
absolute knowledge-base path; the exact outer setting name varies by client:

```json
{
  "mcpServers": {
    "regesto": {
      "command": "/absolute/path/to/regesto",
      "args": ["--config", "/Users/you/regesto-kb/config.toml", "mcp"]
    }
  }
}
```

The command is:

```bash
regesto --config /absolute/path/to/regesto-kb/config.toml mcp
```

Use `command -v regesto` to find the executable for a client that requires an absolute
`command` value.

The client starts and owns this process. It communicates over stdin/stdout; Regesto opens
no network listener. Protocol output stays on stdout and diagnostics go to stderr.

The server exposes:

- `regesto_search` — search canonical facts.
- `regesto_get_fact` — read one exact fact.
- `regesto_resolve_project` — map a working directory to its canonical project.
- `regesto_write_fact` — validate and atomically write a fact with explicit provenance.
- `regesto://index` and `regesto://facts/<id>` resources.

MCP access does not itself force the model to consult Regesto. If the client supports
persistent instructions, add something equivalent to:

```markdown
Before answering a question that a recorded decision, preference, convention, or
environment fact could settle, call `regesto_search` and read the matching exact facts.
When the user explicitly asks you to remember something, call `regesto_write_fact` with
source `<client-id>@<machine-name>`.
```

Restart the client after changing its MCP configuration, then confirm that it can list
the four Regesto tools.

## No local integration surface

A hosted, web, or mobile client may expose no local skills, instructions, hook, MCP, or
memory files. Regesto cannot integrate with it automatically. Export or copy the
conversation, then run the CLI from the knowledge-base directory:

First choose an installed agent CLI that can turn the transcript into facts. Configure a
fallback chain in `~/regesto-kb/config.toml`:

```toml
[normalize]
commands = "claude -p ;; codex exec --sandbox read-only"
```

Only one working command is required; Regesto tries the entries from left to right. You
can instead select one command for a single run with `--command "claude -p"`.

Then promote the export:

```bash
cd ~/regesto-kb
regesto promote ~/Downloads/conversation.md
# Or promote copied text:
pbpaste | regesto promote -
```

Review the resulting facts, and then run:

```bash
regesto lint --fix --rebuild
```

`/regesto-promote` is only a convenience when the current agent already has Regesto's
skills installed. The `regesto promote` CLI form works independently of that client.

## Reading `doctor`

Run `regesto doctor --integration <id>` after local integration setup. Its statuses mean:

- `ok` or `current`: no action is required.
- `manual`: Regesto prints a step it cannot safely perform for you.
- `unsupported`: the profile does not declare that capability; this is expected when you
  intentionally omitted it.
- `warning` or `error`: read the attached remediation before opening a new session.

After a clean install, open a new client session so it reloads skills and instructions.

## Safety boundaries

Do not point an agent's auto-memory directory at `knowledge/`. Vendor memory is a bounded,
pruned cache; Regesto should harvest changes from it instead of letting it own canonical
facts. Do not expose the stdio MCP process through a public network listener. It provides
access to the complete local knowledge base and is designed to be launched locally by the
client.
