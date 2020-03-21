package contracts

type Logger interface {
	Debug(msg string, context interface{})
	Info(msg string, context interface{})
	Warning(msg string, context interface{})
	Error(msg string, context interface{})
}
