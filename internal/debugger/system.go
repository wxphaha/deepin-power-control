// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package debugger

import (
	"fmt"
	"os"
	"path/filepath"

	"deepin-power-control/internal/global"
	"deepin-power-control/internal/utils"
)

// PrepareSystemForReboot creates and enables a systemd service for the debugger.
// Since it's called by root during the first 'debug' run, we don't need a password.
func PrepareSystemForReboot() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get excutable path: %v", err)
	}
	if realPath, err := filepath.EvalSymlinks(execPath); err != nil {
		execPath = realPath
	}

	// Create systemd service content
	// Type=oneshot: The service is considered up after the main process exits.
	// We use resume to let the engine know it's a resume.
	content := fmt.Sprintf(`[Unit]
Description=DSPC Debugger Reboot Agent
After=graphical.target

[Service]
Type=oneshot
ExecStart=%s debug resume
RemainAfterExit=yes

[Install]
WantedBy=graphical.target
`, execPath)

	// 1. Write the service file
	if err := os.WriteFile(global.SystemdServicePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to create systemd service: %w", err)
	}

	// 2. Enable the service: systemctl enable deepin-power-control-debug
	if err := utils.RunCmd("systemctl", "enable", "deepin-power-control-debug.service"); err != nil {
		return fmt.Errorf("failed to enable systemd service: %w", err)
	}

	return nil
}

// RestoreSystem cleans up the systemd service and other debug artifacts.
func RestoreSystem() {
	// 1. Disable and remove systemd service
	_ = utils.RunCmd("systemctl", "disable", "deepin-power-control-debug.service")
	if _, err := os.Stat(global.SystemdServicePath); err == nil {
		_ = os.Remove(global.SystemdServicePath)
	}

	// 2. Clear state and temp configs
	_ = ClearState()
	// tempConfig removal is handled by Engine.Restore()
}
