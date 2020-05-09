package rdrive

import (
	"database/sql"
	"fmt"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/app"
	"github.com/svetlyi/gdriveapp/contracts"
	lfile "github.com/svetlyi/gdriveapp/ldrive/file"
	lfileHash "github.com/svetlyi/gdriveapp/ldrive/file/hash"
	"github.com/svetlyi/gdriveapp/rdrive/specification"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"io"
	"os"
	"time"
)

var fileFieldsSet = "id, name, mimeType, parents, shared, md5Checksum, size, modifiedTime, trashed, explicitlyTrashed"

// getFilesList puts files from the remote drive into filesChan channel
// one by one
func (d *Drive) getFilesList(filesChan chan *drive.File) {
	var nextPageToken = ""
	var filesListCall *drive.FilesListCall

	for {
		filesListCall = d.filesService.List()
		if "" != nextPageToken {
			filesListCall.PageToken(nextPageToken)
		}
		fileList, err := filesListCall.PageSize(d.pageSizeToQuery).Fields(
			googleapi.Field(fmt.Sprintf("nextPageToken, files(%s)", fileFieldsSet)),
		).Do()

		if err != nil {
			d.log.Error("Unable to retrieve files: %v", err)
			close(filesChan)
		}
		nextPageToken = fileList.NextPageToken

		d.log.Info("Getting files list...")

		for _, rfile := range fileList.Files {
			d.log.Debug("Found file", rfile)
			filesChan <- rfile
		}
		if "" == nextPageToken {
			break
		}
	}
	close(filesChan)
}

// getChangedFilesList gets the changed files from the remote drive into filesChan channel
// one by one
func (d *Drive) getChangedFilesList(filesChan chan *drive.Change, exitChan contracts.ExitChan) {
	nextPageToken, err := d.appState.Get(app.NextChangeToken)
	if err != nil && errors.Cause(err) != sql.ErrNoRows {
		d.log.Error("error getting NextChangeToken from app state", err)
		close(exitChan)
	}

	var changesListCall *drive.ChangesListCall

	startPageToken, err := d.changesService.GetStartPageToken().Do()
	if err != nil {
		d.log.Error("error getting start page token in changed files list", err)
		close(exitChan)
	}

	for {
		d.log.Debug("change token", struct {
			nextPageToken  string
			startPageToken string
		}{nextPageToken, startPageToken.StartPageToken})

		if nextPageToken == "" {
			nextPageToken = startPageToken.StartPageToken
			d.log.Info("next page token", nextPageToken)
		}
		changesListCall = d.changesService.List(nextPageToken)
		changeList, err := changesListCall.PageSize(d.pageSizeToQuery).Fields(
			googleapi.Field(fmt.Sprintf("nextPageToken, changes(removed, fileId, file(%s))", fileFieldsSet)),
		).Do()

		if err != nil {
			d.log.Error("Unable to retrieve changed files: %v", err)
			close(exitChan)
			break
		}
		nextPageToken = changeList.NextPageToken

		d.log.Info("getting changed files list. amount of changes: ", len(changeList.Changes))

		for _, change := range changeList.Changes {
			// if the file was removed, there is no any information except its identifier
			if change.Removed {
				d.log.Debug(fmt.Sprintf("changeList:file %s was removed", change.FileId))
			} else if change.File.Trashed {
				d.log.Debug(fmt.Sprintf("changeList:file %s was trashed", change.FileId))
			} else if change.File.ExplicitlyTrashed {
				d.log.Debug(fmt.Sprintf("changeList:file %s was explicitly trashed", change.FileId))
			} else {
				d.log.Debug("changeList:found change", struct {
					Id           string
					Name         string
					ModifiedTime string
				}{
					Id:           change.FileId,
					Name:         change.File.Name,
					ModifiedTime: change.File.ModifiedTime,
				})
			}
			filesChan <- change
		}

		if "" == nextPageToken {
			d.log.Debug("no more changes")
			break
		}
	}
	if err := d.appState.Set(app.NextChangeToken, startPageToken.StartPageToken); err != nil {
		d.log.Error("error saving NextChangeToken to app state", err)
		close(exitChan)
	}
	d.log.Debug("changes:closing files channel")

	close(filesChan)
}

