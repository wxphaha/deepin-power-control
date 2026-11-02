// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package global

import (
	"fmt"
	"io"
	"log"
	"log/syslog"
	"os"
	"path/filepath"
)

// LogLevel defines the severity of the log
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

var (
	// CurrentLogLevel can be changed at runtime via -d flag
	CurrentLogLevel = LevelInfo
	// Logger for system logs (will go to journal)
	internalLogger *log.Logger
)

func InitLog(verbose bool, logToFile bool) {
	var (
		writers []io.Writer
		err     error
		f       *os.File
		flags   int = 0
	)

	if logToFile {
		dir := filepath.Dir(DebugLogPath)
		_ = os.MkdirAll(dir, 0750)

		f, err = os.OpenFile(DebugLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
		}
	}

	if f == nil {
		writer, err := syslog.New(syslog.LOG_INFO|syslog.LOG_USER, "deepin-power-control")
		if err == nil {
			writers = append(writers, writer)
		} else {
			fmt.Fprintf(os.Stderr, "Failed to open syslog: %v\n", err)
		}
	} else {
		writers = append(writers, f)
		flags = log.LstdFlags
	}
	if verbose || len(writers) == 0 {
		writers = append(writers, os.Stderr)
	}
	internalLogger = log.New(io.MultiWriter(writers...), "", flags)
}

// SetLogLevel sets the log level to cur
func SetLogLevel(cur LogLevel) {
	CurrentLogLevel = cur
}

// logMessage is an internal helper to format and print logs
func logMessage(level LogLevel, prefix string, format string, v ...interface{}) {
	if level < CurrentLogLevel {
		return
	}

	if internalLogger == nil {
		//fmt.Fprintf(os.Stderr, "[deepin-power-control-fallback] [%s]: "+format+"\n", append([]interface{}{prefix}, v...)...)
		return
	}

	message := fmt.Sprintf(format, v...)
	// Format: [deepin-power-control] INFO: This is a message
	internalLogger.Printf("[%s]: %s", prefix, message)
}

// Debug logs high-volume, detailed information for developers
func Debug(format string, v ...interface{}) {
	logMessage(LevelDebug, "DEBUG", format, v...)
}

// Info logs general operational messages (e.g., service start/stop)
func Info(format string, v ...interface{}) {
	logMessage(LevelInfo, "INFO", format, v...)
}

// Warn logs non-critical issues that don't stop the program
func Warn(format string, v ...interface{}) {
	logMessage(LevelWarn, "WARN", format, v...)
}

// Error logs critical issues that require immediate attention
func Error(format string, v ...interface{}) {
	logMessage(LevelError, "ERROR", format, v...)
}
