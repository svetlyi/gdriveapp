package logger

import (
	"fmt"
	"github.com/pkg/errors"
	"log"
	"os"
	"path/filepath"
)

var logPath string

type Logger struct {
	appName        string
	logFileMaxSize int64
}

func New(appName string, logFileMaxSize int64) (Logger, error) {
	l := Logger{appName, logFileMaxSize}
	logPath = filepath.Join(os.TempDir(), appName+".log")
	if stat, err := os.Stat(logPath); nil == err {
		if stat.Size() > logFileMaxSize {
			if err = os.Remove(logPath); nil != err {
				return Logger{}, errors.Wrapf(err, "could not remove log file %s", logPath)
			}
		}
	} else if !os.IsNotExist(err) {
		return Logger{}, errors.Wrapf(err, "could not open log file %s", logPath)
	}
	return l, nil
}

func (l Logger) Debug(v ...interface{}) {
	l.Info(v...)
}
func (l Logger) Info(v ...interface{}) {
	var msg string
	isFirstArgString := "string" == fmt.Sprintf("%T", v[0])

	switch {
	case isFirstArgString && len(v[1:]) > 0:
		msg = fmt.Sprintf("%s, %+v", v[0], v[1:])
	case isFirstArgString:
		msg = fmt.Sprintf("%s", v[0])
	default:
		msg = fmt.Sprintf("%+v", v)
	}

	log.Println(msg) // TODO: remove after
	f := l.getLogFile()
	defer f.Close()
	_, err := f.WriteString(msg + "\n")
	if err != nil {
		log.Fatal(err)
	}
}
func (l Logger) Warning(v ...interface{}) {
	l.Info(v...)
}
func (l Logger) Error(v ...interface{}) {
	l.Info(v...)
}
func (l Logger) getLogFile() *os.File {
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if nil != err {
		log.Fatal(err)
	}
	return file
}
