package rdrive

import (
	"crypto/md5"
	"database/sql"
	"fmt"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/app"
	"github.com/svetlyi/gdriveapp/config"
	"github.com/svetlyi/gdriveapp/contracts"
	lfile "github.com/svetlyi/gdriveapp/ldrive/file"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"io"
	"os"
	"strings"
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
		fileList, err := filesListCall.PageSize(config.PageSizeToQuery).Fields(
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
		changeList, err := changesListCall.PageSize(config.PageSizeToQuery).Fields(
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
	d.log.Info("changes:closing files channel")

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
		(canDownloadFile(file) || isFolder(file)) {
		d.log.Debug("SyncRemoteWithLocal. change types", struct {
			file             contracts.File
			localChangeType  contracts.FileChangeType
			remoteChangeType contracts.FileChangeType
		}{file, localChangeType, remoteChangeType})
	}

	curFullFilePath := lfile.GetCurFullPath(file)
	if isFolder(file) {
		switch { // the only things that can happen to a folder is: move, delete
		case contracts.FILE_MOVED == remoteChangeType:
			return d.handleMovedRemotely(file)
		case contracts.FILE_DELETED == remoteChangeType:
			return d.handleRemovedRemotely(file)
		case contracts.FILE_UPDATED == remoteChangeType ||
			(contracts.FILE_NOT_CHANGED == remoteChangeType && contracts.FILE_NOT_EXIST == localChangeType):
			d.log.Debug("creating folder", struct {
				name string
			}{name: file.CurRemoteName})
			if err := os.Mkdir(curFullFilePath, 0644); !os.IsExist(err) {
				return errors.Wrap(err, "could not create dir")
			}
		}
	}
	if !canDownloadFile(file) {
		return nil
	}

	switch {
	case contracts.FILE_NOT_CHANGED == remoteChangeType && contracts.FILE_NOT_EXIST == localChangeType:
		d.log.Debug("downloading file. remote file has not changed. local one does not exist", file)
		if err = d.download(file); err != nil {
			err = errors.Wrapf(err, "could not download file %s", file.Id)
		}
	case contracts.FILE_NOT_CHANGED == remoteChangeType && contracts.FILE_NOT_CHANGED == localChangeType:
		if file.DownloadTime.IsZero() {
			err = d.setDownloadTimeByStats(file)
		}
		break // do nothing
	case contracts.FILE_NOT_CHANGED == remoteChangeType && contracts.FILE_UPDATED == localChangeType:
		d.log.Debug("uploading file. remote file has not changed. local one updated", file)
		if err = d.upload(file); err != nil {
			err = errors.Wrapf(err, "could not upload file %s", file.Id)
		}
	case contracts.FILE_NOT_CHANGED == remoteChangeType && contracts.FILE_DELETED == localChangeType:
		d.log.Debug("deleting file remotely. remote file has not changed. local one deleted", file)
		if err = d.delete(file); err != nil {
			err = errors.Wrapf(err, "could not delete file %s", file.Id)
		}
	case contracts.FILE_UPDATED == remoteChangeType && contracts.FILE_NOT_CHANGED == localChangeType:
		d.log.Debug("downloading file. remote file changed", file)
		if err = d.download(file); err != nil {
			err = errors.Wrapf(err, "could not download file %s", file.Id)
		}
	case contracts.FILE_UPDATED == remoteChangeType && contracts.FILE_UPDATED == localChangeType:
		break //TODO: conflict
	case contracts.FILE_UPDATED == remoteChangeType && contracts.FILE_DELETED == localChangeType:
		break //TODO: conflict
	case contracts.FILE_DELETED == remoteChangeType && contracts.FILE_NOT_CHANGED == localChangeType:
		d.log.Debug("deleting file locally", file)
		err = os.Remove(curFullFilePath)
	case contracts.FILE_DELETED == remoteChangeType && contracts.FILE_UPDATED == localChangeType:
		break //TODO: conflict
	case contracts.FILE_DELETED == remoteChangeType && contracts.FILE_DELETED == localChangeType:
		err = d.fileRepository.Delete(file.Id)
	case contracts.FILE_MOVED == remoteChangeType && contracts.FILE_NOT_CHANGED == localChangeType:
		err = d.handleMovedRemotely(file)
	case contracts.FILE_MOVED == remoteChangeType && contracts.FILE_UPDATED == localChangeType:
		break //TODO: conflict
	case contracts.FILE_MOVED == remoteChangeType && contracts.FILE_DELETED == localChangeType:
		d.log.Debug("downloading file. remote file was moved, local one deleted", file)
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
		err = d.setDownloadTimeByStats(file)
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

func (d *Drive) upload(file contracts.File) error {
	if sameFileExists, err := d.isLocalSameAsRemote(file); err == nil && sameFileExists {
		d.log.Debug(fmt.Sprintf("skipping file %s: already exists", file.Id))
		return d.setDownloadTimeByStats(file) // most probably it was not downloaded previously
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

func (d *Drive) delete(file contracts.File) error {
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
		if err = d.setDownloadTimeByStats(file); err != nil {
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
		return d.setDownloadTimeByStats(file)
	}

	return err
}

// isLocalSameAsRemote checks that a file with the same path, name and hash exists
// if it exists, we won't download it. We need it when for some reason the database was empty
// or the downloaded time in the database is null
func (d *Drive) isLocalSameAsRemote(file contracts.File) (bool, error) {
	fileFullPath := lfile.GetCurFullPath(file)
	f, err := os.Open(fileFullPath)

	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		} else {
			return false, errors.Wrapf(err, "could not check if the file %s exists", fileFullPath)
		}
	}
	defer f.Close()
	if stat, err := f.Stat(); nil == err {
		if stat.IsDir() {
			return true, nil
		}
	} else {
		return false, err
	}

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, errors.Wrapf(err, "could not get the file's %s hash", fileFullPath)
	}

	fileHash := fmt.Sprintf("%x", h.Sum(nil))
	d.log.Debug(fmt.Sprintf("calculated hash for %s: %s. File id: %s", fileFullPath, fileHash, file.Id))

	return file.Hash == fileHash, nil
}

func (d *Drive) setDownloadTimeByStats(file contracts.File) error {
	fileFullPath := lfile.GetCurFullPath(file)
	if stat, err := os.Stat(fileFullPath); nil == err {
		return d.fileRepository.SetDownloadTime(file.Id, stat.ModTime())
	} else {
		return errors.Wrapf(err, "could not get the file's %s stats", fileFullPath)
	}
}

func isFolder(file contracts.File) bool {
	return file.MimeType == "application/vnd.google-apps.folder"
}

func canDownloadFile(file contracts.File) bool {
	return !strings.Contains(file.MimeType, "application/vnd.google-apps")
}
