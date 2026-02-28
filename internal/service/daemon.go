package service

import (
	"bufio"
	"context"
	"deepin-power-control/internal/global"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

const (
	daemonEnv = "INTERNAL_DAEMON_MODE"
)

// for test mock, can mock by test
var (
	socketAddr   = "@deepin-power-control.daemon"
	executable   = os.Executable
	newListener  = net.Listen
	writePIDFile = func(pid int) error {
		return ioutil.WriteFile(global.PidFile, []byte(strconv.Itoa(pid)), 0644)
	}
	exitProcess = os.Exit
	newDial     = net.Dial
	killProcess = defaultKillProcess
)

func defaultKillProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGTERM)
}

// StartDaemonize now returns a Context.
func StartDaemonize() (context.Context, error) {
	if isRunning() {
		return nil, fmt.Errorf("daemon is already running")
	}

	isChild := os.Getenv(daemonEnv) == "true"
	if !isChild {
		return startParent()
	}
	return startChild()
}

func startParent() (context.Context, error) {
	exe, err := executable()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), daemonEnv+"=true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout, cmd.Stderr, cmd.Stdin = nil, nil, nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start background process: %w", err)
	}

	global.Info("DSP Daemon started in background (PID: %d)\n", cmd.Process.Pid)
	exitProcess(0)
	return nil, nil // unreachable, but satisfies compiler
}

func startChild() (context.Context, error) {
	ctx, cancel := context.WithCancel(context.Background())

	l, err := newListener("unix", socketAddr)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to setup socket: %v", err)
	}

	if err := writePIDFile(os.Getpid()); err != nil {
		// Log but don't fail, Keep original behavior.
		global.Warn("Failed to write PID file: %v", err)
	}

	go monitorShutdown(l, cancel)
	return ctx, nil
}

func monitorShutdown(l net.Listener, cancel context.CancelFunc) {
	defer l.Close()
	defer os.Remove(global.PidFile)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	socketChan := make(chan bool)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				global.Error("Accept failed: %v", err)
				return
			}
			scanner := bufio.NewScanner(conn)
			if scanner.Scan() && scanner.Text() == "exit" {
				socketChan <- true
			}
			conn.Close()
		}
	}()

	select {
	case <-socketChan:
		global.Info("Daemon received 'exit' command via socket. Stopping...")
	case sig := <-sigChan:
		global.Warn("Daemon received signal [%v]. Terminated by system/user.", sig)
	}
}

func isRunning() bool {
	conn, err := newDial("unix", socketAddr)
	if err == nil {
		conn.Close()
		return true
	}
	return false
}

func StopDaemonize() error {
	if err := sendExitCommand(); err == nil {
		return nil
	}

	// Fallback to PID kill
	pidData, err := ioutil.ReadFile(global.PidFile)
	if err != nil {
		global.Info("no daemon process found")
		return nil
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil || pid <= 0 {
		global.Info("invalid PID file")
		return nil
	}

	if err := killProcess(pid); err != nil {
		global.Warn("failed to signal daemon: %v", err)
		return fmt.Errorf("failed to signal daemon: %w", err)
	}

	return nil
}

func sendExitCommand() error {
	conn, err := newDial("unix", socketAddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = fmt.Fprintln(conn, "exit")
	return err
}
