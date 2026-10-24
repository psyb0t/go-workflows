package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/psyb0t/go-workflows/internal/cleanup"

	"github.com/google/uuid"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/backend/test"
	"github.com/psyb0t/go-workflows/tests/testinfra"
)

var testInfra *testinfra.Infra

func TestMain(m *testing.M) {
	testinfra.Exit(m, testinfra.MySQL, &testInfra)
}

// Creating and dropping databases is terribly inefficient, but easiest for complete test isolation. For
// the future consider nested transactions, or manually TRUNCATE-ing the tables in-between tests.

func Test_MysqlBackend(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	var dbName string

	test.BackendTest(t, func(options ...backend.BackendOption) test.TestBackend {
		db, err := sql.Open("mysql", testInfra.MySQLDSN(""))
		if err != nil {
			panic(err)
		}

		dbName = "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := db.Exec("CREATE DATABASE " + dbName); err != nil {
			panic(fmt.Errorf("creating database: %w", err))
		}

		if err := db.Close(); err != nil {
			panic(err)
		}

		options = append(options, backend.WithStickyTimeout(0))

		return NewMysqlBackend(testInfra.Host, testInfra.Port, testInfra.User, testInfra.Password, dbName, WithBackendOptions(options...))
	}, func(b test.TestBackend) {
		if err := b.(*mysqlBackend).db.Close(); err != nil {
			panic(err)
		}

		db, err := sql.Open("mysql", testInfra.MySQLDSN(""))
		if err != nil {
			panic(err)
		}

		if _, err := db.Exec("DROP DATABASE IF EXISTS " + dbName); err != nil {
			panic(fmt.Errorf("dropping database: %w", err))
		}

		if err := db.Close(); err != nil {
			panic(err)
		}
	})
}

func TestMySqlBackendE2E(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	var dbName string

	test.EndToEndBackendTest(t, func(options ...backend.BackendOption) test.TestBackend {
		db, err := sql.Open("mysql", testInfra.MySQLDSN(""))
		if err != nil {
			panic(err)
		}

		dbName = "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := db.Exec("CREATE DATABASE " + dbName); err != nil {
			panic(fmt.Errorf("creating database: %w", err))
		}

		if err := db.Close(); err != nil {
			panic(err)
		}

		options = append(options, backend.WithStickyTimeout(0))

		return NewMysqlBackend(testInfra.Host, testInfra.Port, testInfra.User, testInfra.Password, dbName, WithBackendOptions(options...))
	}, func(b test.TestBackend) {
		if err := b.(*mysqlBackend).db.Close(); err != nil {
			panic(err)
		}

		db, err := sql.Open("mysql", testInfra.MySQLDSN(""))
		if err != nil {
			panic(err)
		}

		if _, err := db.Exec("DROP DATABASE IF EXISTS " + dbName); err != nil {
			panic(fmt.Errorf("dropping database: %w", err))
		}

		if err := db.Close(); err != nil {
			panic(err)
		}
	})
}

var _ test.TestBackend = (*mysqlBackend)(nil)

func Test_MysqlBackend_WorkerName(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Run("DefaultWorkerName", func(t *testing.T) {
		// Create a backend without specifying worker name
		// Since we can't connect to MySQL without it being available, we'll test the getWorkerName function directly
		options := &options{
			Options: backend.ApplyOptions(),
		}
		workerName := getWorkerName(options)

		// The default worker name should be in the format "worker-<uuid>"
		if !strings.Contains(workerName, "worker-") {
			t.Errorf("Expected worker name to contain 'worker-', got: %s", workerName)
		}
		if len(workerName) != 43 { // "worker-" (7) + UUID (36)
			t.Errorf("Expected worker name length to be 43, got: %d", len(workerName))
		}
	})

	t.Run("CustomWorkerName", func(t *testing.T) {
		customWorkerName := "test-worker-123"
		options := &options{
			Options: backend.ApplyOptions(backend.WithWorkerName(customWorkerName)),
		}
		workerName := getWorkerName(options)

		if workerName != customWorkerName {
			t.Errorf("Expected worker name to be '%s', got: %s", customWorkerName, workerName)
		}
	})

	t.Run("EmptyWorkerNameUsesDefault", func(t *testing.T) {
		options := &options{
			Options: backend.ApplyOptions(backend.WithWorkerName("")),
		}
		workerName := getWorkerName(options)

		// Empty worker name should fall back to UUID generation
		if !strings.Contains(workerName, "worker-") {
			t.Errorf("Expected worker name to contain 'worker-', got: %s", workerName)
		}
		if len(workerName) != 43 { // "worker-" (7) + UUID (36)
			t.Errorf("Expected worker name length to be 43, got: %d", len(workerName))
		}
	})
}

