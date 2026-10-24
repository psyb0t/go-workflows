# Testing

All Go test targets run in the development image with `-race`, an explicit timeout and caching disabled. `make test` runs unit tests, backend contracts and process recovery tests. `make test-coverage` runs the same packages with atomic coverage and fails below 90%. CI calls that coverage target. A stricter `MIN_TEST_COVERAGE` is allowed, but values below 90 are rejected.

The command prints function coverage, retains the detailed `coverage.out` profile and writes `coverage-percent.txt` for CI badges. The gate counts distinct instrumented statement blocks, including function literals in package-level initializer tables. Go's function summary only counts named function declarations, so its total can differ. Generated `*.gen.go`, directive-only `gen.go`, and the root `samples/` and `bench/` trees are excluded from reporting. Tests still run across all packages with whole-module instrumentation, so library code exercised by sample or integration tests counts. Integration helpers remain in the measured scope. `make clean` removes those reports.

`TEST_TIMEOUT` bounds each package. `TEST_COUNT` repeats normal and recovery suites; coverage always uses a fresh single run. Set `MIN_TEST_COVERAGE` to a value from 90 through 100 to tighten the gate. Raising the configured minimum does not establish that the current code reaches it; the measured run must pass.

## Commands

```bash
make test-unit
make test-recovery
make test-recovery TEST_COUNT=3
make test
make test-coverage
```

The complete suite starts isolated PostgreSQL, MySQL and Redis containers through `tests/testinfra`. Missing Docker is a failure. The unit target excludes external services; file-backed SQLite scenarios still run. No production database or credentials are needed.

## Independent worker recovery

`tests/integration` starts workers in separate operating-system processes. Each child receives its configuration on stdin and opens the production backend without recreating its schema. The parent applies production migrations before starting workers. Normal tests reexecute the race-instrumented test binary. Coverage builds a regular race-instrumented worker binary so it can snapshot counters before SIGKILL. The parent waits for the snapshot acknowledgement, then kills the worker. A failed snapshot fails the test. Child counters merge with the parent profile before filtering and threshold checks.

PostgreSQL and file-backed SQLite scenarios check:

- A live worker retains its activity lease beyond the original expiration through heartbeats.
- Killing an activity worker lets another worker recover the expired task.
- Replay reuses the completed activity result instead of executing it again.
- Signals sent before or after replacement-worker startup survive restart.
- Durable timers and cancellation survive closing and reopening the backend.

PostgreSQL scenarios also check separate workflow/activity queues, workers that must not consume unrelated queues, competing replicas, graceful shutdown, transient retries, retry exhaustion, permanent failures, retry deadlines and recovery during retry backoff.

The crash test deliberately applies an external side effect before killing its worker without a recorded result. A replacement repeats that activity. The external fixture deduplicates by operation ID and step. This tests at-least-once delivery and application-owned idempotency, not exactly-once side effects.

Cancellation is cooperative. A workflow waiting on a signal selects between that signal and `ctx.Done()`. A plain channel receive does not automatically observe cancellation.

## Scope

Sample tests run both README programs against SQLite and verify that their complete source matches the README code blocks. Regression tests also execute real registered activity methods and check the scaling worker's backend flag in a child process. Makefile tests inspect Go, integration and UI run commands with local and CI settings to prevent fixed CPU quotas from returning.

The existing backend suites cover PostgreSQL, MySQL, Redis, SQLite and monoprocess behavior. Process crash recovery is tested on PostgreSQL and SQLite; SQLite workers share a database file on one host. These tests do not establish distributed SQLite support or deterministic workflow safety for arbitrary application code.

Storage failure tests close real connections, make individual SQL tables unavailable and corrupt stored metadata or event payloads. They check returned errors, transaction rollback and successful retries after restoring storage. Redis tests check closed-client errors and serialization failures without losing the leased workflow. Executor tests feed inconsistent replay events through `ExecuteTask` and check the resulting workflow failure.

SQL lease tests reject unknown workflow and activity identities, retry failed activity completion without losing its task, and reject duplicate completion. MySQL and PostgreSQL return `commerr.ErrNotFound` when an activity heartbeat matches no worker-owned row. Retention tests verify that a failed history or attribute deletion rolls back the instance deletion.

SQL repository tests rerun migrations over populated databases, check migration-ledger stability and verify that a one-connection pool remains usable. Physical-schema fixtures remain readable and completable through generated repositories. Separate database handles use identical workflow identities without sharing query state. Cursor walks cover tied timestamps, partial pages and the empty end page. Batched histories retain sequence order, runnable counts count workflows rather than pending events, and retention preserves active executions whose IDs overlap deleted executions.

Keep workflow definitions replay-compatible and test application workflows separately. Runtime recovery tests do not replace that responsibility.
