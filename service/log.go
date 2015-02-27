package service

import (
	"log"
)

// Log configures optional loggers.
type Log struct {
	ErrorLogger *log.Logger
	InfoLogger  *log.Logger
	DebugLogger *log.Logger
}

func (l *Log) Error(args ...interface{}) {
	if l.ErrorLogger != nil {
		l.ErrorLogger.Print(args...)
	}
}

func (l *Log) Errorf(fmt string, args ...interface{}) {
	if l.ErrorLogger != nil {
		l.ErrorLogger.Printf(fmt, args...)
	}
}

func (l *Log) Info(args ...interface{}) {
	if l.InfoLogger != nil {
		l.InfoLogger.Print(args...)
	}
}

func (l *Log) Infof(fmt string, args ...interface{}) {
	if l.InfoLogger != nil {
		l.InfoLogger.Printf(fmt, args...)
	}
}

func (l *Log) Debug(args ...interface{}) {
	if l.DebugLogger != nil {
		l.DebugLogger.Print(args...)
	}
}

func (l *Log) Debugf(fmt string, args ...interface{}) {
	if l.DebugLogger != nil {
		l.DebugLogger.Printf(fmt, args...)
	}
}
