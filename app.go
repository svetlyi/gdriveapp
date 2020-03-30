package main

import (
	"database/sql"
	"github.com/pkg/errors"
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
		}
	} else {
		log.Info("The database already exists", nil)
		if err := rd.SaveChangesToDb(); nil != err {
			log.Error("saving changes to db error", err)
			os.Exit(1)
		}
	}
	log.Info("Metadata syncing has finished", nil)

	// now sync changes from the remote (saved in DB on the previous step) to local drive
	tr := synchronization.New(repository, log, dbInstance, rd)
	if err = tr.SyncRemoteWithLocal(); nil != err {
		log.Error("SyncRemoteWithLocal error", err)
		os.Exit(1)
	} else if err = repository.CleanUpDatabase(); nil != err {
		log.Error("error cleaning up database", err)
		os.Exit(1)
	}
}
