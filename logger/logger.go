package logger

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/svetlyi/gdriveapp/contracts"
	"log"
	"os"
	"path/filepath"
	"time"
)

var logPath string

type Logger struct {
	appName        string
	logFileMaxSize int64
	verbosity      uint8
	alsoUseStdout  bool
}

func New(appName string, logFileMaxSize int64, verbosity uint8, alsoUseStdout bool) (Logger, error) {
	l := Logger{appName, logFileMaxSize, verbosity, alsoUseStdout}
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
	l.Info("log file location", logPath)
	return l, nil
}

func (l Logger) Debug(v ...interface{}) {
	if l.verbosity >= contracts.LogDebugLevel {
		l.log(v...)
	}
}
func (l Logger) Info(v ...interface{}) {
	if l.verbosity >= contracts.LogInfoLevel {
		l.log(v...)
	}
}
func (l Logger) Warning(v ...interface{}) {
	if l.verbosity >= contracts.LogWarningLevel {
		l.log(v...)
	}
}
func (l Logger) Error(v ...interface{}) {
	l.log(v...)
}

func (l Logger) log(v ...interface{}) {
	var msg string
	isFirstArgString := "string" == fmt.Sprintf("%T", v[0])

	timeString := time.Now().Format(time.RFC3339)
	switch {
	case isFirstArgString && len(v[1:]) > 0:
		msg = fmt.Sprintf("[%s] %s, %+v", timeString, v[0], v[1:])
	case isFirstArgString:
		msg = fmt.Sprintf("[%s] %s", timeString, v[0])
	default:
		msg = fmt.Sprintf("[%s] %+v", timeString, v)
	}

	if l.alsoUseStdout {
		fmt.Println(msg)
	}
	f := l.getLogFile()
	defer f.Close()
	_, err := f.WriteString(msg + "\n")
	if err != nil {
		log.Fatal(err)
	}
}

func (l Logger) getLogFile() *os.File {
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if nil != err {
		log.Fatal(err)
	}
	return file
}
