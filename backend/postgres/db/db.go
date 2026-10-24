// Package db owns postgres connection setup and the backend's immutable migration history.
package db

import (
	"database/sql"

	commondb "github.com/psyb0t/common-go/db"
	commonpostgres "github.com/psyb0t/common-go/db/postgresql"
	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/ctxerrors/commerr"
	postgresgorm "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/psyb0t/go-workflows/backend/postgres/db/migrations"
)

// Open wraps a caller-owned pool. It never changes ownership or creates schema.
func Open(pool *sql.DB) (*gorm.DB, error) {
	if pool == nil {
		return nil, ctxerrors.Wrap(commerr.ErrRequiredFieldNotSet, "database pool")
	}
	dialector := postgresgorm.New(postgresgorm.Config{Conn: pool})
	database, err := gorm.Open(dialector, &gorm.Config{
		Logger:                 commondb.NewGormSlogLogger(),
		SkipDefaultTransaction: true,
		DisableAutomaticPing:   true,
		TranslateError:         true,
	})
	if err != nil {
		return nil, ctxerrors.Wrap(err, "open postgres GORM")
	}
	return database, nil
}

// MigrateUp preserves the existing schema_migrations ledger and pool ownership.
func MigrateUp(pool *sql.DB) error {
	connection := &commonpostgres.Postgresql{SQLDB: pool}
	err := connection.MigrateUp(".", &migrations.FS)
	if err != nil {
		return ctxerrors.Wrap(err, "migrate postgres backend")
	}
	return nil
}
