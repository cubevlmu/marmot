package onebot

import (
	"fmt"
)

type ILogger interface {
	Info(msg string)
	Error(msg string)
	Debug(msg string)
	Warn(msg string)
}

var botLogger ILogger

func SetLogger(logger ILogger) {
	botLogger = logger
}

func LogInfo(tmp string, args ...interface{}) {
	botLogger.Info(fmt.Sprintf(tmp, args...))
}

func LogWarn(tmp string, args ...interface{}) {
	botLogger.Warn(fmt.Sprintf(tmp, args...))
}

func LogError(tmp string, args ...interface{}) {
	botLogger.Error(fmt.Sprintf(tmp, args...))
}

func LogDebug(_ string, _ ...interface{}) {
	//botLogger.Debug(fmt.Sprintf(tmp, args...))
}
