package logger

import (
	"fmt"
	"log"
	"os"
)

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
	if "string" == fmt.Sprintf("%T", v[0]) {
		if len(v[1:]) > 0 {
			msg = fmt.Sprintf("%s, %+v", v[0], v[1:])
		} else {
			msg = fmt.Sprintf("%s", v[0])
		}
	} else {
		msg = fmt.Sprintf("%+v", v)
	}

	log.Println(msg) // TODO: remove after
	if f, err := os.OpenFile("/media/photon/371F40450619A640/gdrive.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		defer f.Close()
		_, err = f.WriteString(msg + "\n")
		if err != nil {
			log.Fatal(err)
		}
	} else {
		log.Fatal(err)
	}
}
func (l Logger) Warning(v ...interface{}) {
	l.Info(v...)
}
func (l Logger) Error(v ...interface{}) {
	l.Info(v...)
}
