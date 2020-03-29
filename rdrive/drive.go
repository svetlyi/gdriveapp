package rdrive

import (
	"database/sql"
	"fmt"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/app"
	"github.com/svetlyi/gdriveapp/contracts"
	"github.com/svetlyi/gdriveapp/rdrive/db/file"
	"google.golang.org/api/drive/v3"
	"math"
	"os"
	"time"
)

type Drive struct {
	filesService   drive.FilesService
	changesService drive.ChangesService
	fileRepository file.Repository
	appState       app.Store
	log            contracts.Logger
}

func New(
	filesService drive.FilesService,
	changesService drive.ChangesService,
	fileRepository file.Repository,
	log contracts.Logger,
	appState app.Store,
) Drive {
	return Drive{
		filesService:   filesService,
		changesService: changesService,
		fileRepository: fileRepository,
		log:            log,
		appState:       appState,
	}
}

// FillDb synchronizes information about the files in google drive with the information
// about them in the local database. Having the information locally
// saves us from querying the server many times. With that information we can
// easily find a deleted, modified file not only by modification date,
// but also by hash and full path
func (d *Drive) FillDb() error {
	var filesChan = make(chan *drive.File)

	var rootFolder = d.getRootFolder()
	var err error

	if err = d.fileRepository.SaveRootFolder(rootFolder); err != nil {
		d.log.Error("Error saving root folder", err)
		return err
	}

	go d.getFilesList(filesChan)
	for gfile := range filesChan {
		if _, err = d.fileRepository.GetFileById(gfile.Id); err == nil {
			if t, err := time.Parse(time.RFC3339, gfile.ModifiedTime); err != nil {
				return d.fileRepository.SetCurRemoteData(gfile.Id, t, gfile.Name, gfile.Parents)
			}
		} else if sql.ErrNoRows == errors.Cause(err) { // if gfile is a new file in the remote drive
			d.log.Debug("creating file in db", struct {
				id   string
				name string
			}{
				id:   gfile.Id,
				name: gfile.Name,
			})
			if err = d.fileRepository.CreateFile(gfile); err != nil {
				return errors.Wrap(err, "error creating a new file in db in FillDb")
			}
		} else {
			return errors.Wrap(err, "error getting file by id in FillDb")
		}
	}
	return nil
}

func (d *Drive) SaveChangesToDb() error {
	var changesChan = make(chan *drive.Change)
	var exitChan = make(contracts.ExitChan)

	go d.getChangedFilesList(changesChan, exitChan)

	var err error = nil
	for {
		select {
		case change, ok := <-changesChan:
			if !ok { //no more changes
				d.log.Debug("changesChan closed", nil)
				return nil
			}
			// we do not have a trash been here, so we mark just as removed
			if change.Removed || change.File.Trashed || change.File.ExplicitlyTrashed {
				d.log.Debug("changes:removed", struct{ id string }{id: change.FileId})

				if err = d.fileRepository.SetRemovedRemotely(change.FileId); err != nil {
					return errors.Wrap(err, "could not SetRemovedRemotely")
				}
			} else {
				if _, err = d.fileRepository.GetFileById(change.FileId); err == nil {
					if t, err := time.Parse(time.RFC3339, change.File.ModifiedTime); err == nil {
						d.log.Debug("changes:setting remote data", struct {
							id   string
							name string
						}{
							id:   change.FileId,
							name: change.File.Name,
						})
						if err = d.fileRepository.SetCurRemoteData(change.FileId, t, change.File.Name, change.File.Parents); err != nil {
							return errors.Wrap(err, "could not SetCurRemoteData")
						}
					} else {
						return errors.Wrapf(err, "could not parse modified time %v", change.File.ModifiedTime)
					}
				} else if sql.ErrNoRows == errors.Cause(err) { // if gfile is a new file in the remote drive
					d.log.Debug("changes:creating a new file in db", struct {
						id   string
						name string
					}{
						id:   change.FileId,
						name: change.File.Name,
					})
					if err = d.fileRepository.CreateFile(change.File); err != nil {
						return errors.Wrap(err, "could not CreateFile in db")
					}
				} else {
					return errors.Wrap(err, "could not GetFileById")
				}
			}
		case <-exitChan:
			err = errors.New("saving changes to db error")
		case <-time.After(10 * time.Second):
			err = errors.New("timeout error")
		}

		if err != nil {
			return err
		}
	}
}

func PrintUsageStats(aboutService *drive.AboutService, log contracts.Logger) {
	aboutData, err := aboutService.Get().Fields("storageQuota").Do()
	if err != nil {
		log.Error("Unable to retrieve About data: %v", err)
		os.Exit(1)
	}
	fmt.Println("Usage stats:")
	fmt.Printf("Used: %.3f GB\n", float64(aboutData.StorageQuota.Usage)/math.Pow(1024, 2))
}
