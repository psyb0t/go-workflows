// Package db owns mysql connection setup and the backend's immutable migration history.
package db

import (
	"database/sql"

	commondb "github.com/psyb0t/common-go/db"
	commonmysql "github.com/psyb0t/common-go/db/mysql"
	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/ctxerrors/commerr"
	mysqlgorm "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/psyb0t/go-workflows/backend/mysql/db/migrations"
)

// Open wraps a caller-owned pool. It never changes ownership or creates schema.
func Open(pool *sql.DB) (*gorm.DB, error) {
	if pool == nil {
		return nil, ctxerrors.Wrap(commerr.ErrRequiredFieldNotSet, "database pool")
	}
	dialector := mysqlgorm.New(mysqlgorm.Config{Conn: pool, SkipInitializeWithVersion: true})
	database, err := gorm.Open(dialector, &gorm.Config{
		Logger:                 commondb.NewGormSlogLogger(),
		SkipDefaultTransaction: true,
		DisableAutomaticPing:   true,
		TranslateError:         true,
	})
	if err != nil {
		return nil, ctxerrors.Wrap(err, "open mysql GORM")
	}
	return database, nil
}

// MigrateUp preserves the existing schema_migrations ledger and pool ownership.
func MigrateUp(pool *sql.DB) error {
	err := commonmysql.MigrateUp(pool, ".", &migrations.FS)
	if err != nil {
		return ctxerrors.Wrap(err, "migrate mysql backend")
	}
	return nil
}
