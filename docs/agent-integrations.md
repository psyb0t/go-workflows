---
title: "Agent integrations"
description: "Installing the go-workflows usage skill for Claude Code, Codex and OpenClaw, and what the passive agent package contains."
nav_order: 6
---

# Agent integrations

The [go-workflows skill](https://github.com/psyb0t/go-workflows/blob/main/.agents/skills/go-workflows/SKILL.md) teaches coding agents how to connect clients and workers, choose storage, route queues, use activities, retries, signals and timers, and test durable execution. Installing it adds instructions, not a running workflow service.

## Claude Code and Codex

Both install from the psyb0t marketplace:

```bash
claude plugin marketplace add psyb0t/agents
claude plugin install go-workflows@psyb0t

codex plugin marketplace add psyb0t/agents
codex plugin add go-workflows@psyb0t
```

The plugin manifests are [`.agents/.claude-plugin/plugin.json`](https://github.com/psyb0t/go-workflows/blob/main/.agents/.claude-plugin/plugin.json) and [`.agents/.codex-plugin/plugin.json`](https://github.com/psyb0t/go-workflows/blob/main/.agents/.codex-plugin/plugin.json).

## OpenClaw

OpenClaw installs the skill from ClawHub:

```bash
openclaw skills install @psyb0t/go-workflows
```

## A local checkout

Publication follows a tagged release after default-branch checks pass, as described in [releases](releases.md). Before publication, Codex discovers [`.agents/skills/`](https://github.com/psyb0t/go-workflows/tree/main/.agents/skills) in a checkout directly, and Claude Code can load it with:

```bash
claude --plugin-dir ./.agents
```

The package contains no hooks, automatic installers or MCP server.