func Test_MysqlBackendWithDB(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Run("UsesProvidedConnection", func(t *testing.T) {
		// Create database for test
		adminDB, err := sql.Open("mysql", testInfra.MySQLDSN(""))
		if err != nil {
			t.Fatal(err)
		}

		dbName := "test_withdb_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := adminDB.Exec("CREATE DATABASE " + dbName); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := adminDB.Exec("DROP DATABASE IF EXISTS " + dbName); err != nil {
				t.Errorf("unexpected operation error: %v", err)
			}
			if err := adminDB.Close(); err != nil {
				t.Errorf("unexpected operation error: %v", err)
			}
		}()

		// Create our own connection to the test database
		dsn := testInfra.MySQLDSN(dbName)
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup.Close(context.Background(), db)

		// Create backend with existing connection and migration DSN
		migrationDSN := dsn + "&multiStatements=true"
		backend := NewMysqlBackendWithDB(db,
			WithApplyMigrations(true),
			WithMigrationDSN(migrationDSN),
		)

		// Verify the backend uses our connection
		if backend.db != db {
			t.Error("Backend should use provided db connection")
		}
		if backend.ownsConnection {
			t.Error("Backend should not own the connection")
		}

		// Close backend - should NOT close our connection
		if err := backend.Close(); err != nil {
			t.Fatal(err)
		}

		// Verify our connection is still usable
		if err := db.Ping(); err != nil {
			t.Errorf("Connection should still be open after backend.Close(): %v", err)
		}
	})

	t.Run("MigrationsDisabledByDefault", func(t *testing.T) {
		// Create database for test
		adminDB, err := sql.Open("mysql", testInfra.MySQLDSN(""))
		if err != nil {
			t.Fatal(err)
		}

		dbName := "test_withdb2_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := adminDB.Exec("CREATE DATABASE " + dbName); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := adminDB.Exec("DROP DATABASE IF EXISTS " + dbName); err != nil {
				t.Errorf("unexpected operation error: %v", err)
			}
			if err := adminDB.Close(); err != nil {
				t.Errorf("unexpected operation error: %v", err)
			}
		}()

		dsn := testInfra.MySQLDSN(dbName)
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup.Close(context.Background(), db)

		// Create backend without enabling migrations
		backend := NewMysqlBackendWithDB(db)
		defer cleanup.Close(context.Background(), backend)

		// Tables should not exist since migrations weren't applied
		_, err = db.Exec("SELECT 1 FROM instances LIMIT 1")
		if err == nil {
			t.Error("Expected error because table should not exist")
		}
	})

	t.Run("MigrationFailsWithoutDSN", func(t *testing.T) {
		// Create database for test
		adminDB, err := sql.Open("mysql", testInfra.MySQLDSN(""))
		if err != nil {
			t.Fatal(err)
		}

		dbName := "test_withdb3_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := adminDB.Exec("CREATE DATABASE " + dbName); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := adminDB.Exec("DROP DATABASE IF EXISTS " + dbName); err != nil {
				t.Errorf("unexpected operation error: %v", err)
			}
			if err := adminDB.Close(); err != nil {
				t.Errorf("unexpected operation error: %v", err)
			}
		}()

		dsn := testInfra.MySQLDSN(dbName)
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup.Close(context.Background(), db)

		// Create backend without migration DSN - should panic when trying to migrate
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when ApplyMigrations=true without MigrationDSN")
			}
		}()

		NewMysqlBackendWithDB(db, WithApplyMigrations(true))
	})
}