func (d *Drive) getRootFolder() (*drive.File, error) {
	rootFolder, err := d.filesService.Get("root").Fields(googleapi.Field(fileFieldsSet)).Do()
	if err != nil {
		return &drive.File{}, errors.Wrap(err, "Could not fetch root folder info")
	}
	d.log.Debug("Found root folder", struct {
		Id   string
		Name string
	}{
		rootFolder.Id,
		rootFolder.Name,
	})

	return rootFolder, nil
}

// SyncRemoteWithLocal synchronizes the local file system with remote one
// file - file information from remote
func (d *Drive) SyncRemoteWithLocal(file contracts.File) error {
	localChangeType, err := d.isChangedLocally(file)
	if err != nil {
		return errors.Wrap(err, "could not determine if it was locally changed")
	}
	remoteChangeType, err := d.isChangedRemotely(file)
	if err != nil {
		return errors.Wrap(err, "could not determine if it was remotely changed")
	}

	if (localChangeType != contracts.FILE_NOT_CHANGED || remoteChangeType != contracts.FILE_NOT_CHANGED) &&
		(specification.CanDownloadFile(file) || specification.IsFolder(file)) {
		d.log.Debug("SyncRemoteWithLocal. change types", struct {
			file             contracts.File
			localChangeType  contracts.FileChangeType
			remoteChangeType contracts.FileChangeType
		}{file, localChangeType, remoteChangeType})
	}

	curFullFilePath := lfile.GetCurFullPath(file)
	if specification.IsFolder(file) {
		switch { // the only the things that can happen to a folder are: move, Delete
		case contracts.FILE_MOVED == remoteChangeType:
			return d.handleMovedRemotely(file)
		case contracts.FILE_DELETED == remoteChangeType:
			return d.handleRemovedRemotely(file)
		case contracts.FILE_UPDATED == remoteChangeType ||
			(contracts.FILE_NOT_CHANGED == remoteChangeType && contracts.FILE_NOT_EXIST == localChangeType):
			d.log.Info("creating folder", struct {
				name string
			}{name: file.CurRemoteName})
			if err := os.Mkdir(curFullFilePath, 0744); !os.IsExist(err) {
				return errors.Wrap(err, "could not create dir")
			}
			if err := d.fileRepository.SetPrevRemoteDataToCur(file.Id); nil != err {
				return errors.Wrap(err, "could not set previous remote data to current")
			}
		}
	}
	if !specification.CanDownloadFile(file) {
		return nil
	}

	switch {
	case contracts.FILE_NOT_CHANGED == remoteChangeType && contracts.FILE_NOT_EXIST == localChangeType:
		d.log.Info("downloading file. remote file has not changed. local one does not exist", file)
		if err = d.download(file); err != nil {
			err = errors.Wrapf(err, "could not download file %s", file.Id)
		}
	case contracts.FILE_NOT_CHANGED == remoteChangeType && contracts.FILE_NOT_CHANGED == localChangeType:
		if file.DownloadTime.IsZero() {
			err = d.setDownloadTimeByStatsForFile(file)
		}
		break // do nothing
	case contracts.FILE_NOT_CHANGED == remoteChangeType && contracts.FILE_UPDATED == localChangeType:
		d.log.Info("uploading file. remote file has not changed. local one updated", file)
		if err = d.updateRemote(file); err != nil {
			err = errors.Wrapf(err, "could not updateRemote file %s", file.Id)
		}
	case contracts.FILE_NOT_CHANGED == remoteChangeType && contracts.FILE_DELETED == localChangeType:
		d.log.Info("setting file to be deleted remotely. remote file has not changed. local one deleted", file)
		if err = d.fileRepository.SetRemovedLocally(file.Id, true); err != nil {
			err = errors.Wrapf(err, "could not set removed locally for file %s", file.Id)
		}
	case contracts.FILE_UPDATED == remoteChangeType && contracts.FILE_NOT_CHANGED == localChangeType:
		d.log.Info("downloading file. remote file changed", file)
		if err = d.download(file); err != nil {
			err = errors.Wrapf(err, "could not download file %s", file.Id)
		}
	case contracts.FILE_UPDATED == remoteChangeType && contracts.FILE_UPDATED == localChangeType:
		d.log.Warning("CONFLICT. remote and local files were changed", file)
		break //TODO: conflict
	case contracts.FILE_UPDATED == remoteChangeType && contracts.FILE_DELETED == localChangeType:
		d.log.Warning("CONFLICT. remote file was changed, but local one was deleted", file)
		break //TODO: conflict
	case contracts.FILE_DELETED == remoteChangeType && contracts.FILE_NOT_CHANGED == localChangeType:
		d.log.Info("deleting file locally", file)
		err = os.Remove(curFullFilePath)
	case contracts.FILE_DELETED == remoteChangeType && contracts.FILE_UPDATED == localChangeType:
		d.log.Warning("CONFLICT. remote file was deleted, but local one was updated", file)
		break //TODO: conflict
	case contracts.FILE_DELETED == remoteChangeType && contracts.FILE_DELETED == localChangeType:
		err = d.fileRepository.SetRemovedLocally(file.Id, true)
	case contracts.FILE_MOVED == remoteChangeType && contracts.FILE_NOT_CHANGED == localChangeType:
		err = d.handleMovedRemotely(file)
	case contracts.FILE_MOVED == remoteChangeType && contracts.FILE_UPDATED == localChangeType:
		d.log.Warning("CONFLICT. remote file was moved, but local one was updated", file)
		break //TODO: conflict
	case contracts.FILE_MOVED == remoteChangeType && contracts.FILE_DELETED == localChangeType:
		d.log.Info("downloading file. remote file was moved, local one deleted", file)
		if err = d.download(file); err != nil {
			err = errors.Wrapf(err, "could not download file %s", file.Id)
		}
	}

	return err
}

