// Command repogen generates repositories from each backend's existing schema models.
package main

import (
	"flag"
	"fmt"
	"os"

	"gorm.io/gen"

	mysqlmodels "github.com/psyb0t/go-workflows/backend/mysql/db/models"
	postgresmodels "github.com/psyb0t/go-workflows/backend/postgres/db/models"
	sqlitemodels "github.com/psyb0t/go-workflows/backend/sqlite/db/models"
)

func main() {
	dialect := flag.String("dialect", "", "sqlite, mysql or postgres")
	output := flag.String("out", ".", "repository output directory")
	flag.Parse()
	generator := gen.NewGenerator(gen.Config{
		OutPath: *output,
		OutFile: "repositories.gen.go",
		Mode:    gen.WithoutContext | gen.WithQueryInterface,
	})
	switch *dialect {
	case "sqlite":
		generator.ApplyBasic(
			sqlitemodels.Instance{},
			sqlitemodels.PendingEvent{},
			sqlitemodels.HistoryEvent{},
			sqlitemodels.Activity{},
			sqlitemodels.Attribute{},
		)
		generator.ApplyInterface(func(SqliteEventQuerier) {}, sqlitemodels.PendingEvent{}, sqlitemodels.HistoryEvent{})
		generator.ApplyInterface(func(SqliteInstanceQueries) {}, sqlitemodels.Instance{})
		generator.ApplyInterface(func(SQLiteInstanceQuerier) {}, sqlitemodels.Instance{})
	case "mysql":
		generator.ApplyBasic(
			mysqlmodels.Instance{},
			mysqlmodels.PendingEvent{},
			mysqlmodels.HistoryEvent{},
			mysqlmodels.Activity{},
			mysqlmodels.Attribute{},
		)
		generator.ApplyInterface(func(MysqlEventQuerier) {}, mysqlmodels.PendingEvent{}, mysqlmodels.HistoryEvent{})
		generator.ApplyInterface(func(MysqlInstanceQueries) {}, mysqlmodels.Instance{})
		generator.ApplyInterface(func(ServerInstanceQuerier) {}, mysqlmodels.Instance{})
	case "postgres":
		generator.ApplyBasic(
			postgresmodels.Instance{},
			postgresmodels.PendingEvent{},
			postgresmodels.HistoryEvent{},
			postgresmodels.Activity{},
			postgresmodels.Attribute{},
		)
		generator.ApplyInterface(func(PostgresEventQuerier) {}, postgresmodels.PendingEvent{}, postgresmodels.HistoryEvent{})
		generator.ApplyInterface(func(PostgresInstanceQueries) {}, postgresmodels.Instance{})
		generator.ApplyInterface(func(ServerInstanceQuerier) {}, postgresmodels.Instance{})
	default:
		fmt.Fprintln(os.Stderr, "repogen requires -dialect=sqlite, mysql or postgres")
		os.Exit(2)
	}
	generator.Execute()
}
