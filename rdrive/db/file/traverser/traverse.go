package traverser

import (
	"database/sql"
	"fmt"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/contracts"
	"github.com/svetlyi/gdriveapp/rdrive/db/file"
	"os"
	"strings"
	"time"
)

var fieldsToQuery = `
	files.id,
	files.name,
	files.hash,
	files.download_time,
	files.mime_type,
	files.shared,
	files.root_folder,
	files.trashed
`

type Traverser struct {
	fr  file.Repository
	log contracts.Logger
	db  *sql.DB
}

func New(fr file.Repository, log contracts.Logger, db *sql.DB) Traverser {
	return Traverser{fr: fr, log: log, db: db}
}

// TraverseFiles goes through files in hierarchical order. So, first goes the
// root directory (My Drive), then all the children of the root, then the children of
// the children and so on.
func (tr *Traverser) TraverseFiles(filesChan contracts.FilesChan) {
	root, err := tr.fr.GetRootFolder()
	if nil != err {
		tr.log.Error("Error getting root folder.", err)
		os.Exit(1)
	}
	root.Path = root.Name
	filesChan <- root
	var parentNodes = []string{root.Name}
	if err = tr.getFilesByParent(root.Id, filesChan, parentNodes); err != nil {
		tr.log.Error("Error getting files by parent", err)
	}
	close(filesChan)
}

func (tr *Traverser) getFilesByParent(parentId string, filesChan contracts.FilesChan, parentNodes []string) error {
	filesList, err := tr.getFilesList(parentId)
	if err != nil {
		return errors.Wrapf(err, "could not get files list for %s", parentId)
	}
	for _, f := range filesList {
		var currentParentNodes = append([]string(nil), parentNodes...)
		currentParentNodes = append(currentParentNodes, f.Name)

		f.Path = strings.Join(currentParentNodes, "/")
		filesChan <- f
		if err := tr.getFilesByParent(f.Id, filesChan, currentParentNodes); err != nil {
			return errors.Wrap(err, "could not get files by parent")
		}
	}

	return nil
}

func (tr *Traverser) getFilesList(parentId string) ([]contracts.File, error) {
	var filesList []contracts.File

	tr.log.Debug("getting files by parent", struct {
		parentId string
	}{
		parentId: parentId,
	})
	selectFilesStmt, err := tr.db.Prepare(
		fmt.Sprintf(`
			SELECT 
			%s
			FROM files 
			JOIN files_parents fp ON files.id = fp.file_id 
			WHERE fp.parent_id=?`,
			fieldsToQuery,
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

		var DownloadTime interface{}
		err = rows.Scan(
			&f.Id,
			&f.Name,
			&f.Hash,
			&DownloadTime,
			&f.MimeType,
			&f.Shared,
			&f.RootFolder,
			&f.Trashed,
		)
		if nil != err {
			return filesList, errors.Wrap(err, "Error looping over files.")
		}
		if DownloadTime != nil {
			f.DownloadTime = DownloadTime.(time.Time)
		} else {
			f.DownloadTime = time.Time{}
		}

		filesList = append(filesList, f)
	}
	if err = rows.Err(); err != nil {
		return filesList, errors.Wrap(err, "Error fetching files by parent id.")
	}
	return filesList, nil
}
