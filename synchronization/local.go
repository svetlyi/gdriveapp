package synchronization

import (
	"database/sql"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/contracts"
	"github.com/svetlyi/gdriveapp/structures"
	"os"
	"path/filepath"
	"strings"
)

// SyncLocalWithRemote synchronize local files and their changes
// with remote version. It uploads new files, creates new folders remotely
func (s *Synchronizer) SyncLocalWithRemote(drivePath string, rootFolder contracts.File) error {
	locallyRemovedFoldersIds, err := s.fr.GetLocallyRemovedFoldersIds()
	if nil != err {
		s.log.Error(err)
		os.Exit(1)
	}
	var parentsStack structures.StringStack
	parentsStack.Push(rootFolder.Id)
	var curDepth int
	var parentId string

	return filepath.Walk(
		filepath.Join(drivePath, rootFolder.CurRemoteName),
		func(path string, info os.FileInfo, err error) error {
			if nil != err {
				return errors.Wrapf(err, "cold not walk in path %s", path)
			}
			s.log.Debug("next local path", path)
			curRelativeFilePath := path[len(drivePath):]
			fileId, fileIdErr := s.fr.GetFileIdByCurPath(curRelativeFilePath, rootFolder)

			curDepth = len(strings.Split(curRelativeFilePath, string(os.PathSeparator)))

			if !info.IsDir() {
				curDepth-- // minus file itself if the current element is a file
				if parentsStack.Len() > curDepth {
					if err := parentsStack.PopTimes(parentsStack.Len() - curDepth); nil != err {
						return err
					}
				}
			}
			if parentId, err = parentsStack.Front(); nil != err {
				return err
			}
			s.log.Debug("depth info", struct {
				currentDepth        int
				currentRelativePath string
				parentsStackLength  int
				parentId            string
				path                string
			}{
				curDepth,
				curRelativeFilePath,
				parentsStack.Len(),
				parentId,
				path,
			})
			// it means the file or directory is new (created, moved or copied)
			// here just new files are being synchronized. The rest have have already been synchronized previously
			if sql.ErrNoRows == errors.Cause(fileIdErr) {
				if info.IsDir() {
					currentParentId, err := s.fr.GetFileParentIdByCurPath(curRelativeFilePath, rootFolder)
					if nil != err {
						return errors.Wrapf(err, "could not GetFileParentIdByCurPath for %s", curRelativeFilePath)
					}
					// if it is a dir, first guess, it was moved from somewhere else
					// so, we are looking for the moved dir among the locally removed
					var hasSameRemFolder = false
					var locallyRemovedFolderId string
					for _, locallyRemovedFolderId = range locallyRemovedFoldersIds {
						if hasSameRemFolder, err = s.AreFoldersTheSame(path, locallyRemovedFolderId); nil != err {
							return errors.Wrapf(
								err,
								"error while checking if the folders %s(path) and %s(id) are the same",
								path,
								locallyRemovedFolderId,
							)
						}
					}
					if hasSameRemFolder {
						oldParentId, err := s.fr.GetParentIdByChildId(locallyRemovedFolderId)
						if nil != err {
							return errors.Wrapf(err, "could not GetParentIdByChildId for file id %s", locallyRemovedFolderId)
						}
						s.log.Debug("local move detected", struct {
							movedFolderId   string
							currentParentId string
							currentName     string
						}{locallyRemovedFolderId, currentParentId, info.Name()})
						// at this point it is known, that the folder with id locallyRemovedFolderId was
						// moved from a folder with id oldParentId to a folder with id currentParentId and now
						// the moved folder has name info.Name(). This is the information, that goes to the database
						f, err := s.rd.Update(locallyRemovedFolderId, info.Name(), []string{currentParentId}, []string{oldParentId})
						if nil != err {
							return err
						}
						fileId = f.Id
						err = s.fr.SetRemovedLocally(locallyRemovedFolderId, false)
						if nil != err {
							return err
						}
						err = s.fr.SetCurRemoteData(locallyRemovedFolderId, f.ModifiedTime, f.Name, f.Parents)
						if nil != err {
							return err
						}
						err = s.fr.SetPrevRemoteDataToCur(locallyRemovedFolderId)
						if nil != err {
							return err
						}
					} else {
						s.log.Info("creating folder", struct {
							path     string
							parentId string
						}{path, parentId})
						if fileId, err = s.rd.CreateFolder(path, []string{parentId}); nil != err {
							return errors.Wrapf(err, "could not create folder %s", path)
						}
					}
				} else {
					s.log.Info("creating file", path, "in", parentId)
					if err = s.rd.Upload(path, []string{parentId}); nil != err {
						return errors.Wrapf(err, "could not upload file %s", path)
					}
				}
			} else if nil != fileIdErr {
				return errors.Wrapf(fileIdErr, "could not GetFileIdByCurPath for %s", curRelativeFilePath)
			}

			if info.IsDir() {
				if parentsStack.Len() < curDepth {
					parentsStack.Push(fileId)
				} else if parentsStack.Len() > curDepth {
					if err := parentsStack.PopTimes(parentsStack.Len() - curDepth); nil != err {
						return err
					}
				} else if parentId != fileId {
					if err := parentsStack.Pop(); nil != err {
						return err
					}
					parentsStack.Push(fileId)
				}
			}

			return nil
		},
	)
}
