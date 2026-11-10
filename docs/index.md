---
title: "go-workflows"
description: "Documentation for go-workflows, a durable workflow engine that runs inside a Go application and persists history in SQLite, PostgreSQL, MySQL or Redis."
nav_order: 1
permalink: /
seo:
  type: WebSite
---

# go-workflows

Durable workflows in Go. Run activities, wait for signals and timers, and resume from persisted history after a worker restarts. The engine is a library inside your application, not a separate server.

Start with [the runnable SQLite example](https://github.com/psyb0t/go-workflows/blob/main/samples/quickstart/main.go). The [README](https://github.com/psyb0t/go-workflows#readme) shows it and a smaller in-memory program in full.

| Topic | Read |
| --- | --- |
| Application roles, queues, replicas and identity | [Architecture and workers](architecture.md) |
| Determinism, activities, retries, timers and signals | [Workflow guide](guide.md) |
| Storage, migrations, ownership and retention | [Backends and operations](backends.md) |
| HTTP UI/API, authentication boundary and telemetry | [Diagnostics](diagnostics.md) |
| The usage skill for Claude Code, Codex and OpenClaw | [Agent integrations](agent-integrations.md) |
| Container toolchains, Make commands and dependencies | [Development](development.md) |
| Mockery, database repositories and generation drift | [Code generation](generation.md) |
| Race tests, real backends, recovery and coverage | [Testing](testing.md) |
| Tags, compatibility, CI and repository automation | [Releases](releases.md) |

Runnable examples for each feature are listed in the [samples feature map](https://github.com/psyb0t/go-workflows/blob/main/samples/README.md). Exported signatures are documented in [Go Reference](https://pkg.go.dev/github.com/psyb0t/go-workflows).
