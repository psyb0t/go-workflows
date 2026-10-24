// Package mysql opens shared SQL and GORM connections and runs versioned SQL migrations.
package mysql

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/psyb0t/common-go/db"
	"github.com/psyb0t/ctxerrors"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const (
	driverName             = "mysql"
	connectionTimeout      = 5 * time.Second
	connectionRetryTimeout = 30 * time.Second
)

// Mysql owns SQLDB and GormDB's shared pool. Close it once on shutdown.
type Mysql struct {
	SQLDB  *sql.DB
	GormDB *gorm.DB
	config Config
}

// New loads DB_MYSQL_* environment variables before opening the connection.
func New(ctx context.Context, ensureDBExists bool) (*Mysql, error) {
	cfg, err := parseConfig()
	if err != nil {
		return nil, ctxerrors.Wrap(err, "load MySQL config")
	}

	return NewWithConfig(ctx, cfg, ensureDBExists)
}

// NewWithConfig connects to an existing database, optionally creating it first.
// Database creation requires CREATE privileges; ordinary connections do not.
func NewWithConfig(ctx context.Context, cfg Config, ensureDBExists bool) (*Mysql, error) {
	if err := cfg.validate(); err != nil {
		return nil, ctxerrors.Wrap(err, "validate MySQL config")
	}

	if ensureDBExists {
		if err := ensureDatabaseExists(ctx, cfg); err != nil {
			return nil, ctxerrors.Wrap(err, "ensure MySQL database exists")
		}
	}

	sqlDB, err := connect(ctx, cfg, cfg.Database)
	if err != nil {
		return nil, ctxerrors.Wrap(err, "connect MySQL")
	}

	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{
		Conn: sqlDB,
	}), &gorm.Config{
		Logger:         db.NewGormSlogLogger(),
		TranslateError: true,
	})
	if err != nil {
		return nil, ctxerrors.Wrap(errors.Join(err, sqlDB.Close()), "open MySQL GORM connection")
	}

	return &Mysql{SQLDB: sqlDB, GormDB: gormDB, config: cfg}, nil
}

// Close releases the pool shared by SQLDB and GormDB.
func (m *Mysql) Close() error {
	if m == nil || m.SQLDB == nil {
		return nil
	}

	if err := m.SQLDB.Close(); err != nil {
		return ctxerrors.Wrap(err, "close MySQL")
	}

	return nil
}

func driverConfig(cfg Config, database string, multiStatements bool) *mysqldriver.Config {
	config := mysqldriver.NewConfig()
	config.Net = "tcp"
	config.Addr = net.JoinHostPort(cfg.Hostname, strconv.Itoa(cfg.Port))
	config.User = cfg.Username
	config.Passwd = cfg.Password
	config.DBName = database
	config.ParseTime = true
	config.Loc = time.UTC
	config.Timeout = connectionTimeout

	config.MultiStatements = multiStatements
	if cfg.IsSSL {
		config.TLSConfig = "true"
	}

	return config
}

func connect(ctx context.Context, cfg Config, database string) (*sql.DB, error) {
	connector, err := mysqldriver.NewConnector(driverConfig(cfg, database, false))
	if err != nil {
		return nil, ctxerrors.Wrap(err, "configure MySQL connector")
	}

	sqlDB := sql.OpenDB(connector)

	pingCtx, cancel := context.WithTimeout(ctx, connectionRetryTimeout)
	defer cancel()

	_, err = backoff.Retry(pingCtx, func() (bool, error) {
		if err := sqlDB.PingContext(pingCtx); err != nil {
			return false, ctxerrors.Wrap(err, "ping MySQL")
		}

		return true, nil
	}, backoff.WithMaxElapsedTime(connectionRetryTimeout))
	if err != nil {
		return nil, ctxerrors.Wrap(errors.Join(err, sqlDB.Close()), "connect MySQL pool")
	}

	return sqlDB, nil
}

func ensureDatabaseExists(ctx context.Context, cfg Config) error {
	sqlDB, err := connect(ctx, cfg, "")
	if err != nil {
		return ctxerrors.Wrap(err, "connect MySQL server for database creation")
	}

	// Identifiers cannot be bound. Quote embedded backticks; never interpolate credentials.
	query := "CREATE DATABASE IF NOT EXISTS `" + strings.ReplaceAll(cfg.Database, "`", "``") + "`"

	_, executionErr := sqlDB.ExecContext(ctx, query)
	if err := errors.Join(executionErr, sqlDB.Close()); err != nil {
		return ctxerrors.Wrap(err, "create MySQL database")
	}

	return nil
}
