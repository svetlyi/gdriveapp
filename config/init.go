package config

import (
	"bufio"
	"fmt"
	"github.com/pkg/errors"
	"os"
	"strings"
)

type isParamValid func(param string) bool

// InitCfg creates all the necessary folders and configuration files.
func InitCfg() error {
	if err := createDirIfNotExist(); err != nil {
		return err
	}
	if err := createCfgIfNotExist(); err != nil {
		return err
	}

	return nil
}

func createDirIfNotExist() error {
	dir, err := GetDir()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.Mkdir(dir, 0700)
	}

	return err
}

func createCfgIfNotExist() error {
	if cfgPath, err := getCfgPath(); err != nil {
		return err
	} else if _, err := os.Stat(cfgPath); err != nil {
		if !os.IsNotExist(errors.Cause(err)) {
			return err
		}
	} else {
		return nil
	}

	cfg, err := newDefault()
	if nil != err {
		return errors.Wrap(err, "could not get default config")
	}

	drivePath, err := readParam(
		"Store \"My Drive\" folder in absolute path: ",
		cfg.DrivePath,
		isDrivePathValid,
	)
	if nil != err {
		return errors.Wrap(err, "could not read drive path param")
	}
	cfg.DrivePath = drivePath

	if err = Save(cfg); nil != err {
		return errors.Wrap(err, "could not save config")
	}
	return nil
}

func isDrivePathValid(path string) bool {
	return strings.HasSuffix(path, string(os.PathSeparator)) &&
		strings.HasPrefix(path, string(os.PathSeparator))
}

// readParam reads a new parameter for configuration. It will ask again if the parameter is not valid.
func readParam(hint string, def string, valid isParamValid) (string, error) {
	r := bufio.NewReader(os.Stdin)
	var param string
	var err error
	for {
		fmt.Printf("%s (%s): ", hint, def)
		param, err = r.ReadString('\n')
		param = strings.Trim(param, " \n")
		if nil != err {
			return "", errors.Wrap(err, "could not read param")
		}
		if "" == param {
			return def, nil
		} else if valid(param) {
			return param, nil
		}
	}
}
