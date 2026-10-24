# PostgreSQL connections and migrations

`New` loads `DB_POSTGRESQL_*` configuration; `NewWithConfig` accepts a `Config` directly. Both open a pgx-backed SQL pool shared by `SQLDB` and `GormDB`. Close the `Postgresql` value when the application shuts down.

## Migration ownership

`MigrateUp(path, fs)`, `MigrateDown(path, steps, fs)` and `MigrateForce(path, version, fs)` reserve one connection from `SQLDB`. They return it on success, source errors, SQL errors and driver initialization errors. They never close the application pool. A pool capped at one open connection remains usable after each call.

Pass an embedded filesystem and its relative directory, or pass a filesystem path with a nil filesystem. The SQL files and `schema_migrations` ledger determine which migrations run. Repeating UP preserves existing rows. DOWN requires a positive step count. Force changes migration bookkeeping only and does not execute SQL.

Migration execution uses a connection-scoped PostgreSQL migration driver over the existing pgx connection. This separates migration-driver cleanup from application-pool ownership. Normal SQL and GORM operations continue using pgx.

Migration failures are returned with their causes. Repair the schema before forcing a dirty version; forcing the ledger alone does not repair partially applied SQL. Back up databases before schema changes and test UP, DOWN and UP against disposable databases first.

## Tests

Run `make test PKG=./db/postgresql`. The suite starts isolated PostgreSQL and exercises production connection setup, pool ownership, embedded and filesystem sources, concurrent calls and dirty-version recovery with the race detector.

See [development commands](../../README.md#development) for complete checks.
