package logger

import (
	"fmt"
	"github.com/smart1986/go-quick/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
	"strings"
	"time"
)

type (
	QuickLogger struct {
		Logger      *zap.SugaredLogger
		LogLevel    zapcore.Level
		TimeHandler ITimeOffset
	}
	ITimeOffset interface {
		GetTimeOffset() int64
		GetNowSecond() int64
	}
)

var DefaultLogger *QuickLogger

func NewLogger(c *config.Config) {
	q := &QuickLogger{}
	q.NewLoggerOfTimeOffset(c, nil, false)
	DefaultLogger = q
}
func NewLoggerOfTimeOffset(c *config.Config, timeOffsetHandler ITimeOffset, fileFormatJson bool) {
	q := &QuickLogger{}
	q.NewLoggerOfTimeOffset(c, timeOffsetHandler, fileFormatJson)
	DefaultLogger = q
}

func (q *QuickLogger) NewLogger(c *config.Config) {
	q.NewLoggerOfTimeOffset(c, nil, false)
}

func (q *QuickLogger) NewLoggerOfTimeOffset(c *config.Config, timeOffsetHandler ITimeOffset, fileFormatJson bool) {
	q.TimeHandler = timeOffsetHandler
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
	q.LogLevel = zapLevel
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
		var encoder zapcore.Encoder
		if fileFormatJson {
			encoder = zapcore.NewConsoleEncoder(encoderConfig)
		} else {
			encoder = zapcore.NewConsoleEncoder(encoderConfig)
		}

		fileCore = zapcore.NewCore(
			encoder,
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
			fmt.Printf("zap logger sync error: %v", err)
		}
	}()

	q.Logger = zlog.Sugar()
}

func logWithOffset(q *QuickLogger, level zapcore.Level, args ...interface{}) {
	if q.TimeHandler != nil && q.TimeHandler.GetTimeOffset() != 0 {
		timeStr := "[" + time.Unix(q.TimeHandler.GetNowSecond(), 0).Format("2006-01-02 15:04:05") + "] "
		args = append([]interface{}{timeStr}, args...)
	}
	q.Logger.Desugar().WithOptions(zap.AddCallerSkip(2)).Sugar().Log(level, args...)
}
func logWithOffsetFormat(q *QuickLogger, template string, level zapcore.Level, args ...interface{}) {
	if q.TimeHandler != nil && q.TimeHandler.GetTimeOffset() != 0 {
		timeStr := "[" + time.Unix(q.TimeHandler.GetNowSecond(), 0).Format("2006-01-02 15:04:05") + "] "
		args = append([]interface{}{timeStr}, args...)
	}
	q.Logger.Desugar().WithOptions(zap.AddCallerSkip(2)).Sugar().Logf(level, template, args...)
}

func (q *QuickLogger) Debug(args ...interface{}) {
	logWithOffset(q, zap.DebugLevel, args...)
}

func (q *QuickLogger) Info(args ...interface{}) {
	logWithOffset(q, zap.InfoLevel, args...)
}

func (q *QuickLogger) Warn(args ...interface{}) {
	logWithOffset(q, zap.WarnLevel, args...)
}

func (q *QuickLogger) Error(args ...interface{}) {
	logWithOffset(q, zap.ErrorLevel, args...)
}

func (q *QuickLogger) ErrorWithStack(args ...interface{}) {
	args = append(args, zap.Stack("stack"))
	logWithOffset(q, zap.ErrorLevel, args...)
}

func (q *QuickLogger) Debugf(template string, args ...interface{}) {
	logWithOffsetFormat(q, template, zap.DebugLevel, args...)
}

func (q *QuickLogger) Infof(template string, args ...interface{}) {
	logWithOffsetFormat(q, template, zap.InfoLevel, args...)
}

func (q *QuickLogger) Warnf(template string, args ...interface{}) {
	logWithOffsetFormat(q, template, zap.WarnLevel, args...)
}

func (q *QuickLogger) Errorf(template string, args ...interface{}) {
	logWithOffsetFormat(q, template, zap.ErrorLevel, args...)
}

func (q *QuickLogger) ErrorfWithStack(template string, args ...interface{}) {
	args = append(args, zap.Stack("stack"))
	logWithOffsetFormat(q, template, zap.ErrorLevel, args...)
}

func Debug(args ...interface{}) {
	logWithOffset(DefaultLogger, zap.DebugLevel, args...)
}

func Info(args ...interface{}) {
	logWithOffset(DefaultLogger, zap.InfoLevel, args...)
}

func Warn(args ...interface{}) {
	logWithOffset(DefaultLogger, zap.WarnLevel, args...)
}

func Error(args ...interface{}) {
	logWithOffset(DefaultLogger, zap.ErrorLevel, args...)
}

func ErrorWithStack(args ...interface{}) {
	args = append(args, zap.Stack("stack"))
	logWithOffset(DefaultLogger, zap.ErrorLevel, args...)
}

func Debugf(template string, args ...interface{}) {
	logWithOffsetFormat(DefaultLogger, template, zap.DebugLevel, args...)
}

func Infof(template string, args ...interface{}) {
	logWithOffsetFormat(DefaultLogger, template, zap.InfoLevel, args...)
}

func Warnf(template string, args ...interface{}) {
	logWithOffsetFormat(DefaultLogger, template, zap.WarnLevel, args...)
}

func Errorf(template string, args ...interface{}) {
	logWithOffsetFormat(DefaultLogger, template, zap.ErrorLevel, args...)
}

func ErrorfWithStack(template string, args ...interface{}) {
	args = append(args, zap.Stack("stack"))
	logWithOffsetFormat(DefaultLogger, template, zap.ErrorLevel, args...)
}
