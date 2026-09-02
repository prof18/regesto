# Agent-agnostic Regesto integrations

Make Regesto universally usable through an agent-neutral core and capability-driven integration profiles. Preserve the working Claude Code, Codex, and Hermes integrations while allowing an arbitrary host to adopt portable skills, instructions, validated CLI/MCP operations, optional hooks, and optional file-memory harvesting without changing the knowledge format.

The shared, testable outcomes are in [facts.md](facts.md). The ordered implementation and verification procedure is in [plan.md](plan.md).

The goal is done when every accepted fact holds, all selected automated checks pass, the one live instance is manually updated to the current integration format, and live end-to-end regressions pass for Claude Code, Codex, and Hermes. Automatic migration of historical instance formats, remote services, and proprietary cloud-memory connectors are explicitly outside this goal.
