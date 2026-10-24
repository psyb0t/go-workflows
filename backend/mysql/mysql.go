package mysql

import (
	"context"
	"database/sql"
	"errors"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	commonmysql "github.com/psyb0t/common-go/db/mysql"
	"github.com/psyb0t/ctxerrors"

	"github.com/psyb0t/go-workflows/backend"
	backenddb "github.com/psyb0t/go-workflows/backend/mysql/db"
	"github.com/psyb0t/go-workflows/backend/mysql/db/migrations"
)

type mysqlBackend struct {
	*backenddb.Store
	db             *sql.DB
	workerName     string
	options        *options
	ownsConnection bool
	migrateOwned   func() error
}

var _ backend.Backend = (*mysqlBackend)(nil)

func NewMysqlBackend(host string, port int, user, password, database string, opts ...option) *mysqlBackend {
	connection, err := commonmysql.NewWithConfig(context.Background(), commonmysql.Config{
		Hostname: host, Port: port, Username: user, Password: password, Database: database,
	}, false)
	if err != nil {
		panic(ctxerrors.Wrap(err, "open MySQL backend"))
	}
	return wrapPool(connection.SQLDB, true, func() error { return connection.MigrateUp(".", &migrations.FS) }, opts...)
}

// NewMysqlBackendWithDB leaves connection configuration and ownership with the caller.
// Automatic migrations require WithMigrationDSN and are disabled by default.
func NewMysqlBackendWithDB(pool *sql.DB, opts ...option) *mysqlBackend {
	return wrapPool(pool, false, nil, opts...)
}

func wrapPool(pool *sql.DB, owned bool, migrateOwned func() error, opts ...option) *mysqlBackend {
	settings := &options{Options: backend.ApplyOptions(), ApplyMigrations: owned}
	for _, option := range opts {
		option(settings)
	}
	if owned && settings.MySQLOptions != nil {
		settings.MySQLOptions(pool)
	}
	name := getWorkerName(settings)
	store, err := backenddb.New(pool, name, settings.Options)
	if err != nil {
		if owned {
			err = errors.Join(err, pool.Close())
		}
		panic(ctxerrors.Wrap(err, "initialize MySQL backend"))
	}
	b := &mysqlBackend{Store: store, db: pool, workerName: name, options: settings, ownsConnection: owned, migrateOwned: migrateOwned}
	if settings.ApplyMigrations {
		if err := b.Migrate(); err != nil {
			if owned {
				err = errors.Join(err, pool.Close())
			}
			panic(ctxerrors.Wrap(err, "initialize MySQL migrations"))
		}
	}
	return b
}

func (b *mysqlBackend) Close() error {
	if !b.ownsConnection {
		return nil
	}
	if err := b.db.Close(); err != nil {
		return ctxerrors.Wrap(err, "close MySQL backend")
	}
	return nil
}

// Migrate applies the existing SQL history without closing caller pools.
func (b *mysqlBackend) Migrate() error {
	if b.options.MigrationDSN == "" {
		if b.migrateOwned == nil {
			return ctxerrors.New("cannot apply migrations: no DSN available; use WithMigrationDSN option or apply migrations externally")
		}
		if err := b.migrateOwned(); err != nil {
			return ctxerrors.Wrap(err, "migrate MySQL backend")
		}
		return nil
	}
	config, err := mysqldriver.ParseDSN(b.options.MigrationDSN)
	if err != nil {
		return ctxerrors.Wrap(err, "parse MySQL migration DSN")
	}
	connector, err := mysqldriver.NewConnector(config)
	if err != nil {
		return ctxerrors.Wrap(err, "configure MySQL migration connection")
	}
	pool := sql.OpenDB(connector)
	migrationError := backenddb.MigrateUp(pool)
	if err := errors.Join(migrationError, pool.Close()); err != nil {
		return ctxerrors.Wrap(err, "migrate MySQL backend")
	}
	return nil
}

func getWorkerName(options *options) string {
	if options.WorkerName != "" {
		return options.WorkerName
	}
	return "worker-" + uuid.NewString()
}
