package main

import (
	"database/sql"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/app"
	"github.com/svetlyi/gdriveapp/config"
	"github.com/svetlyi/gdriveapp/contracts"
	"github.com/svetlyi/gdriveapp/logger"
	"github.com/svetlyi/gdriveapp/rdrive"
	"github.com/svetlyi/gdriveapp/rdrive/auth"
	"github.com/svetlyi/gdriveapp/rdrive/db"
	"github.com/svetlyi/gdriveapp/rdrive/db/file"
	"github.com/svetlyi/gdriveapp/rdrive/db/file/traverser"
	"golang.org/x/net/context"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"os"
)

func main() {
	srv, err := drive.NewService(context.Background(), option.WithTokenSource(auth.GetTokenSource()))
	log := logger.New()
	if err != nil {
		log.Error("Unable to retrieve Drive client: %v", err)
		os.Exit(1)
	}

	rdrive.PrintUsageStats(srv.About, log)
	dbInstance := db.New(config.DBPath, log)
	defer dbInstance.Close()
	repository := file.NewRepository(dbInstance, log)

	// first sync changes in the remote drive
	_, err = repository.GetRootFolder()
	rd := rdrive.New(*srv.Files, *srv.Changes, repository, log, app.New(dbInstance, log))
	if errors.Cause(err) == sql.ErrNoRows {
		if err = rd.FillDb(); nil != err {
			log.Error("Synchronization error", err)
			os.Exit(1)
		} else {
			log.Info("Syncing has finished", nil)
		}
	} else {
		log.Info("The database already exists", nil)
		if err := rd.SaveChangesToDb(); err != nil {
			log.Error("saving changes to db error", err)
			os.Exit(1)
		}
	}

	// now sync changes from the remote (saved in DB on the previous step) to local drive
	var remoteFilesChan = make(contracts.FilesChan)
	tr := traverser.New(repository, log, dbInstance)
	go tr.TraverseFiles(remoteFilesChan)
	var syncRemoteWithLocalErr error
	for f := range remoteFilesChan {
		log.Debug("traversing over remote files", struct {
			path string
			mime string
		}{
			path: f.CurPath,
			mime: f.MimeType,
		})
		syncRemoteWithLocalErr = rd.SyncRemoteWithLocal(f)
		if syncRemoteWithLocalErr != nil {
			log.Error("synchronization remote with local error", syncRemoteWithLocalErr)
			os.Exit(1)
		}
	}
}
