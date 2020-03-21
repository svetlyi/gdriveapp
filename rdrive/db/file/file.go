package file

import (
	"database/sql"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/contracts"
	"google.golang.org/api/drive/v3"
	"time"
)

type Repository struct {
	db  *sql.DB
	log contracts.Logger
}

func NewRepository(db *sql.DB, log contracts.Logger) Repository {
	return Repository{db: db, log: log}
}

// CreateFile creates a file. It means either it is a new file or it is the first
// launch of the application. Anyway, the fields remote_modification_time and
// last_remote_modification_time are the same.
func (fr Repository) CreateFile(file *drive.File) error {
	query := `
	INSERT INTO 
	files(
		'id',
		'name',
		'hash',
		'mime_type',
		'remote_modification_time', 
		'last_remote_modification_time',
		'shared',
		'root_folder', 
		'trashed', 
		'size'
	)
	VALUES (?,?,?,?,?,?,?,?,?,?)
	`
	insertStmt, err := fr.db.Prepare(query)
	if err == nil {
		defer insertStmt.Close()
		_, err = insertStmt.Exec(
			file.Id,
			file.Name,
			file.Md5Checksum,
			file.MimeType,
			file.ModifiedTime,
			file.ModifiedTime,
			file.Shared,
			0,
			file.Trashed,
			file.Size,
		)
		if err == nil {
			err = fr.linkWithParents(file)
		}
		return err
	}

	return err
}

func (fr Repository) SaveRootFolder(file *drive.File) error {
	query := `
	INSERT INTO 
	files('id', 'name', 'hash', 'mime_type', 'shared', 'root_folder', 'size') 
	VALUES (?,?,?,?,?,?,?)
	`
	insertStmt, err := fr.db.Prepare(query)
	if err == nil {
		defer insertStmt.Close()
		_, err = insertStmt.Exec(
			file.Id,
			file.Name,
			file.Md5Checksum,
			file.MimeType,
			file.Shared,
			1,
			file.Size,
		)
	}

	return err
}

func (fr *Repository) linkWithParents(file *drive.File) error {
	insertStmt, err := fr.db.Prepare(`INSERT INTO files_parents('file_id', 'parent_id') VALUES (?, ?)`)
	if err == nil {
		defer insertStmt.Close()
		for _, parent := range file.Parents {
			_, err = insertStmt.Exec(file.Id, parent)
			if err != nil {
				return err
			}
		}
	}

	return err
}

// getRootFolder gets the root folder. As in google drive as in Linux
// everything is a file, we return a file.
func (fr *Repository) GetRootFolder() (contracts.File, error) {
	selectRootStmt, err := fr.db.Prepare(
		`SELECT
				files.id,
				files.name,
				files.hash,
				files.download_time,
				files.remote_modification_time,
				files.last_remote_modification_time,
				files.mime_type,
				files.shared,
				files.root_folder,
				files.removed_remotely
			FROM files WHERE files.root_folder = 1 LIMIT 1`,
	)

	if nil != err {
		return contracts.File{}, err
	}

	return getOneFile(selectRootStmt)
}

// SetLastRemoteModificationDate updates last_remote_modification_time so that
// after we could check if it was changed remotely
func (fr *Repository) SetLastRemoteModificationDate(fileId string, date time.Time) error {
	query := `UPDATE files SET 'last_remote_modification_time' = ? WHERE id = ?`
	updateStmt, err := fr.db.Prepare(query)
	if err == nil {
		defer updateStmt.Close()
		_, err = updateStmt.Exec(date.Format(time.RFC3339), fileId)
	}

	return err
}

func (fr *Repository) SetRemovedRemotely(fileId string) error {
	query := `UPDATE files SET 'removed_remotely' = 1 WHERE id = ?`
	updateStmt, err := fr.db.Prepare(query)
	if err == nil {
		defer updateStmt.Close()
		_, err = updateStmt.Exec(fileId)
	}
	if err != nil {
		return errors.Wrap(err, "could not set removed_remotely")
	}

	return nil
}

