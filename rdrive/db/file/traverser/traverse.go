package traverser

import (
	"database/sql"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/contracts"
	"github.com/svetlyi/gdriveapp/rdrive/db/file"
	"os"
)

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
	root.PrevPath = root.PrevRemoteName
	root.CurPath = root.CurRemoteName
	filesChan <- root

	if err = tr.getFilesByParent(root.Id, filesChan); err != nil {
		tr.log.Error("Error getting files by parent", err)
	}
	close(filesChan)
}

func (tr *Traverser) getFilesByParent(parentId string, filesChan contracts.FilesChan) error {
	filesList, err := tr.fr.GetCurFilesListByParent(parentId)
	if err != nil {
		return errors.Wrapf(err, "could not get files list for %s", parentId)
	}
	for _, f := range filesList {
		//f.Path = strings.Join(currentParentNodes, "/")
		filesChan <- f
		if err := tr.getFilesByParent(f.Id, filesChan); err != nil {
			return errors.Wrap(err, "could not get files by parent")
		}
	}

	return nil
}
