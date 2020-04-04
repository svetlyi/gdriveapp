package main

import (
	"database/sql"
	_ "database/sql"
	"fmt"
	_ "fmt"
	"github.com/pkg/errors"
	_ "github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/app"
	"github.com/svetlyi/gdriveapp/config"
	"github.com/svetlyi/gdriveapp/logger"
	"github.com/svetlyi/gdriveapp/rdrive"
	"github.com/svetlyi/gdriveapp/rdrive/auth"
	"github.com/svetlyi/gdriveapp/rdrive/db"
	"github.com/svetlyi/gdriveapp/rdrive/db/file"
	"github.com/svetlyi/gdriveapp/synchronization"
	"golang.org/x/net/context"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"os"
	"path/filepath"
	_ "path/filepath"
)

func main() {
	srv, err := drive.NewService(context.Background(), option.WithTokenSource(auth.GetTokenSource()))
	log := logger.New()
	if err != nil {
		log.Error("unable to retrieve Drive client: %v", err)
		os.Exit(1)
	}

	rdrive.PrintUsageStats(srv.About, log)
	dbInstance := db.New(config.DBPath, log)
	defer dbInstance.Close()
	repository := file.NewRepository(dbInstance, log)

	// first sync changes in the remote drive
	rootFolder, err := repository.GetRootFolder()
	rd := rdrive.New(*srv.Files, *srv.Changes, repository, log, app.New(dbInstance, log))
	//if errors.Cause(err) == sql.ErrNoRows {
	//	if err = rd.FillDb(); nil != err {
	//		log.Error("synchronization error", err)
	//		os.Exit(1)
	//	}
	//} else {
	//	log.Info("the database already exists")
	//	if err := rd.SaveChangesToDb(); nil != err {
	//		log.Error("saving changes to db error", err)
	//		os.Exit(1)
	//	}
	//}
	log.Info("metadata syncing has finished")

	//// now sync changes from the remote (saved in DB on the previous step) to local drive
	synchronizer := synchronization.New(repository, log, dbInstance, rd)
	//if err = synchronizer.SyncRemoteWithLocal(); nil != err {
	//	log.Error("SyncRemoteWithLocal error", err)
	//	os.Exit(1)
	//}
	//log.Info("successfully synchronized")
	//if err = repository.CleanUpDatabase(); nil != err {
	//	log.Error("error cleaning up database", err)
	//	os.Exit(1)
	//}
	//log.Info("cleaned database")

	var parentId string
	deletedFolders, err := repository.GetDeletedFoldersIds()
	if nil != err {
		log.Error(err)
		os.Exit(1)
	}
	filepath.Walk(
		filepath.Join(config.DrivePath, rootFolder.CurRemoteName),
		func(path string, info os.FileInfo, err error) error {
			fmt.Println(path)
			curFilePath := path[len(config.DrivePath):]
			fileId, err := repository.GetFileIdByCurPath(curFilePath, rootFolder)
			if sql.ErrNoRows == errors.Cause(err) {
				if info.IsDir() {
					for _, deletedFolder := range deletedFolders {
						if same, err := synchronizer.AreFoldersTheSame(path, deletedFolder); (nil == err) && same {
							fmt.Println(curFilePath, "moved from", deletedFolder)
						}
					}
				}
				fmt.Println("creating", curFilePath, "in", parentId)
			}
			if info.IsDir() {
				parentId = fileId
			}
			return nil
		},
	)
}
