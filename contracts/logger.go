package contracts

type Logger interface {
	Debug(v ...interface{})
	Info(v ...interface{})
	Warning(v ...interface{})
	Error(v ...interface{})
}

const (
	LogErrorLevel uint8 = iota
	LogInfoLevel
	LogWarningLevel
	LogDebugLevel
)
