package logger

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// Logger 全局日志实例
var Logger *logrus.Logger

func init() {
	Logger = logrus.New()

	// 从环境变量读取日志级别，默认为 Info
	logLevel := strings.ToLower(os.Getenv("LOG_LEVEL"))
	switch logLevel {
	case "debug":
		Logger.SetLevel(logrus.DebugLevel)
	case "info":
		Logger.SetLevel(logrus.InfoLevel)
	case "warn", "warning":
		Logger.SetLevel(logrus.WarnLevel)
	case "error":
		Logger.SetLevel(logrus.ErrorLevel)
	case "fatal":
		Logger.SetLevel(logrus.FatalLevel)
	default:
		Logger.SetLevel(logrus.InfoLevel)
	}

	// 从环境变量读取日志格式，默认为 text
	logFormat := strings.ToLower(os.Getenv("LOG_FORMAT"))
	if logFormat == "json" {
		Logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
		})
	} else {
		Logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	}

	// 输出到标准输出
	Logger.SetOutput(os.Stdout)
}

// Debug 输出调试级别日志
func Debug(args ...interface{}) {
	Logger.Debug(args...)
}

// Debugf 输出格式化的调试级别日志
func Debugf(format string, args ...interface{}) {
	Logger.Debugf(format, args...)
}

// Info 输出信息级别日志
func Info(args ...interface{}) {
	Logger.Info(args...)
}

// Infof 输出格式化的信息级别日志
func Infof(format string, args ...interface{}) {
	Logger.Infof(format, args...)
}

// Warn 输出警告级别日志
func Warn(args ...interface{}) {
	Logger.Warn(args...)
}

// Warnf 输出格式化的警告级别日志
func Warnf(format string, args ...interface{}) {
	Logger.Warnf(format, args...)
}

// Error 输出错误级别日志
func Error(args ...interface{}) {
	Logger.Error(args...)
}

// Errorf 输出格式化的错误级别日志
func Errorf(format string, args ...interface{}) {
	Logger.Errorf(format, args...)
}

// Fatal 输出致命错误日志并退出程序
func Fatal(args ...interface{}) {
	Logger.Fatal(args...)
}

// Fatalf 输出格式化的致命错误日志并退出程序
func Fatalf(format string, args ...interface{}) {
	Logger.Fatalf(format, args...)
}

// WithField 添加单个字段到日志
func WithField(key string, value interface{}) *logrus.Entry {
	return Logger.WithField(key, value)
}

// WithFields 添加多个字段到日志（结构化日志）
func WithFields(fields logrus.Fields) *logrus.Entry {
	return Logger.WithFields(fields)
}

// WithError 添加错误字段到日志
func WithError(err error) *logrus.Entry {
	return Logger.WithError(err)
}