// handleRemovedRemotely removes a file locally because it was remoted remotely
func (d *Drive) handleRemovedRemotely(file contracts.File) (err error) {
	d.log.Debug("removing file", file)
	curFullFilePath := lfile.GetCurFullPath(file)
	if _, err = os.Stat(curFullFilePath); os.IsNotExist(err) {
		return nil // we are going to remove a file, but it does not exist. just do nothing in this case
	}

	err = os.RemoveAll(curFullFilePath)
	if nil == err {
		err = d.fileRepository.Delete(file.Id)
	}

	return err
}

// handleMovedRemotely moves a file from the old to the new location
func (d *Drive) handleMovedRemotely(file contracts.File) (err error) {
	d.log.Debug("moving file", file)
	curFullFilePath := lfile.GetCurFullPath(file)
	getPrevFullPath := lfile.GetPrevFullPath(file)
	if _, err = os.Stat(getPrevFullPath); os.IsNotExist(err) {
		return nil // we are going to move a file, but it does not exist. just do nothing in this case
	}

	// if the file does not exist at the destination (current path)
	if _, err = os.Stat(curFullFilePath); os.IsNotExist(err) {
		err = os.Rename(getPrevFullPath, curFullFilePath)
	}
	if err != nil {
		err = d.fileRepository.SetPrevRemoteDataToCur(file.Id)
	}
	if err != nil {
		err = d.setDownloadTimeByStatsForFile(file)
	}

	return err
}

// isChangedLocally determines if the file was changed locally (updated or deleted)
func (d *Drive) isChangedLocally(file contracts.File) (contracts.FileChangeType, error) {
	curFullPath := lfile.GetCurFullPath(file)

	if stats, err := os.Stat(curFullPath); os.IsNotExist(err) {
		if file.DownloadTime.IsZero() {
			return contracts.FILE_NOT_EXIST, nil
		} else {
			return contracts.FILE_DELETED, nil
		}
	} else if err == nil { // if the file exist
		if file.DownloadTime.IsZero() { // it has never been downloaded, but it exists
			if sameAsRemote, err := d.isLocalSameAsRemote(file); nil == err {
				if sameAsRemote {
					return contracts.FILE_NOT_CHANGED, nil
				} else {
					return contracts.FILE_UPDATED, nil
				}
			} else {
				return contracts.FILE_ERROR, errors.Wrap(err, "could not determine if the same as remote")
			}
		}
		if file.DownloadTime.Unix() == stats.ModTime().Unix() {
			return contracts.FILE_NOT_CHANGED, nil
		} else {
			return contracts.FILE_UPDATED, nil
		}
	} else {
		return contracts.FILE_ERROR, errors.Wrapf(err, "could not get file '%s' stats", curFullPath)
	}
}

