package synchronization

import (
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
	rd  rdrive.Drive
}

func New(fr file.Repository, log contracts.Logger, rd rdrive.Drive) Synchronizer {
	return Synchronizer{fr, log, rd}
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
