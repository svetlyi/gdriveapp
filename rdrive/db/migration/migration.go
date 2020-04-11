package migration

import (
	"database/sql"
	"fmt"
	"github.com/svetlyi/gdriveapp/contracts"
)

var queries []string

func init() {
	queries = append(queries, `
CREATE TABLE IF NOT EXISTS files (
	id VARCHAR(255) PRIMARY KEY,
	prev_remote_name VARCHAR(255),
	cur_remote_name VARCHAR(255),
	hash VARCHAR(255),
	download_time DATETIME,
	prev_remote_modification_time DATETIME,
	cur_remote_modification_time DATETIME,
	mime_type VARCHAR(255),
	shared SMALLINT,
	root_folder SMALLINT,
	size INTEGER,
	trashed SMALLINT DEFAULT 0,
	removed_remotely SMALLINT DEFAULT 0,
	removed_locally SMALLINT DEFAULT 0
)
`)
	queries = append(queries, `
CREATE TABLE IF NOT EXISTS files_parents (
	file_id VARCHAR(255),
	prev_parent_id VARCHAR(255),
	cur_parent_id VARCHAR(255)
)
`)
	queries = append(queries, `
CREATE TABLE IF NOT EXISTS app_state (
	setting VARCHAR(255) PRIMARY KEY,
	value VARCHAR(255)
)
`)
}

func RunMigrations(db *sql.DB, logger contracts.Logger) error {
	for _, query := range queries {
		if _, err := (*db).Exec(query); err != nil {
			logger.Error(fmt.Sprintf("%q: %s\n", err, query))
			return err
		}
	}
	logger.Info("Migrated successfully")

	return nil
}
