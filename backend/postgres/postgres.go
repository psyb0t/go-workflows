package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/psyb0t/common-go/db/postgresql"
	"github.com/psyb0t/ctxerrors"

	"github.com/psyb0t/go-workflows/backend"
	backenddb "github.com/psyb0t/go-workflows/backend/postgres/db"
)

type postgresBackend struct {
	*backenddb.Store
	db             *sql.DB
	workerName     string
	options        *options
	ownsConnection bool
}

var _ backend.Backend = (*postgresBackend)(nil)

func NewPostgresBackend(host string, port int, user, password, database string, opts ...option) *postgresBackend {
	connection, err := postgresql.NewWithConfig(context.Background(), postgresql.Config{
		Hostname: host, Port: port, Username: user, Password: password, Database: database,
	}, false)
	if err != nil {
		panic(ctxerrors.Wrap(err, "open PostgreSQL backend"))
	}
	return wrapPool(connection.SQLDB, true, opts...)
}

// NewPostgresBackendWithDB leaves connection configuration and ownership with the caller.
// Automatic migrations are disabled by default.
func NewPostgresBackendWithDB(pool *sql.DB, opts ...option) *postgresBackend {
	return wrapPool(pool, false, opts...)
}

func wrapPool(pool *sql.DB, owned bool, opts ...option) *postgresBackend {
	settings := &options{Options: backend.ApplyOptions(), ApplyMigrations: owned}
	for _, option := range opts {
		option(settings)
	}
	if owned && settings.PostgresOptions != nil {
		settings.PostgresOptions(pool)
	}
	name := getWorkerName(settings)
	store, err := backenddb.New(pool, name, settings.Options)
	if err != nil {
		if owned {
			err = errors.Join(err, pool.Close())
		}
		panic(ctxerrors.Wrap(err, "initialize PostgreSQL backend"))
	}
	b := &postgresBackend{Store: store, db: pool, workerName: name, options: settings, ownsConnection: owned}
	if settings.ApplyMigrations {
		if err := b.Migrate(); err != nil {
			if owned {
				err = errors.Join(err, pool.Close())
			}
			panic(ctxerrors.Wrap(err, "initialize PostgreSQL migrations"))
		}
	}
	return b
}

func (b *postgresBackend) Close() error {
	if !b.ownsConnection {
		return nil
	}
	if err := b.db.Close(); err != nil {
		return ctxerrors.Wrap(err, "close PostgreSQL backend")
	}
	return nil
}

// Migrate applies the existing SQL history without closing caller pools.
func (b *postgresBackend) Migrate() error { return backenddb.MigrateUp(b.db) }

func getWorkerName(options *options) string {
	if options.WorkerName != "" {
		return options.WorkerName
	}
	return "worker-" + uuid.NewString()
}
