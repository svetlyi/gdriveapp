package config

import (
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/contracts"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"
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
	if err := InitCfg(); err != nil {
		log.Fatal("could not initialize config", err)
	}
	if path, err := GetDir(); nil == err {
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

	cfgDir, cfgDirErr := GetDir()
	if nil != cfgDirErr {
		return Cfg{}, errors.Wrap(cfgDirErr, "could not get config dir")
	}
	defaultCfg.DBPath = filepath.Join(cfgDir, "sync.db")
	defaultCfg.DrivePath = usr.HomeDir + string(os.PathSeparator)

	return defaultCfg, nil
}

func GetAppName() string {
	return appName
}

func getCfgPath() (string, error) {
	if path, err := GetDir(); nil == err {
		return filepath.Join(path, "config.json"), nil
	} else {
		return "", err
	}
}

func GetDir() (string, error) {
	usrConfDir, err := os.UserConfigDir()
	if nil != err {
		return "", errors.Wrap(err, "could not get current user's config dir")
	}
	dir := filepath.Join(usrConfDir, appName)

	return dir, err
}
