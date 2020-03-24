package file

import (
	"database/sql"
	"fmt"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/contracts"
	"google.golang.org/api/drive/v3"
	"time"
)

type Repository struct {
	db  *sql.DB
	log contracts.Logger
}

// fileSelectFields contains all the fields from the "files" table. We are going to
// select all of them most of the time. So, they are shared between methods
var fileSelectFields = `
    files.id,
    files.prev_remote_name,
    files.cur_remote_name,
    files.hash,
    files.download_time,
    files.prev_remote_modification_time,
    files.cur_remote_modification_time,
    files.mime_type,
    files.shared,
    files.root_folder,
    files.size,
    files.trashed,
    files.removed_remotely
`

func NewRepository(db *sql.DB, log contracts.Logger) Repository {
	return Repository{db: db, log: log}
}

// CreateFile creates a file. It means either it is a new file or it is the first
// launch of the application. Anyway, the fields prev_remote_modification_time and
// cur_remote_modification_time are the same.
func (fr Repository) CreateFile(file *drive.File) error {
	query := `
	INSERT INTO 
	files(
		id,
		prev_remote_name,
		cur_remote_name,
		hash,
		prev_remote_modification_time,
		cur_remote_modification_time,
		mime_type,
		shared,
		root_folder,
		'size',
		trashed,
		removed_remotely
	)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
	`
	insertStmt, err := fr.db.Prepare(query)
	if err == nil {
		defer insertStmt.Close()
		_, err = insertStmt.Exec(
			file.Id,
			file.Name,
			file.Name,
			file.Md5Checksum,
			file.ModifiedTime,
			file.ModifiedTime,
			file.MimeType,
			file.Shared,
			0,
			file.Size,
			file.Trashed,
			0,
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
	files(
		id,
		prev_remote_name,
		cur_remote_name,
		hash,
		mime_type,
		shared,
		root_folder,
		'size',
		trashed,
		removed_remotely
	)
	VALUES (?,?,?,?,?,?,?,?,?,?)
	`
	insertStmt, err := fr.db.Prepare(query)
	if err == nil {
		defer insertStmt.Close()
		_, err = insertStmt.Exec(
			file.Id,
			file.Name,
			file.Name,
			file.Md5Checksum,
			file.MimeType,
			file.Shared,
			1,
			file.Size,
			0,
			0,
		)
	}

	return err
}

func (fr *Repository) linkWithParents(file *drive.File) error {
	if len(file.Parents) > 1 {
		return errors.New("there is no support for multiple parents yet")
	}
	if len(file.Parents) == 0 {
		return nil
	}
	insertStmt, err := fr.db.Prepare(
		`INSERT INTO 
				files_parents('file_id', 'prev_parent_id', 'cur_parent_id') 
				VALUES (?, ?, ?)`,
	)
	if err == nil {
		defer insertStmt.Close()
		_, err = insertStmt.Exec(file.Id, file.Parents[0], file.Parents[0])
	}

	return err
}

// getRootFolder gets the root folder. As in google drive as in Linux
// everything is a file, we return a file.
func (fr *Repository) GetRootFolder() (contracts.File, error) {
	selectRootStmt, err := fr.db.Prepare(
		fmt.Sprintf(`SELECT %s FROM files WHERE files.root_folder = 1 LIMIT 1`, fileSelectFields),
	)

	if nil != err {
		return contracts.File{}, err
	}

	return getOneFile(selectRootStmt)
}

// SetCurRemoteData updates cur_remote_modification_time and other data so that
// after we could check if it was changed remotely
func (fr *Repository) SetCurRemoteData(fileId string, mtime time.Time, name string, parents []string) error {
	if len(parents) > 1 {
		return errors.New("there is no support for multiple parents yet")
	}

	if err := fr.setFileCurRemoteData(fileId, mtime, parents[0]); err != nil {
		return err
	}
	if err := fr.setCurRemoteFileParent(fileId, parents[0]); err != nil {
		return err
	}

	return nil
}

func (fr *Repository) setFileCurRemoteData(fileId string, mtime time.Time, name string) error {
	query := `UPDATE files SET 'cur_remote_modification_time' = ?, 'cur_remote_name' = ? WHERE id = ?`
	updateStmt, err := fr.db.Prepare(query)
	if err == nil {
		defer updateStmt.Close()
		_, err = updateStmt.Exec(mtime.Format(time.RFC3339), name, fileId)
	}
	if err != nil {
		err = errors.Wrapf(err, "could not update file's %s data", fileId)
	}
	return err
}
func (fr *Repository) setCurRemoteFileParent(fileId string, parentId string) error {
	query := `UPDATE files_parents SET 'cur_parent_id' = ? WHERE file_id = ?`
	updateStmt, err := fr.db.Prepare(query)
	if err == nil {
		defer updateStmt.Close()
		_, err = updateStmt.Exec(parentId, fileId)
	}
	if err != nil {
		err = errors.Wrapf(err, "could not update file's %s parent data", fileId)
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
	query := `UPDATE files SET 'prev_remote_modification_time' = ? WHERE id = ?`
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
		fmt.Sprintf(`SELECT %s FROM files WHERE files.id = ? LIMIT 1`, fileSelectFields),
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

// GetFilePath gets the full file path for the file with the provided id
func (fr *Repository) GetFilePath(id string) (curPath string, prevPath string, err error) {
	selectPathStmt, err := fr.db.Prepare(`
		WITH get_prev_parents (parent_id, name) AS (
			SELECT fp.prev_parent_id, f.prev_remote_name
			FROM files f
					 JOIN files_parents fp ON f.id = fp.file_id
			WHERE id = ?
			UNION ALL
			SELECT fp.prev_parent_id, f.prev_remote_name
			FROM get_prev_parents gp
					 JOIN files f ON gp.parent_id = f.id
					 JOIN files_parents fp ON f.id = fp.file_id
		),
			 get_cur_parents (parent_id, name) AS (
				 SELECT fp.cur_parent_id, f.cur_remote_name
				 FROM files f
						  JOIN files_parents fp ON f.id = fp.file_id
				 WHERE id = ?
				 UNION ALL
				 SELECT fp.cur_parent_id, f.cur_remote_name
				 FROM get_cur_parents gp
						  JOIN files f ON gp.parent_id = f.id
						  JOIN files_parents fp ON f.id = fp.file_id
			 )
		select *
		from (SELECT group_concat(gpp_f.prev_remote_name, '/') as prevPath
			  FROM get_prev_parents gpp
					   join files gpp_f on gpp.parent_id = gpp_f.id)
				 join (SELECT group_concat(gcp_f.cur_remote_name, '/') as curPath
					   FROM get_cur_parents gcp
								join files gcp_f on gcp.parent_id = gcp_f.id);
	`)

	if nil != err {
		err = errors.Wrap(err, "could not prepare selectPathStmt")
		return
	}
	defer selectPathStmt.Close()

	err = selectPathStmt.QueryRow(id, id).Scan(&prevPath, &curPath)
	return
}

func (fr *Repository) GetCurFilesListByParent(parentId string) ([]contracts.File, error) {
	var filesList []contracts.File

	fr.log.Debug("getting files by parent", struct {
		parentId string
	}{
		parentId: parentId,
	})
	selectFilesStmt, err := fr.db.Prepare(
		fmt.Sprintf(`
			SELECT %s
			FROM files 
			JOIN files_parents fp ON files.id = fp.file_id 
			WHERE fp.cur_parent_id=?`,
			fileSelectFields,
		),
	)
	if nil != err {
		return filesList, errors.Wrap(err, "Error preparing statement for selecting files.")
	}
	defer selectFilesStmt.Close()
	rows, err := selectFilesStmt.Query(parentId)
	if nil != err {
		return filesList, errors.Wrap(err, "Error querying files by parent id.")
	}
	defer rows.Close()
	for rows.Next() {
		var f contracts.File

		if f, err = parseFileFromRow(rows); err == nil {
			f.CurPath, f.PrevPath, err = fr.GetFilePath(f.Id)
			if err != nil {
				return filesList, errors.Wrapf(err, "Could not get full path for file %s", f.Id)
			}
			filesList = append(filesList, f)
		} else {
			return filesList, errors.Wrap(err, "Error looping over files in getFilesList.")
		}
	}
	if err = rows.Err(); err != nil {
		return filesList, errors.Wrap(err, "Error fetching files by parent id.")
	}
	return filesList, nil
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

	if err := stmt.QueryRow(id).Scan(&hasBeenChanged); errors.Cause(err) == sql.ErrNoRows {
		return false, nil
	} else if err == nil {
		return true, nil
	} else {
		return false, err
	}
}

func getOneFile(stmt *sql.Stmt, args ...interface{}) (f contracts.File, err error) {
	row := stmt.QueryRow(args...)
	defer stmt.Close()
	f, err = parseFileFromRow(row)
	return
}

func parseFileFromRow(row contracts.RowScanner) (f contracts.File, err error) {
	var DownloadTime interface{}
	var PrevRemoteModificationTime interface{}
	var CurRemoteModificationTime interface{}
	err = row.Scan(
		&f.Id,
		&f.PrevRemoteName,
		&f.CurRemoteName,
		&f.Hash,
		&DownloadTime,
		&PrevRemoteModificationTime,
		&CurRemoteModificationTime,
		&f.MimeType,
		&f.Shared,
		&f.RootFolder,
		&f.SizeBytes,
		&f.Trashed,
		&f.RemovedRemotely,
	)

	if err == nil {
		f.DownloadTime = parseTime(DownloadTime)
		f.PrevRemoteModTime = parseTime(PrevRemoteModificationTime)
		f.CurRemoteModTime = parseTime(CurRemoteModificationTime)
	} else {
		err = errors.Wrap(err, "could not scan file data from db")
	}

	return
}

func parseTime(t interface{}) time.Time {
	if t != nil {
		return t.(time.Time)
	} else {
		return time.Time{}
	}
}
