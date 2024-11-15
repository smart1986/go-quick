package logger

import (
	"github.com/smart1986/go-quick/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"log"
	"os"
	"strings"
)

var Logger *zap.SugaredLogger

func NewLogger(c *config.Config) {
	// Lumberjack configuration for file logging

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000")
	encoderConfig.CallerKey = "caller"

	zapLevel := zap.InfoLevel
	level := c.Log.Level
	level = strings.ToLower(level)
	if level != "" {
		if "debug" == level {
			zapLevel = zap.DebugLevel
		}
		if "info" == level {
			zapLevel = zap.InfoLevel
		}
		if "warn" == level {
			zapLevel = zap.WarnLevel
		}
		if "error" == level {
			zapLevel = zap.ErrorLevel
		}
	}
	errSync := os.Stdout.Sync()

	var fileCore zapcore.Core
	if c.Log.FileEnable {
		filename := c.Log.File
		if filename == "" {
			filename = "logs/app.log"
		}
		maxSize := c.Log.MaxSize
		if maxSize == 0 {
			maxSize = 10
		}
		maxBackups := c.Log.MaxBack
		if maxBackups == 0 {
			maxBackups = 3
		}
		maxAge := c.Log.MaxAge
		if maxAge == 0 {
			maxAge = 28
		}

		fileSync := zapcore.AddSync(&lumberjack.Logger{
			Filename:   filename,
			MaxSize:    maxSize, // megabytes
			MaxBackups: maxBackups,
			MaxAge:     maxAge, // days
		})
		fileCore = zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			fileSync,
			zapLevel,
		)
	}
	var consoleCore zapcore.Core
	if errSync == nil {
		consoleSync := zapcore.AddSync(os.Stdout)

		consoleCore = zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig),
			consoleSync,
			zapLevel,
		)
	}

	// Combine cores
	var core zapcore.Core
	if errSync == nil {
		if c.Log.FileEnable {
			core = zapcore.NewTee(fileCore, consoleCore)
		} else {
			core = consoleCore
		}
	}
	if c.Log.FileEnable && errSync == nil {
		core = zapcore.NewTee(fileCore, consoleCore)
	} else {
		if errSync != nil {
			core = fileCore
		} else {
			core = consoleCore
		}

	}
	if core == nil {
		log.Fatalf("no core")
	}
	zlog := zap.New(core, zap.AddCaller())

	defer func(zlog *zap.Logger) {
		err := zlog.Sync()
		if err != nil {
			log.Fatalf("zap logger sync error: %v", err)
		}
	}(zlog)

	Logger = zlog.Sugar()
}

func Debug(args ...interface{}) {
	Logger.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar().Debug(args...)
}

func Info(args ...interface{}) {
	Logger.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar().Info(args...)
}

func Warn(args ...interface{}) {
	Logger.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar().Warn(args...)
}

func Error(args ...interface{}) {
	Logger.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar().Error(args...)
}

func Debugf(template string, args ...interface{}) {
	Logger.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar().Debugf(template, args...)
}

func Infof(template string, args ...interface{}) {
	Logger.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar().Infof(template, args...)
}

func Warnf(template string, args ...interface{}) {
	Logger.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar().Warnf(template, args...)
}

func Errorf(template string, args ...interface{}) {
	Logger.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar().Errorf(template, args...)
}
