package db

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/svetlyi/gdriveapp/contracts"
	"github.com/svetlyi/gdriveapp/rdrive/db/migration"
	"os"
)

var db *sql.DB = nil

func New(dbPath string, logger contracts.Logger) *sql.DB {
	var err error

	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		logger.Error("Could not open the database file", err)
	}
	if err = migration.RunMigrations(db, logger); err != nil {
		logger.Error("Could not migrate", err)
		os.Exit(1)
	}

	return db
}
