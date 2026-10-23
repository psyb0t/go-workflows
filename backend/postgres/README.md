# PostgreSQL backend

PostgreSQL stores histories, pending events, timers and activity tasks for workers sharing one database. See [backend operations](../../docs/backends.md) for durability, retention and storage isolation.

## Connections and migrations

`NewPostgresBackend` opens and owns a pgx-backed SQL connection and applies embedded migrations by default. Its convenience connection string uses `sslmode=disable`. For a remote database requiring TLS or other connection policy, configure a `*sql.DB` in the host application and pass it to `NewPostgresBackendWithDB`.

`NewPostgresBackendWithDB` leaves connection ownership with the caller and disables migrations by default. Enable them explicitly with `WithApplyMigrations(true)` when the application owns migration startup. `Close` does not close a caller-owned connection. `WithPostgresOptions` configures connections opened by the convenience constructor.

Constructors can panic when initialization or automatic migration fails. Keep credentials outside source and apply migrations before accepting work. Back up the database before schema upgrades.

## Maintaining the backend

Embedded migrations live in `db/migrations`, with paired `.up.sql` and `.down.sql` files. Follow the existing sequence and preserve published migrations. Test schema changes against real PostgreSQL using the production constructor and migration path.

Run `make test-postgres` for backend contracts and `make test-recovery` for independent worker process recovery. Both use isolated PostgreSQL with `-race`; neither needs a production database. See [testing](../../docs/testing.md).
