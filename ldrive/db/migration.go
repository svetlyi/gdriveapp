package db

import (
	"database/sql"
	"github.com/svetlyi/gdriveapp/contracts"
	"github.com/svetlyi/gdriveapp/db/migration"
)

var migrations []migration.Migration

func Migrate(db *sql.DB, logger contracts.Logger) error {
	migrations = append(
		migrations,
		migration.Migration{
			Id: "create_files_table",
			Query: `
				CREATE TABLE IF NOT EXISTS ldrive_files (
					relative_path VARCHAR(255) PRIMARY KEY,
					prev_hash VARCHAR(255) DEFAULT "",
					prev_mod_time DATETIME,
					prev_size INTEGER,
					removed SMALLINT DEFAULT 0
				)
			`,
		},
	)

	return migration.RunMigrations("ldrive", migrations, db, logger)
}
