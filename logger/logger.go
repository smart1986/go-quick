package logger

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/smart1986/go-quick/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
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
	q.initLogger(c, nil, false)
	DefaultLogger = q
}

func NewLoggerOfTimeOffset(c *config.Config, timeOffsetHandler ITimeOffset, fileFormatJson bool) {
	q := &QuickLogger{}
	q.initLogger(c, timeOffsetHandler, fileFormatJson)
	DefaultLogger = q
}

func (q *QuickLogger) NewLogger(c *config.Config) {
	q.initLogger(c, nil, false)
}

// 初始化核心逻辑
func (q *QuickLogger) initLogger(c *config.Config, timeOffsetHandler ITimeOffset, fileFormatJson bool) {
	q.TimeHandler = timeOffsetHandler

	// 时间编码
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000")

	// 日志等级
	level := strings.ToLower(c.Log.Level)
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zap.DebugLevel
	case "info":
		zapLevel = zap.InfoLevel
	case "warn":
		zapLevel = zap.WarnLevel
	case "error":
		zapLevel = zap.ErrorLevel
	default:
		zapLevel = zap.InfoLevel
	}
	q.LogLevel = zapLevel

	// 文件输出
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
			encoder = zapcore.NewJSONEncoder(encoderConfig)
		} else {
			encoder = zapcore.NewConsoleEncoder(encoderConfig)
		}
		fileCore = zapcore.NewCore(encoder, fileSync, zapLevel)
	}

	// 控制台输出
	var consoleCore zapcore.Core
	if c.Log.ConsoleEnable {
		consoleCore = zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig),
			zapcore.AddSync(os.Stdout),
			zapLevel,
		)
	}

	// 组合输出
	var core zapcore.Core
	if c.Log.ConsoleEnable && c.Log.FileEnable {
		core = zapcore.NewTee(fileCore, consoleCore)
	} else if c.Log.ConsoleEnable {
		core = consoleCore
	} else if c.Log.FileEnable {
		core = fileCore
	} else {
		core = zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig),
			zapcore.AddSync(os.Stdout),
			zapLevel,
		)
	}

	// 创建 logger
	zlog := zap.New(core, zap.AddCaller())
	q.Logger = zlog.Sugar()
}

// 进程退出时调用，安全关闭
func (q *QuickLogger) Close() {
	if q != nil && q.Logger != nil {
		_ = q.Logger.Sync()
	}
}

// ============= 内部通用函数 =============
func logWithOffset(q *QuickLogger, level zapcore.Level, args ...interface{}) {
	if q == nil || q.Logger == nil {
		fmt.Println(args...)
		return
	}
	if q.TimeHandler != nil && q.TimeHandler.GetTimeOffset() != 0 {
		timeStr := "[" + time.Unix(q.TimeHandler.GetNowSecond(), 0).Format("2006-01-02 15:04:05") + "] "
		args = append([]interface{}{timeStr}, args...)
	}
	q.Logger.Desugar().WithOptions(zap.AddCallerSkip(2)).Sugar().Log(level, args...)
}

func logWithOffsetFormat(q *QuickLogger, template string, level zapcore.Level, args ...interface{}) {
	if q == nil || q.Logger == nil {
		fmt.Printf(template+"\n", args...)
		return
	}
	if q.TimeHandler != nil && q.TimeHandler.GetTimeOffset() != 0 {
		timeStr := "[" + time.Unix(q.TimeHandler.GetNowSecond(), 0).Format("2006-01-02 15:04:05") + "] "
		args = append([]interface{}{timeStr}, args...)
	}
	q.Logger.Desugar().WithOptions(zap.AddCallerSkip(2)).Sugar().Logf(level, template, args...)
}

// ============= 实例方法 =============
func (q *QuickLogger) Debug(args ...interface{}) { logWithOffset(q, zap.DebugLevel, args...) }
func (q *QuickLogger) Info(args ...interface{})  { logWithOffset(q, zap.InfoLevel, args...) }
func (q *QuickLogger) Warn(args ...interface{})  { logWithOffset(q, zap.WarnLevel, args...) }
func (q *QuickLogger) Error(args ...interface{}) { logWithOffset(q, zap.ErrorLevel, args...) }

func (q *QuickLogger) ErrorWithStack(args ...interface{}) {
	args = append(args, string(debug.Stack()))
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
	args = append(args, string(debug.Stack()))
	logWithOffsetFormat(q, template, zap.ErrorLevel, args...)
}

// ============= 全局快捷函数 =============
func Debug(args ...interface{}) { logWithOffset(DefaultLogger, zap.DebugLevel, args...) }
func Info(args ...interface{})  { logWithOffset(DefaultLogger, zap.InfoLevel, args...) }
func Warn(args ...interface{})  { logWithOffset(DefaultLogger, zap.WarnLevel, args...) }
func Error(args ...interface{}) { logWithOffset(DefaultLogger, zap.ErrorLevel, args...) }
func ErrorWithStack(args ...interface{}) {
	args = append(args, string(debug.Stack()))
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
	args = append(args, string(debug.Stack()))
	logWithOffsetFormat(DefaultLogger, template, zap.ErrorLevel, args...)
}
