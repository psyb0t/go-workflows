# Backends and operations

| Backend | Use | Persistence |
| --- | --- | --- |
| SQLite | One application host, embedded storage | Database file with WAL |
| PostgreSQL | Multiple worker processes | External SQL database |
| MySQL | Multiple worker processes | External SQL database |
| Redis | Multiple worker processes | Redis persistence policy |
| Monoprocess | Local single-process execution | Wraps another backend with in-process dispatch notifications |

Use each backend's constructor and options. The [quick start](../samples/quickstart/main.go) uses `sqlite.NewSqliteBackend`. Network backends take connection settings from the application; never embed credentials in source. Worker names, polling, lock durations, sticky execution and history limits are configurable through backend and worker options.

## Migrations and ownership

SQL backends embed production migrations. Constructors can apply migrations; constructors accepting existing database connections have distinct ownership and migration defaults. Read their Go documentation before supplying a shared connection. Close workers before storage. A backend created with a caller-owned database does not own that connection.

Use isolated databases or schemas for applications with separate retention, access and upgrade requirements. Back up storage before schema or library upgrades and test restoration with compatible workers. Diagnostics are not a backup.

## Locks and durability

Workers heartbeat active tasks. Expired locks allow another attempt, including an activity whose external side effect already happened. Set lock timeouts and heartbeat intervals for the workload, and make external operations idempotent.

SQLite serializes writers. Monoprocess dispatch is local to its process. Neither is a distributed worker fleet. Redis durability depends on the server's AOF, snapshots and replication. SQL durability depends on database transaction and storage settings.

## Retention

Finished histories remain until removed or expired. The client exposes automatic expiration where supported. Some operations return `backend.ErrNotSupported`; inspect that typed error rather than assuming identical maintenance features. Retention deletes workflow history, not external records or files created by activities.

`ContinueAsNew` creates another execution. `backend.WithRemoveContinuedAsNewInstances` controls immediate removal of prior executions. Choose retention with audit and debugging requirements in mind.

## Diagnostics

See [diagnostics](diagnostics.md) for the embedded UI, HTTP API, access control, metrics and tracing.

## Integration services

See [testing](testing.md) for isolated real-backend contracts and process recovery. Tests do not need a shared Compose stack.
