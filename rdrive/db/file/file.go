package file

import (
	"database/sql"
	"fmt"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/contracts"
	"google.golang.org/api/drive/v3"
	"os"
	"path/filepath"
	"strings"
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
    files.removed_remotely,
    files.removed_locally
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
	_, err := fr.db.Exec(
		query,
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
	if nil == err {
		err = fr.linkWithParents(file)
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
	_, err := fr.db.Exec(
		query,
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
	if nil != err {
		err = errors.Wrap(err, "error while inserting root folder")
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
	query := `INSERT INTO 
				files_parents('file_id', 'prev_parent_id', 'cur_parent_id') 
				VALUES (?, ?, ?)`
	_, err := fr.db.Exec(query, file.Id, file.Parents[0], file.Parents[0])

	return err
}

// getRootFolder gets the root folder. As in google drive as in Linux
// everything is a file, we return a file.
func (fr *Repository) GetRootFolder() (contracts.File, error) {
	row := fr.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM files WHERE files.root_folder = 1 LIMIT 1`, fileSelectFields),
	)
	return parseFileFromRow(row)
}

// SetCurRemoteData updates cur_remote_modification_time and other data so that
// after we could check if it was changed remotely. mtime is in RFC3339Nano format
func (fr *Repository) SetCurRemoteData(fileId string, mtime string, name string, parents []string) error {
	if len(parents) > 1 {
		return errors.New("there is no support for multiple parents yet")
	} else if len(parents) == 0 {
		return nil
	}
	mtimeParsed, err := time.Parse(time.RFC3339Nano, mtime)
	if nil != err {
		return errors.Wrapf(err, "wrong ModifiedTime %s", mtime)
	}

	if err := fr.setFileCurRemoteData(fileId, mtimeParsed, name); err != nil {
		return err
	}
	if err := fr.setCurRemoteFileParent(fileId, parents[0]); err != nil {
		return err
	}

	return nil
}

func (fr *Repository) SetPrevRemoteDataToCur(fileId string) error {
	var err error
	err = fr.setPrevRemoteModTimeToCur(fileId)
	if err != nil {
		err = fr.setPrevRemoteParentToCur(fileId)
	}
	return err
}

func (fr *Repository) setPrevRemoteParentToCur(fileId string) error {
	query := `UPDATE files_parents SET prev_parent_id = cur_parent_id WHERE files_parents.file_id = ?`
	_, err := fr.db.Exec(query, fileId)
	if err != nil {
		err = errors.Wrapf(err, "could not update file's %s previous mod time", fileId)
	}
	return err
}

func (fr *Repository) setPrevRemoteModTimeToCur(fileId string) (err error) {
	query := `UPDATE files SET
		prev_remote_modification_time = cur_remote_modification_time,
		prev_remote_name = cur_remote_name
		WHERE files.id = ?
	`

	if _, err = fr.db.Exec(query, fileId); nil != err {
		err = errors.Wrapf(err, "could not update file's %s previous mod time", fileId)
	}

	return
}

func (fr *Repository) setFileCurRemoteData(fileId string, mtime time.Time, name string) (err error) {
	query := `UPDATE files SET 'cur_remote_modification_time' = ?, 'cur_remote_name' = ? WHERE id = ?`

	if _, err = fr.db.Exec(query, mtime.Format(time.RFC3339Nano), name, fileId); err != nil {
		err = errors.Wrapf(err, "could not update file's %s data", fileId)
	}
	return
}
func (fr *Repository) setCurRemoteFileParent(fileId string, parentId string) (err error) {
	query := `UPDATE files_parents SET 'cur_parent_id' = ? WHERE file_id = ?`

	if _, err := fr.db.Exec(query, parentId, fileId); err != nil {
		err = errors.Wrapf(err, "could not update file's %s parent data", fileId)
	}

	return
}

func (fr *Repository) SetRemovedRemotely(fileId string) (err error) {
	query := `UPDATE files SET 'removed_remotely' = 1 WHERE id = ?`

	if _, err = fr.db.Exec(query, fileId); err != nil {
		err = errors.Wrapf(err, "could not set removed_remotely for id %s", fileId)
	}

	return
}

func (fr *Repository) SetRemovedLocally(fileId string, removed bool) (err error) {
	query := `UPDATE files SET 'removed_locally' = ? WHERE id = ?`

	var removedArg int8
	if removed {
		removedArg = 1
	}
	if _, err = fr.db.Exec(query, removedArg, fileId); err != nil {
		err = errors.Wrapf(err, "could not set removed_locally for id %s", fileId)
	}

	return
}

func (fr *Repository) Delete(fileId string) (err error) {
	query := `DELETE FROM files WHERE id = ?`

	if _, err = fr.db.Exec(query, fileId); err != nil {
		err = errors.Wrapf(err, "could not delete file %s from database", fileId)
	} else {
		err = fr.deleteFromParents(fileId)
	}

	return
}

func (fr *Repository) deleteFromParents(fileId string) (err error) {
	query := `DELETE FROM files_parents WHERE file_id = ?`

	if _, err = fr.db.Exec(query, fileId); err != nil {
		err = errors.Wrapf(err, "could not delete file %s from parents in database", fileId)
	}

	return
}

func (fr *Repository) SetPrevRemoteModificationDate(fileId string, date time.Time) (err error) {
	query := `UPDATE files SET 'prev_remote_modification_time' = ? WHERE id = ?`

	if _, err := fr.db.Exec(query, date.Format(time.RFC3339Nano), fileId); err != nil {
		err = errors.Wrapf(err, "could not set prev_remote_modification_time for filId %s", fileId)
	}

	return
}

// SetDownloadTime updates download_time so that
// after we knew if the file was downloaded and if it was changed. download_time equals
// the last local modification time
func (fr *Repository) SetDownloadTime(fileId string, date time.Time) (err error) {
	query := `UPDATE files SET 'download_time' = ? WHERE id = ?`

	if _, err := fr.db.Exec(query, date.Format(time.RFC3339Nano), fileId); err != nil {
		err = errors.Wrapf(err, "could not set download_time for file id %s", fileId)
	}

	return err
}

// GetFileById gets a file by its id.
func (fr *Repository) GetFileById(id string) (contracts.File, error) {
	row := fr.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM files WHERE files.id = ? LIMIT 1`, fileSelectFields),
		id,
	)
	return parseFileFromRow(row)
}

// GetFileByHash gets a file by its hash.
func (fr *Repository) GetFileByHash(hash string) (contracts.File, error) {
	row := fr.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM files WHERE files.hash = ? LIMIT 1`, fileSelectFields),
		hash,
	)
	return parseFileFromRow(row)
}

// GetFileParentFolderPath gets the path to the parent folder of the file with the provided id
func (fr *Repository) GetFileParentFolderPath(id string) (curPath string, prevPath string, err error) {
	query := `
		WITH get_prev_parents (ordi, parent_id, name) AS (
			SELECT 0, fp.prev_parent_id, f_parent.prev_remote_name
			FROM files f
					 JOIN files_parents fp ON f.id = fp.file_id
					 JOIN files f_parent ON f_parent.id = fp.prev_parent_id
			WHERE f.id = ?
			UNION ALL
			SELECT ordi + 1, fp.prev_parent_id, f_parent.prev_remote_name
			FROM get_prev_parents gp
					 JOIN files f ON gp.parent_id = f.id
					 JOIN files_parents fp ON f.id = fp.file_id
					 JOIN files f_parent ON f_parent.id = fp.prev_parent_id
		),
			 get_cur_parents (ordi, parent_id, name) AS (
				 SELECT 0, fp.cur_parent_id, f_parent.cur_remote_name
				 FROM files f
						  JOIN files_parents fp ON f.id = fp.file_id
						  JOIN files f_parent ON f_parent.id = fp.cur_parent_id
				 WHERE f.id = ?
				 UNION ALL
				 SELECT ordi + 1, fp.cur_parent_id, f_parent.cur_remote_name
				 FROM get_cur_parents gp
						  JOIN files f ON gp.parent_id = f.id
						  JOIN files_parents fp ON f.id = fp.file_id
						  JOIN files f_parent ON f_parent.id = fp.cur_parent_id
			 )
		select *
		from (
			  (select group_concat(gpp.name, '/') as prevPath
			   from (SELECT name
					 FROM get_prev_parents
					 order by ordi desc) gpp)
				 JOIN
			 (select group_concat(gcp.name, '/') as curPath
			  from (SELECT name
					FROM get_cur_parents
					order by ordi desc) gcp))
		WHERE prevPath IS NOT NULL AND curPath IS NOT NULL
	`

	err = fr.db.QueryRow(query, id, id).Scan(&prevPath, &curPath)

	if nil != err && sql.ErrNoRows != err {
		err = errors.Wrap(err, "could not get GetFileParentFolderPath")
	}
	return
}

func (fr *Repository) GetLocallyRemovedFoldersIds() ([]string, error) {
	var ids []string

	fr.log.Debug("getting deleted folders")
	rows, err := fr.db.Query(`
			SELECT files.id
			FROM files 
			WHERE files.removed_locally = 1 AND files.mime_type = ?
			ORDER BY files.cur_remote_name
		`,
		"application/vnd.google-apps.folder",
	)

	if nil != err {
		return ids, errors.Wrap(err, "error querying deleted folders.")
	}
	defer rows.Close()
	var curId string

	for rows.Next() {
		if err := rows.Scan(&curId); nil == err {
			ids = append(ids, curId)
		} else if sql.ErrNoRows == err {
			break
		} else {
			return ids, errors.Wrap(err, "could not scan folder id")
		}
	}

	return ids, nil
}

func (fr *Repository) GetCurFilesListByParent(parentId string) ([]contracts.File, error) {
	var filesList []contracts.File

	fr.log.Debug("getting files by parent", struct {
		parentId string
	}{
		parentId: parentId,
	})
	rows, err := fr.db.Query(
		fmt.Sprintf(`
			SELECT %s
			FROM files 
			JOIN files_parents fp ON files.id = fp.file_id 
			WHERE fp.cur_parent_id=?
			ORDER BY files.cur_remote_name
		`,
			fileSelectFields,
		),
		parentId,
	)

	if nil != err {
		return filesList, errors.Wrap(err, "Error querying files by parent id.")
	}
	defer rows.Close()

	for rows.Next() {
		var f contracts.File

		if f, err = parseFileFromRow(rows); err == nil {
			f.CurPath, f.PrevPath, err = fr.GetFileParentFolderPath(f.Id)
			if sql.ErrNoRows == err {
				// seems like the parent or the file itself was removed
				continue
			}
			if err != nil {
				return filesList, errors.Wrapf(err, "Could not get full path for file %s", f.Id)
			}
			f.CurPath = filepath.Join(f.CurPath, f.CurRemoteName)
			f.PrevPath = filepath.Join(f.PrevPath, f.PrevRemoteName)
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
	row := fr.db.QueryRow(
		`WITH parents AS (
					SELECT f.id
					FROM files f
							 JOIN files_parents fp ON fp.cur_parent_id = f.id
					WHERE file_id = ?
					UNION ALL
					SELECT fp.cur_parent_id
					FROM parents fp_cte
					join files_parents fp where fp.file_id = fp_cte.id
				)
				select 1 from parents p join files f on f.id = p.id where f.trashed = 1 limit 1;`,
		id,
	)

	var hasBeenChanged bool

	if err := row.Scan(&hasBeenChanged); sql.ErrNoRows == err {
		return false, nil
	} else if err == nil {
		return true, nil
	} else {
		return false, errors.Wrap(err, "could not check if has trashed parent")
	}
}

// CleanUpDatabase cleans database from trashed files
func (fr *Repository) CleanUpDatabase() (err error) {
	query := `
	DELETE
	FROM files
	WHERE id IN (SELECT f.id
				 FROM files f
						  JOIN files_parents fp on f.id = fp.file_id
						  LEFT JOIN files fp_file on fp_file.id = fp.cur_parent_id
				 WHERE fp_file.id IS NULL
					OR fp_file.removed_remotely = 1
					OR fp_file.trashed = 1
					OR f.removed_remotely = 1
					OR f.removed_locally = 1
					OR f.trashed = 1)
	`
	if _, err = fr.db.Exec(query); nil != err {
		err = errors.Wrap(err, "could not remove files with removed parents")
	}
	query = `
	DELETE
	FROM files_parents
	WHERE file_id IN (
		SELECT fp.file_id
		FROM files_parents fp
				 LEFT JOIN files f ON fp.file_id = f.id
		WHERE f.id IS NULL)
	`
	if _, err = fr.db.Exec(query); nil != err {
		err = errors.Wrap(err, "could not remove files with removed parents")
	}
	return
}

func (fr *Repository) GetParentIdByChildId(childId string) (string, error) {
	query := `
		SELECT fp.cur_parent_id
		FROM files f
		JOIN files_parents fp ON f.id = fp.file_id
		WHERE
		f.id = ?
	`
	row := fr.db.QueryRow(query, childId)

	var fileId string
	if err := row.Scan(&fileId); nil == err {
		return fileId, nil
	} else if sql.ErrNoRows == err {
		return "", errors.Wrapf(err, "could not find cur_parent_id for file id %s", childId)
	} else {
		return "", errors.Wrap(err, "could not scan parent file id")
	}
}

// GetFileParentIdByCurPath gets file's parent id by its path
func (fr *Repository) GetFileParentIdByCurPath(fullPath string, startWithFolder contracts.File) (string, error) {
	pathSlice := strings.Split(fullPath, string(os.PathSeparator))
	pathSlice = pathSlice[:len(pathSlice)-1]

	return fr.GetFileIdByCurPath(strings.Join(pathSlice, string(os.PathSeparator)), startWithFolder)
}

// GetFileIdByCurPath gets file's id by its path.
func (fr *Repository) GetFileIdByCurPath(fullPath string, startWithFolder contracts.File) (string, error) {
	pathSlice := strings.Split(fullPath, string(os.PathSeparator))
	upperParentName := pathSlice[0]
	if len(pathSlice) == 1 {
		if startWithFolder.CurRemoteName == upperParentName {
			return startWithFolder.Id, nil
		} else {
			return "", sql.ErrNoRows
		}
	} else {
		if fileId, err := fr.GetFileIdByPathSlice(pathSlice[1:], startWithFolder.Id); sql.ErrNoRows == errors.Cause(err) {
			return "", errors.Wrapf(err, "could not find file id with name %s in %s", upperParentName, fullPath)
		} else {
			return fileId, err
		}
	}
}

// GetFileIdByCurPath gets file's id by its path and name.
func (fr *Repository) GetFileIdByPathSlice(lookForPath []string, lookInParentId string) (string, error) {
	lookForName := lookForPath[0]
	query := `
		SELECT f.id
		FROM files f
		LEFT JOIN files_parents fp ON f.id = fp.file_id
		WHERE
		  f.cur_remote_name = ?
		  AND fp.cur_parent_id = ?
	`
	row := fr.db.QueryRow(query, lookForName, lookInParentId)

	var fileId string
	if err := row.Scan(&fileId); nil == err {
		if len(lookForPath) == 1 {
			return fileId, nil
		} else {
			return fr.GetFileIdByPathSlice(lookForPath[1:], fileId)
		}
	} else if sql.ErrNoRows == err {
		return "", errors.Wrapf(err, "could not find file id by path slice with name %s in %s", lookForName, lookInParentId)
	} else {
		return "", errors.Wrap(err, "could not scan file id")
	}
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
		&f.RemovedLocally,
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
