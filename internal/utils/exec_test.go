// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package utils

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestRunCmdWithTimeout(t *testing.T) {
	tests := []struct {
		name          string
		timeout       time.Duration
		cmd           string
		args          []string
		expectSuccess bool
		expectTimeout bool
		expectStderr  string // substring to check
	}{
		{
			name:          "success",
			timeout:       5 * time.Second,
			cmd:           "echo",
			args:          []string{"hello"},
			expectSuccess: true,
		},
		{
			name:          "failure_with_stderr",
			timeout:       5 * time.Second,
			cmd:           "sh",
			args:          []string{"-c", "echo 'error message' >&2; exit 1"},
			expectSuccess: false,
			expectStderr:  "error message",
		},
		{
			name:          "failure_no_stderr",
			timeout:       5 * time.Second,
			cmd:           "false",
			args:          nil,
			expectSuccess: false,
		},
		{
			name:          "timeout",
			timeout:       100 * time.Millisecond,
			cmd:           "sleep",
			args:          []string{"1"},
			expectSuccess: false,
			expectTimeout: true,
		},
		{
			name:          "no_timeout_success",
			timeout:       NoTimeout, // 0 means wait forever
			cmd:           "echo",
			args:          []string{"ok"},
			expectSuccess: true,
		},
		{
			name:          "no_timeout_failure",
			timeout:       NoTimeout,
			cmd:           "sh",
			args:          []string{"-c", "exit 5"},
			expectSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RunCmdWithTimeout(tt.timeout, tt.cmd, tt.args...)

			if tt.expectSuccess {
				if err != nil {
					t.Errorf("expected success, got error: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			// Check timeout error
			if tt.expectTimeout {
				if !isTimeoutError(err) {
					t.Errorf("expected timeout error, got: %v", err)
				}
				return
			}

			// Check CmdError wrapper
			var cmdErr *CmdError
			if !errors.As(err, &cmdErr) {
				t.Fatalf("error should be *CmdError, got: %T", err)
			}

			// Verify command string contains the command
			if cmdErr.Command == "" {
				t.Error("CmdError.Command should not be empty")
			}

			// Check stderr content if expected
			if tt.expectStderr != "" {
				if !bytes.Contains(cmdErr.Cause.Stderr, []byte(tt.expectStderr)) {
					t.Errorf("expected stderr to contain %q, got %q", tt.expectStderr, string(cmdErr.Cause.Stderr))
				}
			}

			// Verify underlying exec.ExitError
			var exitErr *exec.ExitError
			if !errors.As(cmdErr, &exitErr) {
				t.Error("CmdError should wrap *exec.ExitError")
			}
		})
	}
}

// Helper: check if error is a timeout
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return msg == "context deadline exceeded" || strings.Contains(msg, "timed out")
}

// Helper: substring check
func contains(b []byte, substr string) bool {
	return len(b) > 0 && len(substr) > 0 &&
		bytes.Index(b, []byte(substr)) >= 0
}
