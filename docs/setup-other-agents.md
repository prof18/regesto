# Connect any agent to Regesto

Use this guide when your client is not Claude Code, Codex, or Hermes, or when you are
building an integration for a new agent. The [integration matrix](agent-integration.md) is
the advanced reference for exact profiles and protocols.

## Choose a connection

Start with the first capability your client supports:

| Client capability | Setup path | Result |
|---|---|---|
| Built-in Claude Code, Codex, or Hermes profile | [Built-in integrations](#built-in-integrations) | Regesto already knows the client paths and supported hooks |
| Local skills or a persistent instructions file | [Custom local integration](#custom-local-integration) | Regesto installs only the capabilities you configure |
| Local stdio MCP servers | [MCP client](#mcp-client) | The client gets Regesto search, read, project, and write tools |
| None of these | [No local integration surface](#no-local-integration-surface) | Export a conversation and promote it manually |

The local and MCP paths can be combined. For example, persistent instructions can make
consultation habitual while MCP provides the actual tools.

Before continuing, install Regesto and create a knowledge base:

```bash
brew install prof18/tap/regesto
regesto init --dir ~/regesto-kb --examples
```

Run the remaining commands from `~/regesto-kb`, or pass
`--config ~/regesto-kb/config.toml` after `regesto`.

## Built-in integrations

For Claude Code, Codex, or Hermes, add the built-in ID to `config.toml` and run the normal
installer. You do not need to configure paths or copy a custom profile:

```toml
integrations = ["claude", "codex", "hermes"]
```

```bash
cd ~/regesto-kb
regesto install --dry-run
regesto install
regesto doctor
```

Keep only the integrations you use. See the short setup notes for
[Claude Code](setup-claude-code.md), [Codex](setup-codex.md), or
[Hermes](setup-hermes.md) if `doctor` reports a host-specific step.

## Custom local integration

Use the `generic` profile for an unsupported local client. Regesto will not guess an
unknown product's paths, memory format, or hook protocol.

### 1. Find the capabilities the client actually has

Look in the client's documentation or configuration for:

- an Agent Skills-compatible directory;
- a Markdown instructions file loaded into every session;
- optionally, a directory where the client writes Markdown memory;
- optionally, a documented startup or pre-model hook.

You need at least a skills directory or a persistent instructions file for this path. If
the client only supports MCP, skip to [MCP client](#mcp-client).

### 2. Add the minimum configuration

Choose a short, unique ID and add it to the top-level integration list. Configure only
the paths the client supports:

```toml
# Keep any built-in IDs you already use and append the custom ID.
integrations = ["claude", "my-agent"]

[integrations.my-agent]
profile = "generic"
skills_dir = "~/.my-agent/skills"
instructions_file = "~/.my-agent/AGENTS.md"
trust = "quarantine"
```

A skills-only client can omit `instructions_file`. An instructions-only client can omit
`skills_dir`.

Create the instructions file yourself if it does not exist. The generic profile can add
its marked section to an existing file, but it will not create a file owned by another
client.

`quarantine` is the safe default for an unfamiliar source: harvested material stays out
of canonical knowledge until it has been reviewed.

### 3. Preview, install, and verify

```bash
cd ~/regesto-kb
regesto install --dry-run
regesto install
regesto doctor --integration my-agent
```

Read the dry run before applying it. It shows the resolved targets and whether Regesto
will create a link, update a marked instructions section, or leave something untouched.

After a clean result, restart the client and ask it to search Regesto for a known example
fact. Then ask it to remember a disposable fact and confirm that a new Markdown file
appears under `knowledge/facts/`.

### Optional: harvest the client's native memory

If the client writes Markdown memory, add its real location to the same section:

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
write new changes under `inbox/my-agent@<machine>/` for review and normalization.

Do not point `memory_location` at Regesto's `knowledge/` directory. Client memory is an
input to harvest, not the canonical store.

### Optional: add a hook

Do not reuse a built-in hook recipe unless the client documents the same event, input,
and output format. The generic profile deliberately declares no hook.

For a compatible documented hook, copy
`~/regesto-kb/adapters/profiles/generic.json` to
`~/regesto-kb/adapters/profiles/<id>.json`, change its `id` to the filename ID, and declare
the supported protocol with the `manual` registrar. The install dry run will print a
registration recipe instead of rewriting an unfamiliar settings file.

A new hook payload or response format needs a protocol adapter and contract tests in the
Regesto source. This is the only custom-agent path that normally requires code changes.

## MCP client

An MCP-only client does not need an entry in `integrations`. Configure it to launch the
local Regesto stdio server instead:

```json
{
  "mcpServers": {
    "regesto": {
      "command": "regesto",
      "args": ["--config", "/Users/you/regesto-kb/config.toml", "mcp"]
    }
  }
}
```

`regesto` is normally enough because the installation command puts it on your `PATH`.
Replace `/Users/you/regesto-kb` with the absolute path to your knowledge base; MCP clients
often do not expand `~` in JSON configuration.

If the client reports that it cannot find `regesto`, it is not inheriting your shell
`PATH`. Run `command -v regesto` in a terminal and use the returned path as `command`.
For example, Homebrew commonly returns `/opt/homebrew/bin/regesto` on Apple Silicon, but
the location depends on how Regesto was installed.

The equivalent shell command is:

```bash
regesto --config ~/regesto-kb/config.toml mcp
```

The client owns the process and communicates over stdin/stdout. Regesto opens no network
listener. It exposes tools for searching facts, reading an exact fact, resolving a
project, and writing a validated fact, plus resources for the index and individual facts.

MCP makes the tools available; it does not force the model to use them. If the client
supports persistent instructions, add a short rule telling it to search Regesto whenever
a recorded decision or preference could settle the task, and to use the Regesto write
tool when the user explicitly asks it to remember something.

Restart the client after changing its MCP configuration, then confirm it can list the
Regesto tools.

## No local integration surface

When a hosted or mobile client has no skills, instructions, MCP, hooks, or accessible
memory, export or copy the conversation and promote it from the knowledge-base directory.

First configure at least one installed agent CLI that can extract claims:

```toml
[normalize]
commands = "claude -p ;; codex exec --sandbox read-only"
```

Regesto tries the commands from left to right. For a single run, you can select one
explicitly with `--command "claude -p"`.

Then promote the export:

```bash
cd ~/regesto-kb
regesto promote ~/Downloads/conversation.md
# Or promote copied text:
pbpaste | regesto promote -
```

Review the resulting facts, then run `regesto lint --fix --rebuild`.

## Reading `doctor`

`regesto doctor --integration <id>` uses four useful states:

- `ok` or `current`: no action is required;
- `manual`: complete the printed step;
- `unsupported`: the profile intentionally does not declare that capability;
- `warning` or `error`: follow the attached remediation before testing a new session.

Do not expose `regesto mcp` through a public network listener. It provides access to the
complete local knowledge base and is intended to be launched locally by the client.
