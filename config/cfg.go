package config

import (
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

var (
	AppName               = "svetlyi_gdriveapp"
	ConfigPath            = ""
	DBPath                = ""
	PageSizeToQuery int64 = 300
	DrivePath             = ""
	LogFileMaxSize  int64 = 1e7
)

func init() {
	usr, err := user.Current()
	if nil != err {
		log.Fatal("could not get current user info", err)
	}
	confDir, err := os.UserConfigDir()
	if nil != err {
		log.Fatal("could not get current user's config dir", err)
	}

	ConfigPath = filepath.Join(confDir, AppName)
	DBPath = filepath.Join(ConfigPath, "sync.db")

	if "" == DrivePath {
		DrivePathEnv := os.Getenv(strings.ToUpper(AppName) + "_DRIVE_PATH")
		if "" == DrivePathEnv {
			DrivePath = usr.HomeDir
		} else {
			DrivePath = DrivePathEnv
		}
	}
}
