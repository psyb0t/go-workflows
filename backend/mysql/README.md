# MySQL backend

MySQL stores workflow histories, pending events, timers and activities for workers sharing one database. See [backend operations](../../docs/backends.md) for durability, retention and deployment boundaries.

## Connections and migrations

`NewMysqlBackend` opens and owns its SQL connection and applies embedded migrations by default. Its `Close` closes that connection. `WithMySQLOptions` configures the pool before migration.

`NewMysqlBackendWithDB` uses a caller-owned `*sql.DB`. It does not close the connection and disables migrations by default. To apply migrations through that constructor, set `WithApplyMigrations(true)` and provide `WithMigrationDSN` with multi-statement support. Keep connection credentials in application configuration, not source.

Constructors can panic when initialization or automatic migration fails. Apply migrations during controlled startup, before accepting work, and back up the database before schema upgrades.

## Maintaining the backend

Embedded migrations live in `db/migrations`, with paired `.up.sql` and `.down.sql` files. Follow the existing sequence, preserve published migrations and test schema changes against real MySQL. The constructor and `Migrate` method use this embedded source.

Run `make test-mysql` from the repository root. It creates its own isolated MySQL service and runs the backend contracts with `-race`; it does not use a production database. See [testing](../../docs/testing.md).
