// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package global

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// TestLogLevelFiltering verifies that logs below the current level are suppressed
func TestLogLevelFiltering(t *testing.T) {
	// 1. Redirect log output to a buffer for inspection
	var buf bytes.Buffer
	internalLogger = log.New(&buf, "", 0)

	tests := []struct {
		name      string
		setLevel  LogLevel
		logFunc   func(string, ...interface{})
		message   string
		shouldLog bool
	}{
		{"Debug suppressed by Info", LevelInfo, Debug, "debug msg", false},
		{"Info allowed by Info", LevelInfo, Info, "info msg", true},
		{"Info suppressed by Warn", LevelWarn, Info, "info msg", false},
		{"Error allowed by Warn", LevelWarn, Error, "error msg", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			SetLogLevel(tt.setLevel)

			tt.logFunc(tt.message)

			hasLog := buf.Len() > 0
			if hasLog != tt.shouldLog {
				t.Errorf("Level %v: log status was %v, want %v", tt.setLevel, hasLog, tt.shouldLog)
			}

			if tt.shouldLog && !strings.Contains(buf.String(), tt.message) {
				t.Errorf("Expected log to contain %q, but got %q", tt.message, buf.String())
			}
		})
	}
}

// TestLogFormat verifies the [PREFIX]: Message format
func TestLogFormat(t *testing.T) {
	var buf bytes.Buffer
	internalLogger = log.New(&buf, "", 0)
	SetLogLevel(LevelDebug)

	Info("test connection %d", 80)

	expected := "[INFO]: test connection 80"
	// sysLogger.Printf adds a newline at the end
	actual := strings.TrimSpace(buf.String())

	if actual != expected {
		t.Errorf("Format mismatch: got %q, want %q", actual, expected)
	}
}

// TestLoggerNilSafety ensures that logging doesn't panic even if sysLogger is poorly initialized
// In your code, sysLogger is private, so we test the public InitLog fallback.
func TestInitLogFallback(t *testing.T) {
	// We call InitLog to ensure it doesn't crash even if syslog is unavailable
	// On many dev machines/containers, syslog might not be running
	InitLog(true, false)

	if internalLogger == nil {
		t.Fatal("sysLogger should not be nil after InitLog")
	}

	// Test a log call to ensure no panic
	Info("Safe log")
}
