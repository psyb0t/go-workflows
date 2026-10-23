// Package samples provides development-only backend wiring for the feature demos.
// Use -backend memory for isolated single-process examples, or -backend sqlite
// for processes sharing a local file. The default Redis recreation path flushes
// ALL databases on localhost:6379. Never use it with shared or production storage.
package samples

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/psyb0t/go-workflows/internal/cleanup"

	redisv9 "github.com/redis/go-redis/v9"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/backend/mysql"
	postgres "github.com/psyb0t/go-workflows/backend/postgres"
	"github.com/psyb0t/go-workflows/backend/redis"
	"github.com/psyb0t/go-workflows/backend/sqlite"
	"github.com/psyb0t/go-workflows/diag"
)

// GetBackend parses demo CLI flags and opens storage using fixed local settings.
// It is not application configuration. In Redis mode it also starts diagnostics
// on loopback port 3000; demos with their own diagnostics should use another backend.
func GetBackend(name string, recreate bool, opt ...backend.BackendOption) backend.Backend {
	b := flag.String("backend", "redis", "backend to use: memory, sqlite, mysql, redis, postgres")
	flag.Parse()

	switch *b {
	case "memory":
		return sqlite.NewInMemoryBackend(sqlite.WithBackendOptions(opt...))

	case "sqlite":
		return sqlite.NewSqliteBackend(name+".sqlite", sqlite.WithBackendOptions(opt...))

	case "mysql":
		{
			// Create a new MySQL database
			db, err := sql.Open("mysql", "root:root@tcp(localhost:3306)/")
			if err != nil {
				panic(err)
			}

			_, err = db.Exec("CREATE DATABASE IF NOT EXISTS " + name)
			if err != nil {
				panic(err)
			}

			return mysql.NewMysqlBackend("localhost", 3306, "root", "root", name, mysql.WithBackendOptions(opt...))
		}

	case "postgres":
		{
			// Create a new Postgres database
			db, err := sql.Open("pgx", "host=localhost port=5432 user=root password=root dbname=postgres sslmode=disable")
			if err != nil {
				panic(err)
			}
			defer cleanup.Close(context.Background(), db)

			_, err = db.Exec("CREATE DATABASE " + name)
			if err != nil {
				panic(err)
			}

			return postgres.NewPostgresBackend("localhost", 5432, "root", "root", name, postgres.WithBackendOptions(opt...))
		}

	case "redis":
		rclient := redisv9.NewUniversalClient(&redisv9.UniversalOptions{
			Addrs:        []string{"localhost:6379"},
			Username:     "",
			Password:     "RedisPassw0rd",
			DB:           0,
			WriteTimeout: time.Second * 30,
			ReadTimeout:  time.Second * 30,
		})

		if recreate {
			if _, err := rclient.FlushAll(context.Background()).Result(); err != nil {
				panic(err)
			}
		}

		b, err := redis.NewRedisBackend(rclient, redis.WithBackendOptions(opt...))
		if err != nil {
			panic(err)
		}

		// Start diagnostic server under /diag
		m := http.NewServeMux()
		m.Handle("/diag/", http.StripPrefix("/diag", diag.NewServeMux(b)))
		server := &http.Server{Addr: "127.0.0.1:3000", Handler: m, ReadHeaderTimeout: 5 * time.Second}
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("diagnostics server failed: %v", err)
			}
		}()

		log.Println("Debug UI available at http://localhost:3000/diag")

		return b

	default:
		panic("unknown backend " + *b)
	}
}
