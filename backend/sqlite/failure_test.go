package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend/test"
	"github.com/psyb0t/go-workflows/diag"
)

func TestSQLStorageFailures(t *testing.T) {
	t.Parallel()
	factory := func(t *testing.T) (diag.Backend, *sql.DB) {
		t.Helper()
		db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "workflows.db")+"?_txlock=immediate")
		require.NoError(t, err)
		db.SetMaxOpenConns(1)
		require.NoError(t, db.PingContext(context.Background()))

		t.Cleanup(func() { require.NoError(t, db.Close()) })
		return NewSqliteBackendWithDB(db), db
	}
	test.SQLFailureTest(t, factory)
	test.SQLAtomicityTest(t, factory)
	test.SQLCorruptionTest(t, factory, true)
	test.SQLMutationTest(t, factory)
	test.SQLTableFaultTest(t, factory)
	test.SQLRepositoryTest(t, factory, "sqlite")
}
