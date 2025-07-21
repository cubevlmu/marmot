package core

import (
	"fmt"
	"go.uber.org/zap"
)

type ZBLogger struct {
	logger *zap.Logger
}

func NewZBLogger() *ZBLogger {
	return &ZBLogger{
		logger: Common.Logger.GetZap(),
	}
}

func (z *ZBLogger) Info(tmp string) {
	z.logger.Info(tmp)
}

func (z *ZBLogger) Error(tmp string) {
	z.logger.Error(tmp)
}

func (z *ZBLogger) Warn(tmp string) {
	z.logger.Warn(tmp)
}

// disabled

func (z *ZBLogger) Debug(_ string) {
	//return
	//z.logger.Debug(tmp)
}

func (z *ZBLogger) Trace(tmp string) {
	z.logger.Info(fmt.Sprintf("Trace: %s", tmp))
}
