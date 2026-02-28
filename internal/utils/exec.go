package utils

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// NoTimeout is a helper constant to clarify intent for infinite wait
const NoTimeout time.Duration = 0

// CmdError is our custom wrapper (The "Big Envelope")
// We renamed this from ExitError to avoid confusion with exec.ExitError
type CmdError struct {
	Command string
	Cause   *exec.ExitError // The standard library's error (The "Letter")
}

// Error implements the error interface
func (e *CmdError) Error() string {
	if len(e.Cause.Stderr) > 0 {
		return fmt.Sprintf("command { %s } failed: %s, stderr: %s",
			e.Command, e.Cause.Error(), string(e.Cause.Stderr))
	}
	return fmt.Sprintf("command { %s } failed: %s", e.Command, e.Cause.Error())
}

// Unwrap allows standard errors.Is / errors.As to work with the underlying error
func (e *CmdError) Unwrap() error {
	return e.Cause
}

// RunCmd is the default runner with 60s timeout
func RunCmd(name string, arg ...string) error {
	return RunCmdWithTimeout(60*time.Second, name, arg...)
}

// RunCmdWithTimeout executes a command with a specific timeout.
// If timeout <= 0, it waits forever.
func RunCmdWithTimeout(timeout time.Duration, name string, arg ...string) error {
	var ctx context.Context
	var cancel context.CancelFunc

	if timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	cmd := exec.CommandContext(ctx, name, arg...)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err != nil {
		// Handle Context errors (Timeout or Cancel)
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("command { %s %v } timed out after %v", name, arg, timeout)
		}

		// Handle Command Execution Failure (Exit status != 0)
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Fill the standard library struct with captured stderr
			exitErr.Stderr = errBuf.Bytes()
			// Return our clearly named wrapper
			return &CmdError{
				Command: cmd.String(),
				Cause:   exitErr,
			}
		}
		return err
	}
	return nil
}

// RunSudoS executes a command with sudo -S, feeding the password via stdin.
// This avoids using bash -c and handles commands safely.
func RunSudoS(password string, name string, arg ...string) error {
	// Prepend "sudo", "-S" to the arguments
	args := append([]string{"-S", name}, arg...)
	cmd := exec.Command("sudo", args...)

	// Feed password to stdin
	cmd.Stdin = bytes.NewBufferString(password + "\n")

	// You can capture stderr here if you need to debug sudo failures
	return cmd.Run()
}
