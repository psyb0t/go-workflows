package mysql

import (
	"strings"

	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/ctxerrors/commerr"
	"github.com/psyb0t/gonfiguration"
)

const (
	maxPort                   = 65535
	maxDatabaseNameCharacters = 64
)

// Config configures a TCP connection. IsSSL requires verified TLS, without a
// plaintext fallback. Password is optional for servers using passwordless users.
type Config struct {
	Hostname string `env:"DB_MYSQL_HOSTNAME"`
	Port     int    `default:"3306"          env:"DB_MYSQL_PORT"`
	Username string `env:"DB_MYSQL_USERNAME"`
	Password string `env:"DB_MYSQL_PASSWORD"`
	Database string `env:"DB_MYSQL_DATABASE"`
	IsSSL    bool   `default:"true"          env:"DB_MYSQL_ISSSL"`
}

func (c Config) validate() error {
	if strings.TrimSpace(c.Hostname) == "" || c.Username == "" || c.Database == "" {
		return ctxerrors.Wrap(commerr.ErrRequiredFieldNotSet, "MySQL hostname, username and database are required")
	}

	if c.Port < 1 || c.Port > maxPort {
		return ctxerrors.Wrap(commerr.ErrInvalidArgument, "MySQL port must be between 1 and 65535")
	}

	if strings.ContainsRune(c.Database, '\x00') || len([]rune(c.Database)) > maxDatabaseNameCharacters {
		return ctxerrors.Wrap(commerr.ErrInvalidArgument, "MySQL database name must contain at most 64 characters and no NUL")
	}

	return nil
}

func parseConfig() (Config, error) {
	var cfg Config
	if err := gonfiguration.Parse(&cfg); err != nil {
		return Config{}, ctxerrors.Wrap(err, "parse MySQL config")
	}

	return cfg, nil
}
