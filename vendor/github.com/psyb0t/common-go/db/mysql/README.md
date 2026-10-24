# MySQL connections and migrations

`New(ctx, ensureDBExists)` loads `DB_MYSQL_*` environment variables. `NewWithConfig(ctx, cfg, ensureDBExists)` accepts explicit configuration. Both validate before connecting, return a `Mysql` with `SQLDB` and `GormDB` sharing one pool, and leave migrations under the caller's control. Call `Close` on shutdown. No schema is created by GORM `AutoMigrate`.

```go
connection, err := mysql.NewWithConfig(ctx, mysql.Config{
    Hostname: "database.example",
    Port:     3306,
    Username: "example",
    Password: password, // Load from your application's configuration.
    Database: "example",
    IsSSL:    true,
}, false)
if err != nil {
    return ctxerrors.Wrap(err, "connect database")
}
// Register connection.Close with the application's shutdown lifecycle.
if err := connection.MigrateUp("migrations", &migrationFS); err != nil {
    return ctxerrors.Wrap(err, "migrate database")
}
// Feed connection.GormDB into your generated GORM repositories.
```

The example uses `github.com/psyb0t/common-go/db/mysql` and `github.com/psyb0t/ctxerrors`. `migrationFS` is an `embed.FS` containing the application's SQL migrations. Pass `"."` when SQL files are embedded at its root. Pass a filesystem directory and `nil` instead to load migrations from disk.

## Configuration and ownership

`Hostname`, `Port`, `Username` and `Database` are required by explicit configuration. Port must be 1 through 65535. Password may be empty for passwordless accounts. Database names are limited to 64 characters and cannot contain NUL. Optional creation quotes the database identifier and uses `CREATE DATABASE IF NOT EXISTS`; set `ensureDBExists=false` when the application account must not have CREATE privileges.

`New` defaults `DB_MYSQL_PORT` to `3306` and `DB_MYSQL_ISSSL` to `true`. Explicit configuration uses the supplied values, including `IsSSL`. TLS verifies the server certificate and hostname against system roots, with no plaintext fallback. Set `IsSSL=false` only for an intentionally unencrypted local connection. Credentials pass through the driver's structured configuration, not an interpolated DSN.

The pool parses SQL date/time values into UTC `time.Time` values. It does not change the server session's time zone. Connection establishment retries within the caller's context and a 30-second limit, with a five-second dial timeout. GORM uses the shared logger adapter and translates supported constraint errors into GORM sentinels.

## Migrations

Methods `MigrateUp(path, fs)`, `MigrateDown(path, steps, fs)` and `MigrateForce(path, version, fs)` open and close a separate migration pool with multi-statements enabled. The normal pool stays open and rejects multi-statement queries. MySQL advisory locks coordinate independent migrators against the same database and migration table.

Standalone functions accept an existing `*sql.DB` as their first argument, followed by the same method arguments. That pool must already have `multiStatements=true`. Each call reserves and releases one connection. It never closes the caller's pool, including on migration failure. The wrapper does not own caller-supplied pools.

The shared runner handles version ordering and treats an up-to-date database as success. Migration metadata uses golang-migrate's default `schema_migrations` table. Keep existing versions and filenames unchanged when adopting these helpers. A migration directory needs matching `.up.sql` and `.down.sql` files.

MySQL DDL can commit implicitly. A failed multi-statement migration can leave partial schema changes and a dirty migration version; it is not transactionally rolled back as a unit. Inspect and repair the schema before using `MigrateForce` to repair metadata. Force does not execute SQL. Down requires a positive step count. Do not run down or force concurrently with application traffic without a deployment plan.

## Tests

`make test PKG=./db/mysql` runs configuration and real MySQL integration tests with the race detector. The suite starts one pinned MySQL testcontainer, creates isolated databases, and removes its own container after the suite. It requires Docker and does not skip when infrastructure is unavailable.

See the [project README](../../README.md) for the shared development commands.
