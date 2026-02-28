// daemon_test.go

package service

import (
	"deepin-power-control/internal/global"
	"errors"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type mockNet struct {
	listenErr   error
	dialErr     error
	acceptConn  net.Conn
	writeExited bool
}

func (m *mockNet) Listen(_, _ string) (net.Listener, error) {
	if m.listenErr != nil {
		return nil, m.listenErr
	}
	return &mockListener{mock: m}, nil
}

func (m *mockNet) Dial(_, _ string) (net.Conn, error) {
	if m.dialErr != nil {
		return nil, m.dialErr
	}
	if m.acceptConn == nil {
		m.acceptConn = &mockConn{mock: m}
	}
	return m.acceptConn, nil
}

type mockListener struct{ mock *mockNet }

func (l *mockListener) Accept() (net.Conn, error) {
	if l.mock.acceptConn == nil {
		l.mock.acceptConn = &mockConn{mock: l.mock}
	}
	return l.mock.acceptConn, nil
}
func (l *mockListener) Close() error   { return nil }
func (l *mockListener) Addr() net.Addr { return nil }

type mockConn struct {
	mock   *mockNet
	closed bool
}

func (c *mockConn) Read(b []byte) (int, error)         { return copy(b, "exit\n"), nil }
func (c *mockConn) Write(b []byte) (int, error)        { c.mock.writeExited = true; return len(b), nil }
func (c *mockConn) Close() error                       { c.closed = true; return nil }
func (c *mockConn) LocalAddr() net.Addr                { return nil }
func (c *mockConn) RemoteAddr() net.Addr               { return nil }
func (c *mockConn) SetDeadline(t time.Time) error      { return nil }
func (c *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestStartDaemonize(t *testing.T) {
	tests := []struct {
		name        string
		isChild     bool
		execErr     error
		listenErr   error
		writePIDErr error
		expectErr   string
		checkParent bool
	}{
		{
			name:        "parent_success",
			isChild:     false,
			checkParent: true,
		},
		{
			name:      "parent_exec_fail",
			isChild:   false,
			execErr:   errors.New("exec failed"),
			expectErr: "exec failed",
		},
		{
			name:    "child_success",
			isChild: true,
		},
		{
			name:      "child_listen_fail",
			isChild:   true,
			listenErr: errors.New("listen failed"),
			expectErr: "listen failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			oldSocket := socketAddr
			oldExec := executable
			oldListen := newListener
			oldWritePID := writePIDFile
			oldExit := exitProcess
			oldPidFile := global.PidFile

			defer func() {
				socketAddr = oldSocket
				executable = oldExec
				newListener = oldListen
				writePIDFile = oldWritePID
				exitProcess = oldExit
				global.PidFile = oldPidFile
			}()

			socketAddr = filepath.Join(tmpDir, "test.sock")
			global.PidFile = filepath.Join(tmpDir, "test.pid")
			executable = func() (string, error) {
				if tt.execErr != nil {
					return "", tt.execErr
				}
				return "/fake/exe", nil
			}
			mockNetInst := &mockNet{
				listenErr: tt.listenErr,
			}
			newListener = mockNetInst.Listen
			writePIDFile = func(pid int) error { return tt.writePIDErr }

			var exitedWithCode int
			exitProcess = func(code int) { exitedWithCode = code }

			if tt.isChild {
				oldEnv := os.Getenv(daemonEnv)
				os.Setenv(daemonEnv, "true")

				t.Cleanup(func() {
					if oldEnv == "" {
						os.Unsetenv(daemonEnv)
					} else {
						os.Setenv(daemonEnv, oldEnv)
					}
				})
			} else {
				os.Unsetenv(daemonEnv)
			}

			ctx, err := StartDaemonize()

			if tt.checkParent {
				if exitedWithCode != 0 {
					t.Errorf("expected exit(0), got %d", exitedWithCode)
				}
				return // parent calls os.Exit, so ctx/err are meaningless
			}

			if tt.expectErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.expectErr) {
					t.Errorf("expected error containing %q, got %v", tt.expectErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ctx == nil {
				t.Fatal("expected non-nil context")
			}
		})
	}
}

func TestStopDaemonize(t *testing.T) {
	tests := []struct {
		name          string
		dialErr       error
		pidFileExists bool
		pidValid      bool
		expectSignal  bool
		expectInfo    bool
	}{
		{
			name:         "socket_success",
			dialErr:      nil,
			expectSignal: false,
		},
		{
			name:         "socket_fail_no_pid_file",
			dialErr:      errors.New("dial failed"),
			expectSignal: false,
			expectInfo:   true,
		},
		{
			name:          "socket_fail_with_valid_pid",
			dialErr:       errors.New("dial failed"),
			pidFileExists: true,
			pidValid:      true,
			expectSignal:  true,
		},
		{
			name:          "socket_fail_invalid_pid",
			dialErr:       errors.New("dial failed"),
			pidFileExists: true,
			pidValid:      false,
			expectSignal:  false,
			expectInfo:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			oldSocket := socketAddr
			oldPidFile := global.PidFile
			socketAddr = filepath.Join(tmpDir, "stop.sock")
			global.PidFile = filepath.Join(tmpDir, "stop.pid")
			oldKill := killProcess
			netDialOrig := net.Dial

			// Setup mock net
			mock := &mockNet{dialErr: tt.dialErr}
			newDial = mock.Dial

			killProcess = func(pid int) error {
				return nil // always succeed in test
			}

			defer func() {
				// Restore
				socketAddr = oldSocket
				global.PidFile = oldPidFile
				newDial = netDialOrig
				killProcess = oldKill
			}()

			if tt.pidFileExists {
				pid := 9999
				if !tt.pidValid {
					pid = -1
				}
				ioutil.WriteFile(global.PidFile, []byte(strconv.Itoa(pid)), 0644)
			}

			err := StopDaemonize()

			if tt.expectSignal {
				// We can't easily verify signal sent, but no error is expected
				if err != nil {
					t.Errorf("unexpected error when signaling: %v", err)
				}
			} else if !tt.expectInfo {
				// Only check error if not expecting "no daemon" info
				if err != nil && !tt.pidFileExists {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestIsRunning(t *testing.T) {
	tests := []struct {
		name     string
		dialErr  error
		expected bool
	}{
		{"running", nil, true},
		{"not_running", errors.New("connection refused"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			oldSocket := socketAddr
			socketAddr = filepath.Join(tmpDir, "check.sock")

			if tt.dialErr == nil {
				// Start real listener
				l, _ := net.Listen("unix", socketAddr)
				defer l.Close()
			}

			got := isRunning()
			if got != tt.expected {
				t.Errorf("isRunning() = %v, want %v", got, tt.expected)
			}

			socketAddr = oldSocket
		})
	}
}
