// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package global

import (
	"os"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

const (
	// App private configuration and runtime files
	AppConfigFile = "/etc/deepin-power-control/deepin-power-control.ini"

	// Absolute paths for system commands (Security)
	CmdTlp      = "/usr/sbin/tlp"
	CmdTlpDebug = "/usr/sbin/tlp.debug"
	CmdIw       = "/usr/sbin/iw"

	SystemdServicePath = "/etc/systemd/system/deepin-power-control-debug.service"
	StateFile          = "/var/lib/deepin-power-control/debug_state.json"
	DebugLogPath       = "/var/log/deepin-power-control-debug.log"
)

// for test, define those as var ,but it should be const
var (
	PidFile = "/run/deepin-power-control.pid"
	// Paths for TLP configuration management
	RawConfigDir = "/usr/share/deepin-power-control/configs"
	TlpConfigDir = "/etc/tlp.d"
	// Path for rule-based actions (sysfs writes / config generation / program calls)
	RulesConfigDir      = "/usr/share/deepin-power-control/rules"
	TargetTlpFilePrefix = "30_deepin-power-control"
)

// AppConfig holds runtime parameters for the monitor engine
var AppConfig struct {
	WifiThreshold     uint64   `yaml:"wifi_threshold"`      // Base threshold in KB/s
	WifiMultiplier    int      `yaml:"wifi_multiplier"`     // Threshold multiplier for Performance mode
	MinCheckInterval  int      `yaml:"min_check_interval"`  // In seconds
	MaxCheckInterval  int      `yaml:"max_check_interval"`  // In seconds
	WifiIgnoreDevices []string `yaml:"wifi_ignore_devices"` // List of interfaces
	LogLevel          LogLevel `yaml:"log_level"`
	RebootRetryCount  uint     `yaml:"reboot_retry_count"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
// This allows YAML strings like "debug" to be parsed directly into our LogLevel type.
func (l *LogLevel) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	switch strings.ToLower(s) {
	case "debug":
		*l = LevelDebug
	case "info":
		*l = LevelInfo
	case "warn":
		*l = LevelWarn
	case "error":
		*l = LevelError
	default:
		*l = LevelInfo
	}
	return nil
}

// String provides a human-readable representation of the log level
func (l LogLevel) String() string {
	return [...]string{"debug", "info", "warn", "error"}[l]
}

// InitDefaultConfig sets fallback values
func InitDefaultConfig() {
	AppConfig.WifiThreshold = 100
	AppConfig.WifiMultiplier = 3
	AppConfig.MinCheckInterval = 30
	AppConfig.MaxCheckInterval = 300
	AppConfig.WifiIgnoreDevices = []string{"lo", "docker0"}
	AppConfig.LogLevel = LevelInfo
	AppConfig.RebootRetryCount = 50
}

// LoadConfig tries to read config.yaml, falls back to defaults on any error
func LoadConfig(path string) {
	// 1. First, set defaults
	InitDefaultConfig()

	// 2. Try to read file
	data, err := os.ReadFile(path)
	if err != nil {
		// File missing, keep defaults
		return
	}

	// 3. Unmarshal directly into AppConfig.
	// Our custom UnmarshalYAML will handle the LogLevel conversion.
	if err := yaml.Unmarshal(data, &AppConfig); err != nil {
		// If YAML is malformed, we keep the defaults set by InitDefaultConfig
		return
	}

	// 4. Post-Load Validation (Ensuring intervals and multipliers make sense)
	if AppConfig.WifiMultiplier < 1 {
		AppConfig.WifiMultiplier = 3
	}
	// Ensure thresholds aren't zero to avoid logic deadlocks
	if AppConfig.WifiThreshold == 0 {
		AppConfig.WifiThreshold = 100
	}
	if AppConfig.MinCheckInterval <= 0 {
		AppConfig.MinCheckInterval = 30
	}
	if AppConfig.MaxCheckInterval < AppConfig.MinCheckInterval {
		AppConfig.MaxCheckInterval = AppConfig.MinCheckInterval
	}
	if AppConfig.RebootRetryCount > 1000 {
		AppConfig.RebootRetryCount = 50
	}
}

// ParseLogLevel converts string to LogLevel type
func ParseLogLevel(s string) LogLevel {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo // Fallback to info
	}
}
