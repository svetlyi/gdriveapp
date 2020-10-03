package repository

import (
	"database/sql"
	"fmt"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/contracts"
	"github.com/svetlyi/gdriveapp/ldrive/file"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// fileSelectFields contains all the fields from the "files" table. We are going to
// select all of them most of the time. So, they are shared between methods
var fileSelectFields = `
    ldrive_files.relative_path,
    ldrive_files.prev_hash,
    ldrive_files.prev_mod_time,
    ldrive_files.prev_size,
    ldrive_files.removed
`

type Repository struct {
	db        *sql.DB
	log       contracts.Logger
	drivePath string
}

func New(db *sql.DB, log contracts.Logger, drivePath string) Repository {
	return Repository{
		db:        db,
		log:       log,
		drivePath: drivePath,
	}
}

func (fr *Repository) CreateFile(relPath string, modTime time.Time, size uint64) (contracts.File, error) {
	relPath = trimPath(relPath)
	f := file.New(relPath, "", false, modTime, size, fr.drivePath)

	query := `INSERT INTO 
				ldrive_files('relative_path', 'prev_hash', 'prev_mod_time', 'prev_size', 'removed') 
				VALUES (?, ?, ?, ?, 0)`
	hash, err := f.GetHash()
	if err != nil {
		return nil, err
	}
	_, err = fr.db.Exec(query, relPath, hash, modTime.Format(time.RFC3339Nano), size)

	if err != nil {
		return nil, errors.Wrapf(err, "could not insert data for file %s", relPath)
	}
	return f, nil
}

// Update updated the data in a local database. modTime in fact is download time
func (fr *Repository) Update(f contracts.File, modTime time.Time, size uint64) error {
	hash, err := f.GetHash()
	if err != nil {
		return errors.Wrap(err, "could not get hash")
	}
	tx, err := fr.db.Begin()
	if err != nil {
		return errors.Wrap(err, "could not start transaction")
	}
	cleanRelPath := trimPath(f.GetRelativePath())
	res, err := tx.Exec(
		`UPDATE 'ldrive_files'
			SET 
			'relative_path' = ?,
			'prev_hash' = ?,
			'prev_mod_time' = ?,
			'prev_size' = ?,
			'removed' = ?
			WHERE 'ldrive_files'.relative_path = ?`,
		cleanRelPath,
		hash,
		modTime.Format(time.RFC3339Nano),
		size,
		0,
		cleanRelPath,
	)
	if err != nil {
		err = errors.Wrap(err, "could not update ldrive_files")
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			err = errors.Wrapf(rollbackErr, "could not rollback")
		}
		return err
	}
	rows, err := res.RowsAffected()
	if rows != 1 {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			err = errors.Wrapf(rollbackErr, "could not rollback")
		}
		return errors.Wrapf(err, "affected too many files")
	}
	return tx.Commit()
}

// Rename updates the file's relative path
func (fr *Repository) Rename(f contracts.File, newFullPath string, modTime time.Time) error {
	cleanRelPath := trimPath(f.GetRelativePath())
	cleanNewFullPath := trimPath(newFullPath)
	cleanNewRelPath := trimPath(cleanNewFullPath[len(fr.drivePath):])
	tx, err := fr.db.Begin()
	if err != nil {
		return errors.Wrap(err, "could not start transaction")
	}
	res, err := tx.Exec(
		`UPDATE 'ldrive_files'
			SET 
			'relative_path' = ?,
			'prev_mod_time' = ?
			WHERE 'ldrive_files'.relative_path = ?`,
		cleanNewRelPath,
		modTime.Format(time.RFC3339Nano),
		cleanRelPath,
	)
	if err != nil {
		return errors.Wrap(err, "could not update update relative path")
	}
	rows, err := res.RowsAffected()
	if rows != 1 {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			err = errors.Wrapf(rollbackErr, "could not rollback")
		}
		return errors.Wrapf(err, "affected too many files")
	}
	return tx.Commit()
}

func (fr *Repository) GetFileByRelPath(path string) (contracts.File, error) {
	path = trimPath(path)
	row := fr.db.QueryRow(
		fmt.Sprintf(
			`SELECT
			%s 
			FROM ldrive_files 
			WHERE ldrive_files.relative_path = ?
			LIMIT 1`,
			fileSelectFields,
		),
		path,
	)
	return fr.parseFileFromRow(row)
}

// getRootFolder gets the root folder
func (fr *Repository) GetRootFolder() (contracts.File, error) {
	row := fr.db.QueryRow(
		fmt.Sprintf(
			`SELECT
			%s 
			FROM ldrive_files 
			WHERE instr(trim(ldrive_files.relative_path, ?), ?) = 0
			LIMIT 1`,
			fileSelectFields,
		),
		string(os.PathSeparator),
		string(os.PathSeparator),
	)
	return fr.parseFileFromRow(row)
}

func (fr *Repository) parseFileFromRow(row contracts.RowScanner) (f contracts.File, err error) {
	var prevRawModTime interface{}

	var (
		relPath     string
		prevHash    string
		prevModTime time.Time
		sizeBytes   uint64
		removed     uint8
	)
	err = row.Scan(
		&relPath,
		&prevHash,
		&prevRawModTime,
		&sizeBytes,
		&removed,
	)

	if err == nil {
		prevModTime = parseTime(prevRawModTime)
	} else {
		return nil, errors.Wrap(err, "could not scan local file data from db")
	}

	return file.New(
		relPath,
		prevHash,
		removed == 1,
		prevModTime,
		sizeBytes,
		fr.drivePath,
	), nil
}

func parseTime(t interface{}) time.Time {
	if t != nil {
		return t.(time.Time)
	} else {
		return time.Time{}
	}
}

func trimPath(path string) string {
	return filepath.Clean(strings.Trim(path, string(os.PathSeparator)))
}
