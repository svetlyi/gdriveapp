package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/contracts"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

type Cfg struct {
	DBPath          string `json:"db_path"`
	PageSizeToQuery int64  `json:"page_size_to_query"`
	DrivePath       string `json:"drive_path"`
	LogFileMaxSize  int64  `json:"log_file_max_size"`
	LogVerbosity    int64  `json:"log_verbosity"`
}

var appName = "svetlyi_gdriveapp"
var cfgPath string

func init() {
	if path, err := GetCfgDir(); nil == err {
		cfgPath = filepath.Join(path, "config.json")
	} else {
		log.Fatal("could not get configuration dir", err)
	}
}

func Read() (Cfg, error) {
	var cfg Cfg
	fBytes, err := ioutil.ReadFile(cfgPath)
	if nil != err {
		return Cfg{}, errors.Wrapf(err, "could not read config file %s", cfgPath)
	}
	err = json.Unmarshal(fBytes, &cfg)
	if nil != err {
		return Cfg{}, errors.Wrapf(err, "could not parse json in %s", cfgPath)
	}

	return cfg, nil
}

func Save(cfg Cfg) error {
	fBytes, err := json.MarshalIndent(cfg, "", "  ")
	if nil != err {
		return errors.Wrapf(err, "could not create json for %#v", cfg)
	}
	err = ioutil.WriteFile(cfgPath, fBytes, 0755)
	if nil != err {
		return errors.Wrapf(err, "could not write config to %s", cfgPath)
	}
	return nil
}

func newDefault() (Cfg, error) {
	defaultCfg := Cfg{
		DBPath:          "",
		PageSizeToQuery: 300,
		DrivePath:       "",
		LogFileMaxSize:  1e7,
		LogVerbosity:    int64(contracts.LogInfoLevel),
	}
	usr, err := user.Current()
	if nil != err {
		log.Fatal("could not get current user info", err)
	}

	cfgDir, cfgDirErr := GetCfgDir()
	if nil != cfgDirErr {
		return Cfg{}, errors.Wrap(cfgDirErr, "could not get config dir")
	}
	defaultCfg.DBPath = filepath.Join(cfgDir, "sync.db")
	defaultCfg.DrivePath = usr.HomeDir

	return defaultCfg, nil
}

func ReadCreateIfNotExist() (Cfg, error) {
	cfg, err := Read()
	if nil != err {
		if os.IsNotExist(errors.Cause(err)) {
			cfg, err = newDefault()
			if nil != err {
				return cfg, errors.Wrap(err, "could not get default config")
			}
			r := bufio.NewReader(os.Stdin)
			fmt.Printf("Store \"My Drive\" folder in (%s): ", cfg.DrivePath)
			drivePath, err := r.ReadString('\n')
			if nil != err {
				return cfg, errors.Wrap(err, "could not read custom folder")
			}
			if "" != drivePath {
				cfg.DrivePath = strings.Trim(drivePath, " \n")
			}
			if err = Save(cfg); nil != err {
				return cfg, errors.Wrap(err, "could not save config")
			}
		}
	}

	return cfg, err
}

func GetCfgDir() (string, error) {
	usrConfDir, err := os.UserConfigDir()
	if nil != err {
		return "", errors.Wrap(err, "could not get current user's config dir")
	}

	return filepath.Join(usrConfDir, appName), nil
}

func GetAppName() string {
	return appName
}
