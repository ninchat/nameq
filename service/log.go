package service

import (
	"io"
	"log"
	"log/syslog"
	"os"
)

// Logger is a subset of the standard log.Logger.
type Logger interface {
	Print(v ...interface{})
	Printf(format string, v ...interface{})
}

// Log configures optional loggers.
type Log struct {
	ErrorLogger Logger
	InfoLogger  Logger
	DebugLogger Logger
}

// DefaultInit configures at least the error and info loggers, targetting
// stderr or syslog.
func (l *Log) DefaultInit(network, addr, tag string, debug bool) (err error) {
	var w io.Writer = os.Stderr

	if addr != "" {
		if w, err = syslog.Dial(network, addr, syslog.LOG_ERR|syslog.LOG_DAEMON, tag); err != nil {
			return
		}
	}

	l.ErrorLogger = log.New(w, "ERROR: ", 0)
	l.InfoLogger = log.New(w, "INFO: ", 0)

	if debug {
		l.DebugLogger = log.New(w, "DEBUG: ", 0)
	}

	return
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
