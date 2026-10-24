# Changelog

## v1.3.1

- Fix the SQLite, MySQL and PostgreSQL backends logging an ERROR record every time a worker polled and found no work. Update common-go to v0.10.0, whose GORM logger writes a lookup that finds no record as a DEBUG query. Only the log level changes: queries still return `gorm.ErrRecordNotFound`.
- Log GORM records with the caller's ctxscope attributes.
- Keep the local `.telemetry/` directory out of the Docker build context.

No configuration or API migration is needed.

## v1.3.0

- Fix workflow tasks that failed when the executor cache expired or evicted their executor mid-task. The cache closed the executor from its eviction goroutine while the task was still replaying or executing, which raced with the running workflow and failed the replay with "previous workflow execution scheduled an activity which could not be found". An executor closed during a task now stops its workflow after that task finishes, and a task that finds its cached executor already closed runs in a new executor.
- Fix a worker panic on a workflow task with no new events. The SQLite, MySQL and PostgreSQL backends now release a claim whose instance has no visible pending events instead of returning an empty task, and the worker rejects an empty task with `commerr.ErrInvalidState`.
- Add `executor.ErrExecutorClosed`, returned by `ExecuteTask` on a closed executor. `ExecuteTask` also refuses a second concurrent task on the same executor with `commerr.ErrInvalidState`.

No configuration or API migration is needed.

## v1.2.0

- Add a go-workflows usage skill, Claude Code and Codex plugin manifests, and release-gated ClawHub publishing.
- Use GORM Gen repositories for SQLite, MySQL and PostgreSQL storage, with package-local models and common-go connection and migration helpers.
- Preserve existing SQL schemas, migration ledgers and caller-owned connections. Keep worker claims and workflow checkpoints transactional.
- Update common-go to v0.9.1 so PostgreSQL migration connections return to the application pool.
- Count runnable workflows once per instance and keep diagnostic pagination stable across tied timestamps and execution IDs.
- Test legacy SQL rows, migration retries, connection ownership, history batches and retention across reused execution IDs.
- Check coroutine exit through its own deferred cleanup instead of process-wide goroutine counts.
- Explain clients, queues, claims and worker recovery in the README.

Existing databases retain their schema and migration ledger. The GORM storage transition requires no schema conversion or SQL DOWN migration.

## v1.1.0

- Add complete commented client/worker examples, explain the feature samples and check README examples against runnable source.
- Correct the registration demo's activity arguments and allow the scaling worker's backend flag to be registered before parsing.
- Register the queue demo's definitions once before polling and test its routed workflow and activity results.
- Await persisted expiration in backend tests instead of assuming asynchronous cleanup finishes within one second.
- Remove fixed Go and UI CPU quotas. Accept an optional local `DEV_CPUS` setting and leave CI runners uncapped.
- Attach stdin and a terminal in `make shell` so the documented manual sample commands can run interactively.
- Include independent worker coverage snapshots before deliberate crashes and merge child counters with the parent profile.
- Return database state-query errors from MySQL and SQLite instead of reporting an active workflow. Test real storage failures, completion rollback, corrupt persisted data, replay mismatches, orchestrator lifecycle and diagnostic trees.
- Reject MySQL and PostgreSQL activity heartbeats when no matching worker-owned activity exists.
- Separate architecture, worker identities and routing, diagnostics, generation, development, testing and release documentation. Clarify diagnostics access control and backend connection ownership.
- Move `MockBackend` and `NewMockBackend` into `backend/mocks`. Generate `backend.gen.go` with Mockery from colocated `gen.go` and `config.yml`; exclude generated files from coverage, not Git.
- Generate the worker test's generic `MockTaskWorker` under `internal/worker/mocks` instead of maintaining it by hand.
- Enforce a 90% minimum in `make test-coverage`, retain the detailed coverage profile and test coverage-gate boundaries. Exclude generated code, sample programs and benchmarks from the reported percentage while running all tests.
- Add client tests for serialization, storage failures, terminal workflow results and automatic expiration updates.
- Remove the separate workflow analyzer module and custom-linter setup. All Go code, tests and tools use the root module and vendor tree.
- Add independent worker process tests for PostgreSQL and SQLite recovery, activity leases, replay, cancellation, signals, timers, retries and queue routing.
- Run recovery scenarios through the normal race-enabled test and coverage commands, with a focused `make test-recovery` target.

### Breaking tooling changes

Tests must replace imports of `backend.MockBackend` and `backend.NewMockBackend` with `mocks.MockBackend` and `mocks.NewMockBackend` from `github.com/psyb0t/go-workflows/backend/mocks`.

The separate analyzer module and custom-linter plugin are removed. Remove analyzer dependencies, plugin configuration and `MODULE_DIR=analyzer` from development commands. See [migration instructions](docs/releases.md#migrating-from-v100).

## v1.0.0

- Adopt the `github.com/psyb0t/go-workflows` module path and matching analyzer module.
- Add Docker-backed build, formatting, lint, security, dependency and generation commands with locked Go tools and vendored dependencies.
- Run complete race-enabled backend tests against isolated MySQL, PostgreSQL and Redis services with dynamic loopback ports.
- Preserve non-retryable failures, joined causes and cancellation identity through persisted workflow errors.
- Propagate `ctxscope` metadata through workflows, subworkflows and activities while suppressing replayed workflow logs.
- Fix test scheduler timer cancellation and activity-completion races, and reject invalid registration values without reflection panics.
- Report resource cleanup errors and enable `errcheck` and source-security checks.
- Replace the diagnostics UI's Create React App build with a pinned Vite and pnpm toolchain.
- Validate release tags and their Go module major versions, wait for successful default-branch checks for the same commit, and reuse their coverage artifact.
- Add a runnable SQLite quick start, current usage documentation and upstream credits.
- Mirror Git refs to GitLab and Codeberg, archive repository pages, and synchronize mirror issues through reusable workflows.

### Migration

This fork starts a separate module release line. Update imports and dependency declarations:

| Previous module | New module |
| --- | --- |
| `github.com/cschleiden/go-workflows` | `github.com/psyb0t/go-workflows` |
| `github.com/cschleiden/go-workflows/analyzer` | `github.com/psyb0t/go-workflows/analyzer` |

Use the Go version in `go.mod` and run development commands through Make. Existing workflow histories retain their supported error wire fields; arbitrary application error types are not reconstructed after persistence.