// isChangedRemotely determines if the file was changed remotely (updated, moved or deleted)
func (d *Drive) isChangedRemotely(file contracts.File) (contracts.FileChangeType, error) {
	if hasTrashedParent, err := d.fileRepository.HasTrashedParent(file.Id); err == nil {
		if hasTrashedParent {
			d.log.Debug("file has trashed parent", file)
			return contracts.FILE_DELETED, nil
		}
	} else {
		return contracts.FILE_ERROR, errors.Wrap(err, "error checking trashed parent")
	}
	if file.RemovedRemotely == 1 || file.Trashed == 1 {
		return contracts.FILE_DELETED, nil
	}
	if file.PrevPath != file.CurPath {
		return contracts.FILE_MOVED, nil
	}
	if !file.CurRemoteModTime.Equal(file.PrevRemoteModTime) {
		return contracts.FILE_UPDATED, nil
	}
	return contracts.FILE_NOT_CHANGED, nil
}

func (d *Drive) updateRemote(file contracts.File) error {
	if sameFileExists, err := d.isLocalSameAsRemote(file); err == nil && sameFileExists {
		d.log.Debug(fmt.Sprintf("skipping file %s: already exists", file.Id))
		return d.setDownloadTimeByStatsForFile(file) // most probably it was not downloaded previously
	} else if err != nil {
		return err
	}

	curFullPath := lfile.GetCurFullPath(file)
	if lf, err := os.Open(curFullPath); err == nil {
		rf, err := d.filesService.Update(file.Id, &drive.File{}).Media(lf).Do()
		if err != nil {
			return errors.Wrap(err, "could not update file remotely")
		}
		if t, err := time.Parse(time.RFC3339, rf.ModifiedTime); err != nil {
			return d.fileRepository.SetPrevRemoteModificationDate(file.Id, t)
		} else {
			return errors.Wrapf(err, "could not parse modified time %s", rf.ModifiedTime)
		}
	} else {
		return errors.Wrapf(err, "open file %s error", curFullPath)
	}
}

func (d *Drive) Upload(curFullPath string, parentIds []string) error {
	fileHash, err := lfileHash.CalcCachedHash(curFullPath)
	if nil != err {
		return errors.Wrapf(err, "could not calculate hash for %s", curFullPath)
	}
	sameFile, err := d.fileRepository.GetFileByHash(fileHash)
	if nil != err && sql.ErrNoRows != errors.Cause(err) {
		return errors.Wrapf(err, "error finding a file %s by hash %s", curFullPath, fileHash)
	}
	stat, err := os.Stat(curFullPath)
	if nil != err {
		return errors.Wrapf(err, "could not get stat for file %s", curFullPath)
	}
	var rf *drive.File
	if sql.ErrNoRows == errors.Cause(err) || sameFile.SizeBytes != uint64(stat.Size()) {
		// if there is no such a file, then just upload
		lf, err := os.Open(curFullPath)
		if nil != err {
			return errors.Wrapf(err, "error opening file %s", curFullPath)
		}
		defer lf.Close()

		rf, err = d.filesService.
			Create(&drive.File{Name: stat.Name(), Parents: parentIds}).
			Fields(googleapi.Field(fileFieldsSet)).
			Media(lf).Do()
		if nil != err {
			return errors.Wrapf(err, "could not upload file %s", curFullPath)
		}
	} else {
		d.log.Debug("uploading: found the same file. copying", struct {
			sameFileName     string
			sameFileId       string
			fileNameToUpload string
		}{
			sameFile.CurRemoteName,
			sameFile.Id,
			stat.Name(),
		})
		rf, err = d.filesService.
			Copy(sameFile.Id, &drive.File{Name: stat.Name(), Parents: parentIds}).
			Fields(googleapi.Field(fileFieldsSet)).
			Do()
	}
	err = d.fileRepository.CreateFile(rf)
	if nil != err {
		return errors.Wrapf(err, "could not create file %s in db", curFullPath)
	}

	return d.fileRepository.SetDownloadTime(rf.Id, stat.ModTime())
}

