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
	"github.com/svetlyi/gdriveapp/synchronization"
	"golang.org/x/net/context"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"os"
)

func main() {
	cfg, err := config.ReadCreateIfNotExist()
	if nil != err {
		fmt.Println("could not get read config", err)
		os.Exit(1)
	}
	log, logErr := logger.New(config.GetAppName(), cfg.LogFileMaxSize, uint8(cfg.LogVerbosity), false)
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

	err = synchronizer.SyncLocalWithRemote(cfg.DrivePath, rootFolder)

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
