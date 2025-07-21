package core

import (
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"io/fs"
	"marmot/utils"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Logger struct {
	logger *zap.Logger
}

func (l *Logger) GetZap() *zap.Logger {
	return l.logger
}

func (l *Logger) Info(tmp string, args ...interface{}) {
	l.logger.Info(fmt.Sprintf(tmp, args...))
}

func (l *Logger) Warn(tmp string, args ...interface{}) {
	l.logger.Warn(fmt.Sprintf(tmp, args...))
}

func (l *Logger) Error(tmp string, args ...interface{}) {
	l.logger.Error(fmt.Sprintf(tmp, args...))
}

func (l *Logger) Debug(tmp string, args ...interface{}) {
	l.logger.Debug(fmt.Sprintf(tmp, args...))
}

func LogInfo(tmp string, args ...interface{}) {
	Common.Logger.Info(fmt.Sprintf(tmp, args...))
}

func LogWarn(tmp string, args ...interface{}) {
	Common.Logger.Warn(fmt.Sprintf(tmp, args...))
}

func LogError(tmp string, args ...interface{}) {
	Common.Logger.Error(fmt.Sprintf(tmp, args...))
}

func LogDebug(tmp string, args ...interface{}) {
	Common.Logger.Debug(fmt.Sprintf(tmp, args...))
}

func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("15:04:05 -0700"))
}

func customLevelEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	var color string
	switch l {
	case zapcore.DebugLevel:
		color = "\033[36m[D]\033[0m" // aqua
	case zapcore.InfoLevel:
		color = "\033[32m[I]\033[0m" // green
	case zapcore.WarnLevel:
		color = "\033[33m[W]\033[0m" // yellow
	case zapcore.ErrorLevel:
		color = "\033[31m[E]\033[0m" // red
	default:
		color = fmt.Sprintf("%v", l)
	}
	enc.AppendString(color)
}

func customLevelEncoderWithoutColor(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	var level string
	switch l {
	case zapcore.DebugLevel:
		level = "[DEBUG]"
	case zapcore.InfoLevel:
		level = "[INFO]"
	case zapcore.WarnLevel:
		level = "[WARN]"
	case zapcore.ErrorLevel:
		level = "[ERROR]"
	default:
		level = fmt.Sprintf("%v", l)
	}
	enc.AppendString(level)
}

func setupLogFile(r string) (*os.File, error) {
	logPath := filepath.Join(r, "latest.log")

	if utils.IsFileExists(logPath) {
		// format current time
		timestamp := time.Now().Format("2006_01_02_15_04_05")
		backupName := fmt.Sprintf("log_%s.log", timestamp)
		backupPath := filepath.Join(r, backupName)

		// rename file latest.log → log_yyyy_MM_dd_HH_mm_ss.log
		if err := os.Rename(logPath, backupPath); err != nil {
			return nil, fmt.Errorf("log rotate failed: %w", err)
		}
	}

	if AppConfig.AutoCleanOldLogs {
		err := cleanUpOldLogs(r, AppConfig.MaxLogFiles, AppConfig.CleanUpAmount)
		if err != nil {
			fmt.Printf("[WARN] remove old logs failed, err:%v\n", err)
		}
	}

	// open new log file: latest.log
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log: %w", err)
	}
	return logFile, nil
}

type fileInfoWithPath struct {
	path string
	info fs.FileInfo
}

func cleanUpOldLogs(dir string, maxFiles, deleteCount int) error {
	var files []fileInfoWithPath

	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, fileInfoWithPath{path: path, info: info})
		}
		return nil
	})
	if err != nil {
		return err
	}

	if len(files) <= maxFiles {
		return nil
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].info.ModTime().Before(files[j].info.ModTime())
	})

	toDelete := len(files) - maxFiles
	if toDelete > deleteCount {
		toDelete = deleteCount
	}

	for i := 0; i < toDelete; i++ {
		//fmt.Println("deleting :", files[i].path)
		_ = os.Remove(files[i].path)
	}

	return nil
}

func createLogger() *Logger {
	r, s := GetSubDir("logs")
	var logFile *os.File
	if s && AppConfig.RecordLog {
		var err error
		logFile, err = setupLogFile(r)
		if err != nil {
			fmt.Println("[ERROR] Failed to create latest.log for logger", err)
		}
	} else {
		s = false
	}

	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "T",
		LevelKey:       "L",
		NameKey:        "N",
		CallerKey:      "",
		MessageKey:     "M",
		EncodeTime:     customTimeEncoder,
		EncodeLevel:    customLevelEncoder,
		EncodeName:     zapcore.FullNameEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
	}

	consoleEncoder := zapcore.NewConsoleEncoder(encoderCfg)
	consoleWS := zapcore.Lock(os.Stdout)

	if s {
		encoderCfgForFile := zapcore.EncoderConfig{
			TimeKey:        "T",
			LevelKey:       "L",
			NameKey:        "N",
			CallerKey:      "",
			MessageKey:     "M",
			EncodeTime:     customTimeEncoder,
			EncodeLevel:    customLevelEncoderWithoutColor,
			EncodeName:     zapcore.FullNameEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
		}

		fileEncoder := zapcore.NewConsoleEncoder(encoderCfgForFile) // json encoder, use default config
		fileWS := zapcore.AddSync(logFile)

		core := zapcore.NewTee(
			zapcore.NewCore(consoleEncoder, consoleWS, zap.DebugLevel),
			zapcore.NewCore(fileEncoder, fileWS, zap.DebugLevel),
		)

		logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
		realLogger := &Logger{
			logger: logger.Named("bot"),
		}
		return realLogger
	} else {
		core := zapcore.NewTee(
			zapcore.NewCore(consoleEncoder, consoleWS, zap.DebugLevel),
		)

		logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
		realLogger := &Logger{
			logger: logger.Named("bot"),
		}
		return realLogger
	}
}
