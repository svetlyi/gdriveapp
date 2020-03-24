package rdrive

import (
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
	"strconv"
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
			os.Exit(1)
		}
		nextPageToken = fileList.NextPageToken

		d.log.Info("Getting files list...", nil)

		for _, gfile := range fileList.Files {
			d.log.Debug("Found file", struct {
				Id           string
				Name         string
				ModifiedTime string
				Trashed      string
			}{
				"id:" + gfile.Id,
				"name:" + gfile.Name,
				"modifiedTime:" + gfile.ModifiedTime,
				"trashed:" + strconv.FormatBool(gfile.Trashed),
			})
			filesChan <- gfile
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

	for {
		if nextPageToken == "" {
			startPageToken, err := d.changesService.GetStartPageToken().Do()
			if err != nil {
				d.log.Error("error getting start page token in changed files list", err)
				os.Exit(1)
			}
			nextPageToken = startPageToken.StartPageToken
			d.log.Info("next page token", nextPageToken)
		}
		changesListCall = d.changesService.List(nextPageToken)
		changeList, err := changesListCall.PageSize(config.PageSizeToQuery).Fields(
			googleapi.Field(fmt.Sprintf("nextPageToken, changes(removed, fileId, file(%s))", fileFieldsSet)),
		).Do()

		if err != nil {
			d.log.Error("Unable to retrieve changed files: %v", err)
			break
		}
		nextPageToken = changeList.NextPageToken

		d.log.Info("Getting changed files list...", len(changeList.Changes))

		for _, change := range changeList.Changes {
			if change.Removed {
				d.log.Debug(fmt.Sprintf("File with %s was removed", change.FileId), nil)
			} else if change.File.Trashed {
				d.log.Debug(fmt.Sprintf("File with %s was trashed", change.FileId), nil)
			} else if change.File.ExplicitlyTrashed {
				d.log.Debug(fmt.Sprintf("File with %s was explicitly trashed", change.FileId), nil)
			} else {
				d.log.Debug("Found change", struct {
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
			break
		} else if err := d.appState.Set(app.NextChangeToken, nextPageToken); err != nil {
			d.log.Error("error saving NextChangeToken to app state", err)
			close(exitChan)
		}
	}
	close(filesChan)
}

func (d *Drive) getRootFolder() *drive.File {
	rootFolder, err := d.filesService.Get("root").Fields(googleapi.Field(fileFieldsSet)).Do()
	if err != nil {
		d.log.Error("Could not fetch root folder info", err)
		os.Exit(1)
	}
	d.log.Debug("Found root folder", struct {
		Id   string
		Name string
	}{
		rootFolder.Id,
		rootFolder.Name,
	})

	return rootFolder
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

	if (localChangeType != contracts.FILE_NOT_CHANGED ||
		remoteChangeType != contracts.FILE_NOT_CHANGED) &&
		canDownloadFile(file) {
		d.log.Debug("SyncRemoteWithLocal. change types", struct {
			local  contracts.FileChangeType
			remote contracts.FileChangeType
			file   string
			path   string
			mime   string
		}{
			local:  localChangeType,
			remote: remoteChangeType,
			file:   file.CurRemoteName,
			mime:   file.MimeType,
		})
	}

	curFullFilePath := lfile.GetCurFullPath(file)
	if isFolder(file) {
		d.log.Debug("Creating folder", struct {
			name string
		}{name: file.CurRemoteName})
		if err := os.Mkdir(curFullFilePath, 0644); !os.IsExist(err) {
			return errors.Wrap(err, "could not create dir")
		}
		return nil
	}
	if !canDownloadFile(file) {
		return nil
	}

	if contracts.FILE_NOT_CHANGED == remoteChangeType {
		if contracts.FILE_NOT_EXIST == localChangeType {
			d.log.Debug("downloading file. remote file has not changed. local one does not exist", struct {
				id   string
				name string
			}{id: file.Id, name: file.CurRemoteName})
			if err = d.download(file); err != nil {
				return errors.Wrapf(err, "could not download file %s", file.Id)
			}
		} else if contracts.FILE_NOT_CHANGED == localChangeType {
			return nil
		} else if contracts.FILE_UPDATED == localChangeType {
			d.log.Debug("updating file remotely", struct {
				id   string
				name string
			}{id: file.Id, name: file.CurRemoteName})
			if err = d.upload(file); err != nil {
				return errors.Wrapf(err, "could not upload file %s", file.Id)
			}
		} else if contracts.FILE_DELETED == localChangeType {
			d.log.Debug("deleting file remotely", struct {
				id   string
				name string
			}{id: file.Id, name: file.CurRemoteName})
			if err = d.delete(file); err != nil {
				return errors.Wrapf(err, "could not delete file %s", file.Id)
			}
		}
	} else if contracts.FILE_UPDATED == remoteChangeType {
		if contracts.FILE_NOT_CHANGED == localChangeType {
			d.log.Debug("downloading file. remote file changed", struct {
				id   string
				name string
			}{id: file.Id, name: file.CurRemoteName})
			if err = d.download(file); err != nil {
				return errors.Wrapf(err, "could not download file %s", file.Id)
			}
		} else if contracts.FILE_UPDATED == localChangeType {
			//TODO: conflict
		} else if contracts.FILE_DELETED == localChangeType {
			//TODO: conflict
		}
	} else if contracts.FILE_DELETED == remoteChangeType {
		if contracts.FILE_NOT_CHANGED == localChangeType {
			d.log.Debug("deleting file locally", struct {
				id   string
				name string
			}{id: file.Id, name: file.CurRemoteName})
			return os.Remove(curFullFilePath)
		} else if contracts.FILE_UPDATED == localChangeType {
			//TODO: conflict
		} else if contracts.FILE_DELETED == localChangeType {
			return nil
		}
	}

	return nil
}

// isChangedLocally determines if the file was changed locally (updated or deleted)
func (d *Drive) isChangedLocally(file contracts.File) (contracts.FileChangeType, error) {
	// if the file does not exist
	if stats, err := os.Stat(lfile.GetCurFullPath(file)); os.IsNotExist(err) {
		if file.DownloadTime.IsZero() {
			return contracts.FILE_NOT_EXIST, nil
		} else {
			return contracts.FILE_DELETED, nil
		}
		// if the file exist
	} else if err == nil {
		if file.DownloadTime.IsZero() {
			return contracts.FILE_UPDATED, nil
		} else if file.DownloadTime.Unix() == stats.ModTime().Unix() { //TODO: Equal does not work
			return contracts.FILE_NOT_CHANGED, nil
		} else {
			return contracts.FILE_UPDATED, nil
		}
	} else {
		return contracts.FILE_ERROR, err
	}
}

// isChangedRemotely determines if the file was changed remotely (updated or deleted)
func (d *Drive) isChangedRemotely(file contracts.File) (contracts.FileChangeType, error) {
	if hasTrashedParent, err := d.fileRepository.HasTrashedParent(file.Id); err == nil {
		if hasTrashedParent {
			d.log.Debug("file has trashed parent", struct {
				Id string
			}{
				Id: file.Id,
			})
			return contracts.FILE_DELETED, nil
		}
	} else {
		return contracts.FILE_ERROR, err
	}
	if file.RemovedRemotely == 1 || file.Trashed == 1 {
		return contracts.FILE_DELETED, nil
	}
	if file.CurRemoteModTime.Equal(file.PrevRemoteModTime) {
		return contracts.FILE_NOT_CHANGED, nil
	} else {
		return contracts.FILE_UPDATED, nil
	}
}

func (d *Drive) upload(file contracts.File) error {
	curFullPath := lfile.GetCurFullPath(file)
	if lf, err := os.Open(curFullPath); err == nil {
		rf, err := d.filesService.Update(file.Id, &drive.File{}).Media(lf).Do()
		if err != nil {
			return errors.Wrap(err, "could not update file remotely")
		}
		if t, err := time.Parse(time.RFC3339, rf.ModifiedTime); err != nil {
			return d.fileRepository.SetRemoteModificationDate(file.Id, t)
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
	gfileReader, err := d.filesService.Get(file.Id).Download()
	if err != nil {
		d.log.Error("Unable to retrieve file: %v", err)
		return err
	}
	fileFullPath := lfile.GetCurFullPath(file)
	lf, err := os.Create(fileFullPath)

	if err == nil {
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
		if stat, err := os.Stat(fileFullPath); err == nil {
			return d.fileRepository.SetDownloadTime(file.Id, stat.ModTime())
		} else {
			return errors.Wrapf(err, "could not get the file's %s stats", fileFullPath)
		}
	}

	return err
}

func isFolder(file contracts.File) bool {
	return file.MimeType == "application/vnd.google-apps.folder"
}

func canDownloadFile(file contracts.File) bool {
	return !strings.Contains(file.MimeType, "application/vnd.google-apps")
}