func (d *Drive) CreateFolder(curFullPath string, parentIds []string) (string, error) {
	stat, err := os.Stat(curFullPath)
	if nil != err {
		return "", errors.Wrapf(err, "could not get stat for folder %s", curFullPath)
	}
	var rf *drive.File
	rf, err = d.filesService.
		Create(&drive.File{
			Name:     stat.Name(),
			Parents:  parentIds,
			MimeType: specification.GetFolderMime(),
		}).
		Fields(googleapi.Field(fileFieldsSet)).
		Do()
	if nil != err {
		return "", errors.Wrapf(err, "could not upload file %s", curFullPath)
	}
	err = d.fileRepository.CreateFile(rf)
	if nil != err {
		return "", errors.Wrapf(err, "could not create folder %s in db", curFullPath)
	}
	return rf.Id, nil
}

func (d *Drive) Update(fileId string, name string, parentIds []string, oldParentIds []string) (*drive.File, error) {
	f, err := d.filesService.Update(fileId, &drive.File{
		Name: name,
	}).Fields(googleapi.Field(fileFieldsSet)).AddParents(parentIds[0]).RemoveParents(oldParentIds[0]).Do()
	if nil != err {
		err = errors.Wrapf(err, "could not update file with id", fileId)
	}
	return f, err
}

func (d *Drive) Delete(file contracts.File) error {
	var err error
	if err = d.filesService.Delete(file.Id).Do(); err == nil {
		err = d.fileRepository.Delete(file.Id)
	}
	if err != nil {
		err = errors.Wrap(err, "could not delete file remotely")
	}
	return err
}

func (d *Drive) download(file contracts.File) error {
	fileFullPath := lfile.GetCurFullPath(file)

	if sameFileExists, err := d.isLocalSameAsRemote(file); err == nil && sameFileExists {
		d.log.Debug(fmt.Sprintf("skipping file %s: already exists", file.Id))
		if err = d.setDownloadTimeByStatsForFile(file); err != nil {
			return err
		}
		if err = d.fileRepository.SetPrevRemoteModificationDate(file.Id, file.CurRemoteModTime); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	gfileReader, err := d.filesService.Get(file.Id).Download()
	if err != nil {
		d.log.Error("Unable to retrieve file: %v", err)
		return err
	}
	lf, err := os.Create(fileFullPath)

	if nil == err {
		defer lf.Close()
		buf := make([]byte, 1024)
		for {
			// read a chunk
			n, err := gfileReader.Body.Read(buf)
			if err != nil && err != io.EOF {
				d.log.Error("Unable to download file: %v", err)
				if remErr := os.Remove(fileFullPath); remErr != nil {
					return remErr
				}
				return err
			}
			if n == 0 {
				break
			}

			// write a chunk
			if _, err := lf.Write(buf[:n]); err != nil {
				if remErr := os.Remove(fileFullPath); remErr != nil {
					return errors.Wrap(remErr, "Unable to remove file")
				}
				return errors.Wrap(err, "could not write a chunk")
			}
		}
		return d.setDownloadTimeByStatsForFile(file)
	}

	return err
}

// isLocalSameAsRemote checks that a file with the same path, name and hash exists
// if it exists, we won't download it. We need it when for some reason the database was empty
// or the downloaded time in the database is null
func (d *Drive) isLocalSameAsRemote(file contracts.File) (bool, error) {
	fileFullPath := lfile.GetCurFullPath(file)
	stat, err := os.Stat(fileFullPath)

	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		} else {
			return false, errors.Wrapf(err, "could not check if the file %s exists", fileFullPath)
		}
	}

	if stat.IsDir() {
		return true, nil
	}

	if hash, err := lfileHash.CalcCachedHash(fileFullPath); nil != err {
		return false, err
	} else {
		d.log.Debug(fmt.Sprintf("calculated hash for %s: %s. File id: %s", fileFullPath, hash, file.Id))
		return file.Hash == hash, nil
	}
}

func (d *Drive) setDownloadTimeByStatsForFile(file contracts.File) error {
	fileFullPath := lfile.GetCurFullPath(file)
	if stat, err := os.Stat(fileFullPath); nil == err {
		return d.fileRepository.SetDownloadTime(file.Id, stat.ModTime())
	} else {
		return errors.Wrapf(err, "could not get the file's %s stats", fileFullPath)
	}
}
