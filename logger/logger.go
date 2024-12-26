package logger

import (
	"github.com/smart1986/go-quick/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
	"strings"
	"time"
)

var Logger *zap.SugaredLogger
var offsetTimeHandler ITimeOffset

type ITimeOffset interface {
	GetTimeOffset() int64
}

func NewLogger(c *config.Config) {
	NewLoggerOfTimeOffset(c, nil)
}

func NewLoggerOfTimeOffset(c *config.Config, timeOffsetHandler ITimeOffset) {
	offsetTimeHandler = timeOffsetHandler
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
	}

	zapLevel := zap.InfoLevel
	level := strings.ToLower(c.Log.Level)
	switch level {
	case "debug":
		zapLevel = zap.DebugLevel
	case "info":
		zapLevel = zap.InfoLevel
	case "warn":
		zapLevel = zap.WarnLevel
	case "error":
		zapLevel = zap.ErrorLevel
	}

	var fileCore zapcore.Core
	if c.Log.FileEnable {
		filename := c.Log.File
		if filename == "" {
			filename = "logs/app.log"
		}
		fileSync := zapcore.AddSync(&lumberjack.Logger{
			Filename:   filename,
			MaxSize:    c.Log.MaxSize,
			MaxBackups: c.Log.MaxBack,
			MaxAge:     c.Log.MaxAge,
		})
		fileCore = zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			fileSync,
			zapLevel,
		)
	}

	var consoleCore zapcore.Core
	if c.Log.ConsoleEnable {
		consoleCore = zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig),
			zapcore.AddSync(os.Stdout),
			zapLevel,
		)
	}

	var core zapcore.Core
	if c.Log.ConsoleEnable && c.Log.FileEnable {
		core = zapcore.NewTee(fileCore, consoleCore)
	} else if c.Log.ConsoleEnable {
		core = consoleCore
	} else {
		core = fileCore
	}
	if core == nil {
		core = zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig),
			zapcore.AddSync(os.Stdout),
			zapLevel,
		)
	}

	zlog := zap.New(core, zap.AddCaller())
	defer func() {
		if err := zlog.Sync(); err != nil {
			Logger.Warnf("zap logger sync error: %v", err)
		}
	}()

	Logger = zlog.Sugar()
}

func logWithOffset(level zapcore.Level, args ...interface{}) {
	if offsetTimeHandler != nil && offsetTimeHandler.GetTimeOffset() != 0 {
		timeStr := "[" + time.Unix(offsetTimeHandler.GetTimeOffset(), 0).Format("2006-01-02 15:04:05") + "]"
		args = append([]interface{}{timeStr}, args...)
	}
	Logger.Desugar().WithOptions(zap.AddCallerSkip(2)).Sugar().Log(level, args...)
}
func logWithOffsetFormat(template string, level zapcore.Level, args ...interface{}) {
	if offsetTimeHandler != nil && offsetTimeHandler.GetTimeOffset() != 0 {
		timeStr := "[" + time.Unix(offsetTimeHandler.GetTimeOffset(), 0).Format("2006-01-02 15:04:05") + "]"
		args = append([]interface{}{timeStr}, args...)
	}
	Logger.Desugar().WithOptions(zap.AddCallerSkip(2)).Sugar().Logf(level, template, args...)
}

func Debug(args ...interface{}) {
	logWithOffset(zap.DebugLevel, args...)
}

func Info(args ...interface{}) {
	logWithOffset(zap.InfoLevel, args...)
}

func Warn(args ...interface{}) {
	logWithOffset(zap.WarnLevel, args...)
}

func Error(args ...interface{}) {
	logWithOffset(zap.ErrorLevel, args...)
}

func ErrorWithStack(args ...interface{}) {
	args = append(args, zap.Stack("stack"))
	logWithOffset(zap.ErrorLevel, args...)
}

func Debugf(template string, args ...interface{}) {
	logWithOffsetFormat(template, zap.DebugLevel, args...)
}

func Infof(template string, args ...interface{}) {
	logWithOffsetFormat(template, zap.InfoLevel, args...)
}

func Warnf(template string, args ...interface{}) {
	logWithOffsetFormat(template, zap.WarnLevel, args...)
}

func Errorf(template string, args ...interface{}) {
	logWithOffsetFormat(template, zap.ErrorLevel, args...)
}

func ErrorfWithStack(template string, args ...interface{}) {
	args = append(args, zap.Stack("stack"))
	logWithOffsetFormat(template, zap.ErrorLevel, args...)
}
