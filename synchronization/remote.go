package synchronization

import (
	"database/sql"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/contracts"
	lfileHash "github.com/svetlyi/gdriveapp/ldrive/file/hash"
	"github.com/svetlyi/gdriveapp/rdrive"
	"github.com/svetlyi/gdriveapp/rdrive/db/file"
	"github.com/svetlyi/gdriveapp/rdrive/specification"
	"os"
	"path/filepath"
)

type Synchronizer struct {
	fr  file.Repository
	log contracts.Logger
	db  *sql.DB
	rd  rdrive.Drive
}

func New(fr file.Repository, log contracts.Logger, db *sql.DB, rd rdrive.Drive) Synchronizer {
	return Synchronizer{fr, log, db, rd}
}

// SyncRemoteWithLocal synchronize remote metadata saved in a local database
// to the actual files saved locally
func (s *Synchronizer) SyncRemoteWithLocal() error {
	var filesChan = make(contracts.FilesChan)

	// fileSyncDoneChan is a channel for synchronization. Sqlite is used as a metadata storage
	// and it does not work well with multiple threads
	var fileSyncDoneChan = make(contracts.SyncChan)
	go s.traverseFiles(filesChan, fileSyncDoneChan)

	var syncRemoteWithLocalErr error
	for f := range filesChan {
		s.log.Debug("traversing over remote files", struct {
			path string
			mime string
		}{
			path: f.CurPath,
			mime: f.MimeType,
		})
		syncRemoteWithLocalErr = s.rd.SyncRemoteWithLocal(f)
		fileSyncDoneChan <- true
		if syncRemoteWithLocalErr != nil {
			return errors.Wrap(syncRemoteWithLocalErr, "synchronization remote with local error")
		}
	}
	return nil
}

// traverseFiles goes through files in hierarchical order. So, first goes the
// root directory (My Drive), then all the children of the root, then the children of
// the children and so on.
func (s *Synchronizer) traverseFiles(filesChan contracts.FilesChan, sync contracts.SyncChan) {
	root, err := s.fr.GetRootFolder()
	if nil != err {
		s.log.Error("Error getting root folder.", err)
		close(filesChan)
		return
	}
	root.PrevPath = root.PrevRemoteName
	root.CurPath = root.CurRemoteName
	filesChan <- root
	<-sync

	if err = s.getFilesByParent(root.Id, filesChan, sync); err != nil {
		s.log.Error("Error getting files by parent", err)
		close(filesChan)
		return
	}
	close(filesChan)
}

func (s *Synchronizer) getFilesByParent(parentId string, filesChan contracts.FilesChan, sync contracts.SyncChan) error {
	filesList, err := s.fr.GetCurFilesListByParent(parentId)
	if err != nil {
		return errors.Wrapf(err, "could not get files list for %s", parentId)
	}
	for _, f := range filesList {
		filesChan <- f
		<-sync
		if err := s.getFilesByParent(f.Id, filesChan, sync); err != nil {
			return errors.Wrap(err, "could not get files by parent")
		}
	}

	return nil
}

// AreFoldersTheSame compares two folders if they have the same structure, files and
// hashes of the files
func (s *Synchronizer) AreFoldersTheSame(fullFolderPath string, remoteFolderId string) (bool, error) {
	var dbFilesChan = make(contracts.FilesChan)
	var localFilesChan = make(chan contracts.ExtendedFileInfo)
	var syncChan = make(contracts.SyncChan)
	var exitChan = make(contracts.ExitChan)

	go func() {
		if err := s.getFilesByParent(remoteFolderId, dbFilesChan, syncChan); nil != err {
			s.log.Error(err)
		}
		close(dbFilesChan)
	}()
	go func() {
		err := filepath.Walk(
			fullFolderPath,
			func(path string, info os.FileInfo, err error) error {
				if path != fullFolderPath {
					localFilesChan <- contracts.ExtendedFileInfo{FileInfo: info, FullPath: path}
				}
				return nil
			},
		)
		close(localFilesChan)
		if nil != err {
			s.log.Error(err)
		}
	}()
	isDirTheSame := true
	var err error
	go func() {
		for {
			dbFile, dbChanOpened := <-dbFilesChan
			if dbChanOpened {
				syncChan <- true
			} else {
				close(syncChan)
			}
			localFile, localChanOpened := <-localFilesChan
			if !dbChanOpened || !localChanOpened {
				break
			}
			if dbFile.CurRemoteName != localFile.FileInfo.Name() {
				isDirTheSame = false
				break
			}
			if localFile.FileInfo.IsDir() != specification.IsFolder(dbFile) {
				isDirTheSame = false
				break
			}
			if localFile.FileInfo.IsDir() || specification.IsFolder(dbFile) {
				continue
			}
			var hash string
			hash, err = lfileHash.CalcCachedHash(localFile.FullPath)
			if nil != err {
				isDirTheSame = false
				err = errors.Wrap(err, "hash calculation error while comparing folders")
				break
			}
			if hash != dbFile.Hash {
				isDirTheSame = false
				break
			}
		}
		close(exitChan)
	}()
	<-exitChan

	return isDirTheSame, err
}
