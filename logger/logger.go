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

func (l Logger) Debug(msg string, context interface{}) {
	l.Info(msg, context)
}
func (l Logger) Info(msg string, context interface{}) {
	msg = fmt.Sprintf("%s, %+v", msg, context)
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
func (l Logger) Warning(msg string, context interface{}) {
	l.Info(msg, context)
}
func (l Logger) Error(msg string, context interface{}) {
	l.Info(msg, context)
}
