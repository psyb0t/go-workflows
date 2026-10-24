package postgresql

import (
	"context"
	"embed"
	"errors"

	"github.com/golang-migrate/migrate/v4/database"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/psyb0t/common-go/db"
	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/ctxerrors/commerr"
)

// MigrateUp applies pending migrations and releases its connection, not SQLDB.
func (p *Postgresql) MigrateUp(path string, fs *embed.FS) error {
	return p.withMigrationDriver(func(driver database.Driver) error {
		return db.MigrateUp(p.config.Database, driver, path, fs)
	})
}

// MigrateDown reverts steps migrations without closing the application pool.
func (p *Postgresql) MigrateDown(path string, steps int, fs *embed.FS) error {
	if steps <= 0 {
		return ctxerrors.Wrap(commerr.ErrInvalidArgument, "migration steps must be greater than zero")
	}

	return p.withMigrationDriver(func(driver database.Driver) error {
		return db.MigrateDown(p.config.Database, driver, path, fs, steps)
	})
}

// MigrateForce changes the ledger version without executing schema changes.
func (p *Postgresql) MigrateForce(path string, version int, fs *embed.FS) error {
	return p.withMigrationDriver(func(driver database.Driver) error {
		return db.MigrateForce(p.config.Database, driver, path, fs, version)
	})
}

func (p *Postgresql) withMigrationDriver(run func(database.Driver) error) error {
	if p == nil || p.SQLDB == nil {
		return ctxerrors.Wrap(commerr.ErrRequiredFieldNotSet, "PostgreSQL database")
	}

	conn, err := p.SQLDB.Conn(context.Background())
	if err != nil {
		return ctxerrors.Wrap(err, "reserve PostgreSQL migration connection")
	}
	// WithConnection gives the migration driver ownership of only this reservation.
	// The pgx migration driver's WithInstance would also own the application pool.
	driver, err := migratepostgres.WithConnection(context.Background(), conn, &migratepostgres.Config{})
	if err != nil {
		return ctxerrors.Wrap(errors.Join(err, conn.Close()), "create PostgreSQL migration driver")
	}

	runErr := run(driver)
	if err := errors.Join(runErr, driver.Close()); err != nil {
		return ctxerrors.Wrap(err, "run PostgreSQL migrations")
	}

	return nil
}
