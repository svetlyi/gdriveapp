package app

import (
	"database/sql"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/contracts"
)

const NextChangeToken = "next_change_token"

// Store is a storage for application settings,
// that is stored in db
type Store struct {
	db  *sql.DB
	log contracts.Logger
}

func New(db *sql.DB, log contracts.Logger) Store {
	return Store{db: db, log: log}
}

func (fr Store) createSetting(setting string, value string) error {
	fr.log.Debug("creating setting", struct {
		setting string
		value   string
	}{setting, value})
	query := `
	INSERT INTO 
	app_state(
		'setting',
		'value'
	)
	VALUES (?,?)
	`
	insertStmt, err := fr.db.Prepare(query)
	if err == nil {
		defer insertStmt.Close()
		_, err = insertStmt.Exec(
			setting,
			value,
		)
		return err
	}

	return err
}

func (fr *Store) Get(setting string) (string, error) {
	row := fr.db.QueryRow(
		`SELECT
				app_state.value
			FROM app_state WHERE app_state.setting = ? LIMIT 1`,
		setting,
	)

	var token string

	if err := row.Scan(&token); err != nil {
		return "", errors.Wrap(err, "could not scan setting "+setting)
	}

	return token, nil
}

func (fr *Store) Set(setting string, value string) error {
	fr.log.Debug("updating setting", struct {
		setting string
		value   string
	}{setting, value})
	// check if the setting exists. If it does, we create it, otherwise, update it
	_, err := fr.Get(setting)
	if errors.Cause(err) == sql.ErrNoRows {
		return fr.createSetting(setting, value)
	} else if err != nil {
		return err
	}

	query := `UPDATE app_state SET 'value' = ? WHERE app_state.setting = ?`
	updateStmt, err := fr.db.Prepare(query)
	if err == nil {
		defer updateStmt.Close()
		_, err = updateStmt.Exec(value, setting)
	}

	return err
}
