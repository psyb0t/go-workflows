package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend/test"
	"github.com/psyb0t/go-workflows/diag"
)

func TestSQLStorageFailures(t *testing.T) {
	t.Parallel()
	factory := func(t *testing.T) (diag.Backend, *sql.DB) {
		t.Helper()
		db, err := sql.Open("sqlite", "file:"+uuid.NewString()+"?mode=memory&cache=shared")
		require.NoError(t, err)
		require.NoError(t, db.PingContext(context.Background()))

		t.Cleanup(func() { require.NoError(t, db.Close()) })
		return NewSqliteBackendWithDB(db), db
	}
	test.SQLFailureTest(t, factory)
	test.SQLAtomicityTest(t, factory)
	test.SQLCorruptionTest(t, factory, true)
	test.SQLMutationTest(t, factory)
	test.SQLTableFaultTest(t, factory)
}
