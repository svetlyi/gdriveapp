package main

import (
	"database/sql"
	"fmt"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/app"
	"github.com/svetlyi/gdriveapp/config"
	"github.com/svetlyi/gdriveapp/logger"
	"github.com/svetlyi/gdriveapp/rdrive"
	"github.com/svetlyi/gdriveapp/rdrive/auth"
	"github.com/svetlyi/gdriveapp/rdrive/db"
	"github.com/svetlyi/gdriveapp/rdrive/db/file"
	"github.com/svetlyi/gdriveapp/structures"
	"github.com/svetlyi/gdriveapp/synchronization"
	"golang.org/x/net/context"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	cfg, err := config.ReadCreateIfNotExist()
	if nil != err {
		fmt.Println("could not get read config", err)
		os.Exit(1)
	}
	log, logErr := logger.New(config.GetAppName(), cfg.LogFileMaxSize, uint8(cfg.LogVerbosity))
	if nil != logErr {
		fmt.Println("could not create logger", logErr)
		os.Exit(1)
	}

	log.Info("directory to store \"My Drive\"", cfg.DrivePath)
	cfgDir, cfgDirErr := config.GetCfgDir()
	if nil != cfgDirErr {
		log.Error("could not get config dir", cfgDirErr)
		os.Exit(1)
	}
	tokenSource, tokenSourceErr := auth.GetTokenSource(cfgDir)
	if nil != tokenSourceErr {
		log.Error("could not get token source", tokenSourceErr)
		os.Exit(1)
	}
	srv, err := drive.NewService(context.Background(), option.WithTokenSource(tokenSource))
	if err != nil {
		log.Error("unable to retrieve Drive client: %v", err)
		os.Exit(1)
	}

	rdrive.PrintUsageStats(srv.About, log)
	dbInstance := db.New(cfg.DBPath, log)
	defer dbInstance.Close()
	repository := file.NewRepository(dbInstance, log)

	// first sync changes in the remote drive
	rootFolder, err := repository.GetRootFolder()
	rd := rdrive.New(*srv.Files, *srv.Changes, repository, log, app.New(dbInstance, log), cfg.PageSizeToQuery)
	if errors.Cause(err) == sql.ErrNoRows {
		if err = rd.FillDb(); nil != err {
			log.Error("synchronization error", err)
			os.Exit(1)
		}
		if rootFolder, err = repository.GetRootFolder(); nil != err {
			log.Error("could not get root folder", err)
			os.Exit(1)
		}
	} else {
		log.Info("the database already exists")
		if err := rd.SaveChangesToDb(); nil != err {
			log.Error("saving changes to db error", err)
			os.Exit(1)
		}
	}
	log.Info("metadata syncing has finished")

	// now sync changes from the remote (saved in DB on the previous step) to local drive
	synchronizer := synchronization.New(repository, log, dbInstance, rd)
	if err = synchronizer.SyncRemoteWithLocal(); nil != err {
		log.Error("SyncRemoteWithLocal error", err)
		os.Exit(1)
	}
	log.Info("successfully synchronized")

	locallyRemovedFoldersIds, err := repository.GetLocallyRemovedFoldersIds()
	if nil != err {
		log.Error(err)
		os.Exit(1)
	}
	var parentsStack structures.StringStack
	parentsStack.Push(rootFolder.Id)
	var curDepth int
	var parentId string

	err = filepath.Walk(
		filepath.Join(cfg.DrivePath, rootFolder.CurRemoteName),
		func(path string, info os.FileInfo, err error) error {
			if nil != err {
				return errors.Wrapf(err, "cold not walk in path %s", path)
			}
			log.Debug("next local path", path)
			curRelativeFilePath := path[len(cfg.DrivePath):]
			fileId, fileIdErr := repository.GetFileIdByCurPath(curRelativeFilePath, rootFolder)

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
			log.Debug("depth info", struct {
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
					currentParentId, err := repository.GetFileParentIdByCurPath(curRelativeFilePath, rootFolder)
					if nil != err {
						return errors.Wrapf(err, "could not GetFileParentIdByCurPath for %s", curRelativeFilePath)
					}
					// if it is a dir, first guess, it was moved from somewhere else
					// so, we are looking for the moved dir among the locally removed
					var hasSameRemFolder = false
					var locallyRemovedFolderId string
					for _, locallyRemovedFolderId = range locallyRemovedFoldersIds {
						if hasSameRemFolder, err = synchronizer.AreFoldersTheSame(path, locallyRemovedFolderId); nil != err {
							return errors.Wrapf(
								err,
								"error while checking if the folders %s(path) and %s(id) are the same",
								path,
								locallyRemovedFolderId,
							)
						}
					}
					if hasSameRemFolder {
						oldParentId, err := repository.GetParentIdByChildId(locallyRemovedFolderId)
						if nil != err {
							return errors.Wrapf(err, "could not GetParentIdByChildId for file id %s", locallyRemovedFolderId)
						}
						log.Debug("local move detected", struct {
							movedFolderId   string
							currentParentId string
							currentName     string
						}{locallyRemovedFolderId, currentParentId, info.Name()})
						// at this point it is known, that the folder with id locallyRemovedFolderId was
						// moved from a folder with id oldParentId to a folder with id currentParentId and now
						// the moved folder has name info.Name(). This is the information, that goes to the database
						f, err := rd.Update(locallyRemovedFolderId, info.Name(), []string{currentParentId}, []string{oldParentId})
						if nil != err {
							return err
						}
						fileId = f.Id
						err = repository.SetRemovedLocally(locallyRemovedFolderId, false)
						if nil != err {
							return err
						}
						err = repository.SetCurRemoteData(locallyRemovedFolderId, f.ModifiedTime, f.Name, f.Parents)
						if nil != err {
							return err
						}
						err = repository.SetPrevRemoteDataToCur(locallyRemovedFolderId)
						if nil != err {
							return err
						}
					} else {
						log.Info("creating folder", struct {
							path     string
							parentId string
						}{path, parentId})
						if fileId, err = rd.CreateFolder(path, []string{parentId}); nil != err {
							return errors.Wrapf(err, "could not create folder %s", path)
						}
					}
				} else {
					log.Info("creating file", path, "in", parentId)
					if err = rd.Upload(path, []string{parentId}); nil != err {
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
	if nil != err {
		log.Error(err)
		os.Exit(1)
	}
	if err = repository.CleanUpDatabase(); nil != err {
		log.Error("error cleaning up database", err)
		os.Exit(1)
	}
	log.Debug("cleaned database from old files")
}
