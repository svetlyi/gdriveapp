package logger

import (
	"fmt"
	"github.com/svetlyi/gdriveapp/config"
	"log"
	"os"
	"path/filepath"
)

var logPath string

func init() {
	logPath = filepath.Join(os.TempDir(), config.AppName+".log")
	if stat, err := os.Stat(logPath); nil == err {
		if stat.Size() > config.LogFileMaxSize {
			if err = os.Remove(logPath); nil != err {
				log.Fatal("could not open remove log file", logPath, err)
			}
		}
	} else if !os.IsNotExist(err) {
		log.Fatal("could not open log file", logPath, err)
	}
}

type Logger struct {
}

func New() Logger {
	return Logger{}
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
