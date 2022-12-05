package models

import (
	"io"
	"log"
	"os"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/natefinch/lumberjack"
	"github.com/sirupsen/logrus"
)

type Logger struct {
	LogName string
}

func NewLogger(logName string) *logrus.Entry {
	return logrus.WithFields(logrus.Fields{
		"name": logName,
	})
}

func getFileRotation(filename string, maxSize, maxBackup, maxAge int, compress bool) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    maxSize,
		MaxBackups: maxBackup,
		MaxAge:     maxAge,
		Compress:   compress,
	}
}

func setLogLevel(level string) {
	switch level {
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
	case "info":
		logrus.SetLevel(logrus.InfoLevel)
	case "trace":
		logrus.SetLevel(logrus.TraceLevel)
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
	default:
		log.Fatalf("Config Error, The alternative is [debug, info, trace, error]")
	}
}

func InitLogger(filename string, maxSize, maxBackup, maxAge int, logLevel string, compress bool) {
	setLogLevel(logLevel)

	stdWriter := os.Stdout
	fileWriter := getFileRotation(filename, maxSize, maxBackup, maxAge, compress)
	logrus.SetOutput(io.MultiWriter(stdWriter, fileWriter))

	logrus.SetFormatter(&nested.Formatter{
		HideKeys:        true,
		TimestampFormat: "2006/01/02 15:04:05",
		FieldsOrder:     []string{"component", "category"},
	})
}