func (fr *Repository) Delete(fileId string) error {
	query := `DELETE FROM files WHERE id = ?`
	updateStmt, err := fr.db.Prepare(query)
	if err == nil {
		defer updateStmt.Close()
		_, err = updateStmt.Exec(fileId)
	}
	if err != nil {
		return errors.Wrapf(err, "could not delete file %s from database", fileId)
	}

	return nil
}

func (fr *Repository) SetRemoteModificationDate(fileId string, date time.Time) error {
	query := `UPDATE files SET 'remote_modification_time' = ? WHERE id = ?`
	updateStmt, err := fr.db.Prepare(query)
	if err == nil {
		defer updateStmt.Close()
		_, err = updateStmt.Exec(date.Format(time.RFC3339), fileId)
	}

	return err
}

// SetDownloadTime updates download_time so that
// after we knew if the file was downloaded and if it was changed. download_time equals
// the last local modification time
func (fr *Repository) SetDownloadTime(fileId string, date time.Time) error {
	query := `UPDATE files SET 'download_time' = ? WHERE id = ?`
	updateStmt, err := fr.db.Prepare(query)
	if err == nil {
		defer updateStmt.Close()
		_, err = updateStmt.Exec(date.Format(time.RFC3339), fileId)
	}

	return errors.Wrap(err, "could not set download time")
}

// getRootFolder gets the root folder. As in google drive everything is a file,
// we return a file.
func (fr *Repository) GetFileById(id string) (contracts.File, error) {
	selectRootStmt, err := fr.db.Prepare(
		`SELECT
				files.id,
				files.name,
				files.hash,
				files.download_time,
				files.remote_modification_time,
				files.last_remote_modification_time,
				files.mime_type,
				files.shared,
				files.root_folder,
				files.removed_remotely
			FROM files WHERE files.id = ? LIMIT 1`,
	)

	if nil != err {
		fr.log.Debug("did not find file in db", struct {
			id string
		}{
			id: id,
		})
		return contracts.File{}, err
	}
	return getOneFile(selectRootStmt, id)
}

// HasTrashedParent determines if there is a trashed parent.
func (fr *Repository) HasTrashedParent(id string) (bool, error) {
	stmt, err := fr.db.Prepare(
		`WITH parents AS (
					SELECT f.id
					FROM files f
							 JOIN files_parents fp ON fp.parent_id = f.id
					WHERE file_id = ?
					UNION ALL
					SELECT fp.parent_id
					FROM parents fp_cte
					join files_parents fp where fp.file_id = fp_cte.id
				)
				select 1 from parents p join files f on f.id = p.id where f.trashed = 1 limit 1;`,
	)

	if nil != err {
		fr.log.Debug("could not prepare request for trashed parent", struct {
			id string
		}{
			id: id,
		})
		return false, err
	}

	defer stmt.Close()
	var hasBeenChanged bool

	if err := stmt.QueryRow(id).Scan(&hasBeenChanged); err == sql.ErrNoRows {
		return false, nil
	} else if err == nil {
		return true, nil
	} else {
		return false, err
	}
}

func getOneFile(stmt *sql.Stmt, args ...interface{}) (contracts.File, error) {
	var f contracts.File
	var DownloadTime interface{}
	var RemoteModificationTime interface{}
	var LastRemoteModificationTime interface{}
	err := stmt.QueryRow(args...).Scan(
		&f.Id,
		&f.Name,
		&f.Hash,
		&DownloadTime,
		&RemoteModificationTime,
		&LastRemoteModificationTime,
		&f.MimeType,
		&f.Shared,
		&f.RootFolder,
		&f.RemovedRemotely,
	)

	if err != nil {
		return contracts.File{}, err
	}
	defer stmt.Close()

	DownloadTime = parseTime(DownloadTime)
	RemoteModificationTime = parseTime(RemoteModificationTime)
	LastRemoteModificationTime = parseTime(LastRemoteModificationTime)

	return f, nil
}

func parseTime(t interface{}) time.Time {
	if t != nil {
		return t.(time.Time)
	} else {
		return time.Time{}
	}
}
