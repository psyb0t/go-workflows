package mysql

import (
	"context"
	"database/sql"
	"embed"
	"errors"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4/database"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	commondb "github.com/psyb0t/common-go/db"
	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/ctxerrors/commerr"
)

// MigrateUp applies pending migrations on a caller-owned pool configured with
// multiStatements=true. It releases its reserved connection, not the pool.
func MigrateUp(sqlDB *sql.DB, path string, fs *embed.FS) error {
	return withMigrationDriver(sqlDB, func(driver database.Driver) error {
		return commondb.MigrateUp(driverName, driver, path, fs)
	})
}

// MigrateDown reverts steps migrations without closing the caller's pool.
func MigrateDown(sqlDB *sql.DB, path string, steps int, fs *embed.FS) error {
	if steps <= 0 {
		return ctxerrors.Wrap(commerr.ErrInvalidArgument, "migration steps must be greater than zero")
	}

	return withMigrationDriver(sqlDB, func(driver database.Driver) error {
		return commondb.MigrateDown(driverName, driver, path, fs, steps)
	})
}

// MigrateForce changes only the recorded version and dirty flag, not the schema.
func MigrateForce(sqlDB *sql.DB, path string, version int, fs *embed.FS) error {
	return withMigrationDriver(sqlDB, func(driver database.Driver) error {
		return commondb.MigrateForce(driverName, driver, path, fs, version)
	})
}

func withMigrationDriver(sqlDB *sql.DB, run func(database.Driver) error) error {
	if sqlDB == nil {
		return ctxerrors.Wrap(commerr.ErrRequiredFieldNotSet, "MySQL database")
	}

	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		return ctxerrors.Wrap(err, "reserve MySQL migration connection")
	}

	driver, err := migratemysql.WithConnection(context.Background(), conn, &migratemysql.Config{})
	if err != nil {
		return ctxerrors.Wrap(errors.Join(err, conn.Close()), "create MySQL migration driver")
	}

	runErr := run(driver)
	if err := errors.Join(runErr, driver.Close()); err != nil {
		return ctxerrors.Wrap(err, "run MySQL migrations")
	}

	return nil
}

// MigrateUp uses a separate multi-statement pool, leaving SQLDB and GormDB unchanged.
func (m *Mysql) MigrateUp(path string, fs *embed.FS) error {
	return m.withMigrationPool(func(sqlDB *sql.DB) error {
		return MigrateUp(sqlDB, path, fs)
	})
}

// MigrateDown reverts steps migrations using a separate migration pool.
func (m *Mysql) MigrateDown(path string, steps int, fs *embed.FS) error {
	return m.withMigrationPool(func(sqlDB *sql.DB) error {
		return MigrateDown(sqlDB, path, steps, fs)
	})
}

// MigrateForce repairs migration bookkeeping without executing SQL files.
func (m *Mysql) MigrateForce(path string, version int, fs *embed.FS) error {
	return m.withMigrationPool(func(sqlDB *sql.DB) error {
		return MigrateForce(sqlDB, path, version, fs)
	})
}

func (m *Mysql) withMigrationPool(run func(*sql.DB) error) error {
	if m == nil || m.SQLDB == nil {
		return ctxerrors.Wrap(commerr.ErrRequiredFieldNotSet, "MySQL connection")
	}

	if err := m.SQLDB.PingContext(context.Background()); err != nil {
		return ctxerrors.Wrap(err, "check MySQL connection before migrating")
	}

	connector, err := mysqldriver.NewConnector(driverConfig(m.config, m.config.Database, true))
	if err != nil {
		return ctxerrors.Wrap(err, "configure MySQL migration pool")
	}

	sqlDB := sql.OpenDB(connector)

	runErr := run(sqlDB)
	if err := errors.Join(runErr, sqlDB.Close()); err != nil {
		return ctxerrors.Wrap(err, "close MySQL migration pool")
	}

	return nil
}
