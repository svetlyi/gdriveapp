package config

import (
	"log"
	"os"
	"os/user"
	"path/filepath"
)

var (
	ConfigPath            = ""
	DBPath                = ""
	PageSizeToQuery int64 = 300
	DrivePath             = "/media/photon/371F40450619A640/"
)

func init() {
	// TODO: change the logic (temporary)
	if usr, err := user.Current(); nil == err {
		if confDir, err := os.UserConfigDir(); nil == err {
			ConfigPath = filepath.Join(confDir, "gdriveapp")
			DBPath = filepath.Join(ConfigPath, "sync.db")
			if "" == DrivePath {
				DrivePath = usr.HomeDir
			}
		} else {
			log.Fatal(err)
		}
	} else {
		log.Fatal(err)
	}
}
