# go-workflows

Instructions for coding agents (Codex, Claude Code and others) and for people changing this repository. People who want to use go-workflows want the documentation site, https://psyb0t.github.io/go-workflows/, not this file.

## Agent setup

`CLAUDE.md` is a committed symlink to `AGENTS.md`, and `.claude` is a committed symlink to `.agents`, so Claude Code reads the same instructions, rules and skills as Codex. Claude's Edit and Write tools refuse to write through a symlink, so edit `AGENTS.md` and `.agents/` files directly.

## What it is

An importable Go workflow engine (`github.com/psyb0t/go-workflows`), not a separate orchestration server. Workflows run activities, wait for signals and timers, and resume from persisted history after a worker restarts. History lives in SQLite (a file or in memory), PostgreSQL, MySQL or Redis.

## Layout

| Path | What it holds |
| --- | --- |
| `workflow/`, `activity/`, `client/`, `worker/`, `registry/`, `core/` | The public engine API |
| `backend/` | The storage contract and the SQLite, MySQL, PostgreSQL and Redis backends |
| `diag/` | The diagnostics HTTP UI and API; `diag/app/` is the embedded UI source |
| `tester/`, `activitytester/` | Test helpers for workflow and activity code |
| `cmd/` | Generators such as `cmd/repogen` |
| `samples/` | Runnable examples, mapped in `samples/README.md` |
| `tests/` | Backend contracts, end-to-end and process recovery suites |
| `scripts/` | The scripts behind the Make targets |
| `docs/` | The documentation site (GitHub Pages, Jekyll, Just the Docs) |
| `.agents/skills/go-workflows/` | The public usage skill, published to ClawHub |

## Commands

Use Docker-backed Make targets, not host Go or Node commands. `make help` lists the current contract.

```bash
make format lint build
make test-coverage   # race-enabled unit + real-backend tests, 90% floor
make sec             # dependency and source security checks, writes sec.sarif
make generate        # package-local generators, see docs/generation.md
make docs            # build the documentation site into .testing/docs-site
```

`make test` includes real testcontainer backends and process recovery. Use `make pkg-add PKG=module@version` for pinned dependencies and `make pkg-lock` for vendor synchronization.

## Boundaries

Preserve public APIs, existing databases and caller-owned connection lifecycle. Do not hand-edit generated code, vendor sources or published migrations. Workers must register definitions before polling; clients and workers share storage and queues. Workflow code replays, and activities can repeat.

Public rules and skills under `.agents/` are distributed. Keep them accurate and free of credentials or private machine details.

## Rules

Read every `.agents/rules/*.md` before working here. Area rules:

- For `backend/{sqlite,mysql,postgres}/`, `cmd/repogen/`, migrations or SQL dependencies, read `.agents/rule-details/sql-backends.md` and the affected backend README.
- For `.agents/` or publishing changes, read `.agents/rules/agent-integrations.md` and `docs/releases.md`.
- For workflow behavior, read `docs/guide.md` and `docs/architecture.md`.
- For diagnostics, read `docs/diagnostics.md` and `diag/app/README.md`.
