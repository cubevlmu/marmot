package core

import (
	"go.uber.org/zap"
)

type BotLogger struct {
	logger *zap.Logger
}

func (Z *BotLogger) Info(msg string) {
	Z.logger.Info(msg)
}

func (Z *BotLogger) Error(msg string) {
	Z.logger.Error(msg)
}

func (Z *BotLogger) Debug(msg string) {
	Z.logger.Debug(msg)
}

func (Z *BotLogger) Warn(msg string) {
	Z.logger.Warn(msg)
}

func NewZBLogger() *BotLogger {
	return &BotLogger{
		logger: Common.Logger.GetZap(),
	}
}
