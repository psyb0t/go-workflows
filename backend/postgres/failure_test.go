package postgres

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend/test"
	"github.com/psyb0t/go-workflows/diag"
)

func TestSQLStorageFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("external-backend integration requires the complete suite")
	}
	factory := func(t *testing.T) (diag.Backend, *sql.DB) {
		t.Helper()
		db, err := sql.Open("pgx", testInfra.PostgresDSN("postgres"))
		require.NoError(t, err)
		require.NoError(t, db.PingContext(context.Background()))

		database := "failure_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		_, err = db.ExecContext(context.Background(), "CREATE DATABASE "+database) // #nosec G202 -- Identifier is a fixed prefix plus a generated UUID.
		require.NoError(t, err)
		connection, err := sql.Open("pgx", testInfra.PostgresDSN(database))
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, connection.Close())
			_, err := db.ExecContext(context.Background(), "DROP DATABASE "+database+" WITH (FORCE)") // #nosec G202 -- Only the database created by this test.
			require.NoError(t, err)
			require.NoError(t, db.Close())
		})
		return NewPostgresBackendWithDB(connection), connection
	}
	test.SQLFailureTest(t, factory)
	test.SQLAtomicityTest(t, factory)
	test.SQLCorruptionTest(t, factory, false)
	test.SQLMutationTest(t, factory)
	test.SQLTableFaultTest(t, factory)
}
