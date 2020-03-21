package logger

import (
	"log"
	"os"
)

type Logger struct {
	msgChan chan string
}

func New() Logger {
	l := Logger{msgChan: make(chan string)}
	go l.listen()
	return l
}

func (l Logger) Debug(msg string, context interface{}) {
	l.Info(msg, context)
}
func (l Logger) Info(msg string, context interface{}) {
	log.Printf("%s, %+v", msg, context) // TODO: remove when there is an exit chan
	//l.msgChan <- fmt.Sprintf("%s, %v", msg, context)
}
func (l Logger) Warning(msg string, context interface{}) {
	l.Info(msg, context)
}
func (l Logger) Error(msg string, context interface{}) {
	l.Info(msg, context)
}

func (l Logger) listen() {
	if f, err := os.OpenFile("/tmp/gdrive.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		for msg := range l.msgChan {
			log.Println(msg)
			_, err = f.WriteString(msg + "\n")
			if err != nil {
				log.Fatal(err)
			}
		}
	} else {
		log.Fatal(err)
	}
}
