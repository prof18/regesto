# Regesto

**A local knowledge base your agents consult—not another memory cache.**

> *[Regèsto](https://it.wikipedia.org/wiki/Regesto)* (Latin *regĕsta*, “carried back”): an
> archivist’s register of an archive too large to read end to end—one short entry per
> document, recording its date, source, and subject.

Agents forget, compress, and rewrite their own memory. Regesto keeps the decisions you
want to preserve as plain Markdown files you own. Before an agent acts, it can search
those files for the relevant project decisions, preferences, conventions, and environment
facts.

Regesto is designed around three ideas:

- one durable claim per Markdown file;
- a validated way for agents to search and write claims;
- visible history: an outdated claim is marked `superseded`, not deleted.

For example, a project decision can look like this:

```markdown
---
id: dec-sessions-not-jwt
title: Sessions are server-side
type: decision
scope: project:aurora
subject: auth
relation: session-transport
status: active
source: human
---

Session state lives server-side. The cookie contains only an opaque identifier.

**Why:** revocation has to be immediate.
```

The `subject` and `relation` identify what the claim is about. If that decision changes,
Regesto can find the previous claim exactly instead of relying on fuzzy similarity.

## Quick start

This path is for the built-in **Claude Code**, **Codex**, and **Hermes** integrations.
For another client, skip to [Connect another agent](#connect-another-agent).

### 1. Install Regesto

```bash
brew install prof18/tap/regesto
```

You can also use `go install github.com/prof18/regesto/cmd/regesto@latest` or download a
binary from [GitHub Releases](https://github.com/prof18/regesto/releases).

### 2. Create a knowledge base

```bash
regesto init --dir ~/regesto-kb --examples
cd ~/regesto-kb
```

This creates a local folder containing your facts, configuration, agent adapters, and
helper commands. The folder name and location are up to you. Remove `--examples` if you
want to start empty.

During initialization, Regesto detects supported agents installed on this machine and
adds them to `config.toml`. Check that the list matches the agents you want to connect:

```toml
integrations = ["claude", "codex", "hermes"]
```

Keep only the integrations you use. Detection is an initial suggestion, not a permanent
setting.

### 3. Connect the agents

```bash
regesto install --dry-run
regesto install
regesto doctor
```

The dry run shows every file Regesto would touch. Installation adds the appropriate
skills and instructions, registers supported hooks, and backs up host files before
editing them. It is safe to run again.

`doctor` is read-only. It reports what is installed, what needs attention, and the exact
remediation for manual steps.

### 4. Start a new agent session

Open a new session from one of your projects so the agent reloads its hooks, skills, and
instructions. Then try:

> Remember that release builds run on CI.

The installed `regesto-write` skill records the fact through Regesto's validated write
path. In a future session, the agent can find that decision before acting.

That is the complete manual workflow. Automatic harvesting and multi-machine sync are
optional.

## What each built-in integration does

| Agent | How it consults Regesto | Setup notes |
|---|---|---|
| Claude Code | A session-start hook injects relevant context automatically | [Claude Code setup](docs/setup-claude-code.md) |
| Hermes | A first-turn hook injects relevant context when its hook registration is current | [Hermes setup](docs/setup-hermes.md) |
| Codex | Persistent instructions tell it when to search; skills provide the search and write procedures | [Codex setup](docs/setup-codex.md) |

Claude and Hermes can enforce consultation at their supported hook boundaries. Codex has
no equivalent built-in startup hook, so its consultation is instruction-driven. The
[integration matrix](docs/agent-integration.md) contains the exact capability reference.

## Connect another agent

Choose the first option your client supports:

| Client capability | Use |
|---|---|
| Local skills or a persistent instructions file | Configure a custom integration with the `generic` profile |
| Local stdio MCP servers | Launch `regesto mcp` from the client |
| Neither | Export the conversation and run `regesto promote` |

[Connect any agent to Regesto](docs/setup-other-agents.md) walks through each path. The
custom-agent section is intentionally more detailed because Regesto cannot safely guess
an unknown client's paths, memory format, or hook protocol.

## Optional: capture agent memory automatically

Direct writes are the safest way to preserve important facts. Regesto can also watch the
native memory of configured agents and turn new material into canonical facts:

```bash
regesto schedule install
regesto schedule status
```

The scheduled workflow runs two operations:

- `regesto harvest` copies new native-memory changes into `inbox/`;
- `regesto cycle` normalizes captures, resolves contradictions, rebuilds indexes, and
  commits a clean pass.

On macOS, `schedule install` uses launchd. On other systems, schedule `regesto harvest`
and `regesto cycle` with your preferred job runner.

### Cycle health and external alerts

When a cycle is failing, `regesto context` on the machine that runs it puts the failure
reason and start time at the top of agent context and tells the agent to treat generated
indexes as stale. Claude and Hermes receive that warning through their context hooks.
Codex has no startup hook, so use `regesto context` or `regesto schedule status` when
diagnosing its instance.

Regesto does not send desktop notifications itself. A CLI cannot provide a consistent
click action across macOS, Linux, and other environments. If you want an external alert,
configure the executable or CLI you already use:

```toml
[notify]
command = "/absolute/path/to/my-notifier --fixed-option"
renag_hours = "24"
```

`notify.command` is whitespace-separated: the executable path and each fixed argument
must not contain spaces. Shell syntax, or a path with spaces, requires a wrapper script
at a whitespace-free path; the command is not run through a shell.

Regesto executes the command directly when the cycle starts failing, recovers, or reaches
the reminder interval. It appends the notification title and message as the final two
arguments and also sets these environment variables:

- `REGESTO_NOTIFY_KEY` — currently `cycle`;
- `REGESTO_NOTIFY_STATE` — `failing` or `ok`;
- `REGESTO_NOTIFY_TITLE`;
- `REGESTO_NOTIFY_MESSAGE`.

The command is optional; without it, cycle health still appears in agent context on the
cycle machine and in `regesto schedule status`. Set `on = "off"` to temporarily disable a
configured command, or `renag_hours = "0"` to report state transitions without reminders.
Because the command is not run through a shell, put pipelines, redirections, quoting, or
other shell behavior inside that wrapper script.

## More than one machine

Regesto has no sync server. Put the knowledge-base folder in a file-sync tool that keeps
a full local copy on each machine, and exclude `.state/` and `.git` from syncing.

Each machine harvests its own agents. Only one machine should run `regesto cycle` and own
the Git repository. See [multi-machine setup](docs/setup-sync.md) for the safe order and
failure modes.

## Everyday commands

Run commands from the knowledge-base directory, or pass
`--config /absolute/path/to/config.toml` after `regesto`.

| Command | Purpose |
|---|---|
| `regesto search` | Search facts by text, subject, relation, or scope |
| `regesto write` | Validate and record one structured fact |
| `regesto doctor` | Diagnose integration setup without changing files |
| `regesto install` | Install or refresh configured agent integrations |
| `regesto harvest` | Capture changes from configured agent memory |
| `regesto cycle` | Normalize captures, reconcile facts, rebuild indexes, and commit |
| `regesto promote` | Extract facts from an exported conversation |
| `regesto lint` | Validate the knowledge base and report contradictions |
| `regesto config` | Show the resolved instance configuration |

Use `regesto <command> --help` for the complete options.

## Updating and removing Regesto

After updating the binary, refresh the engine-owned files in the knowledge base:

```bash
cd ~/regesto-kb
regesto upgrade --dry-run
regesto upgrade
```

Regesto leaves files you changed alone and reports them. `--force` overwrites customized
engine-owned files only after backing them up.

To remove scheduled jobs, run `regesto schedule uninstall`. To disconnect an agent,
inspect `regesto install --dry-run --json`, then remove only the reported Regesto links,
hook entries, and the marked `regesto:section:start` / `:end` block from the agent's
instructions. Regesto does not remove host-owned configuration automatically.

## Documentation

- [Connect any agent](docs/setup-other-agents.md) — custom local agents, MCP clients, and
  conversation exports.
- [Integration matrix](docs/agent-integration.md) — exact capabilities and built-in
  profile metadata.
- [Multi-machine setup](docs/setup-sync.md) — sync rules and adding another machine.
- [Schema](SCHEMA.md) — what a fact is and how supersession works.
- [Design](DESIGN.md) — architecture and tradeoffs.
- [Contributing](CONTRIBUTING.md) — development workflow and compatibility rules.

## Requirements

Go 1.26 is required to build from source. Most commands are portable; the built-in
scheduler currently targets macOS launchd.

## License

MIT.
