package config

import (
	"bufio"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deepin-power-control/internal/global"
)

// TestGetModeFromFilename checks the filename-based mode extraction
func TestGetModeFromFilename(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"00-base.conf", ""},
		{"10_performance_logic.conf", "performance"},
		{"saving_mode.conf", ""},
		{"xx-saving_mode.conf", "saving"},
		{"lowbat.conf", "lowbat"},
		{"xx_balance_config.conf", "balance"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getModeFromFilename(tt.name)
			if got != tt.expected {
				t.Errorf("getModeFromFilename(%s) = %s; want %s", tt.name, got, tt.expected)
			}
		})
	}
}

// TestApplyModeLogic simulates the full merge process including section-based
// and filename-based overriding.
func TestApplyModeLogic(t *testing.T) {
	// Setup temporary environment
	tmpRawDir := t.TempDir()
	tmpTlpDir := t.TempDir()
	targetFile := filepath.Join(tmpTlpDir, "30_dspc_test_performance.conf")

	// Inject test paths into global variables
	global.RawConfigDir = tmpRawDir
	global.TlpConfigDir = tmpTlpDir
	global.TargetTlpFilePrefix = "30_dspc_test"

	// 1. Base config (Global)
	base := "WIFI_PWR=on\nCPU_SCALING=powersave"
	ioutil.WriteFile(filepath.Join(tmpRawDir, "00-base.conf"), []byte(base), 0644)

	// 2. Mode config (Section-based)
	mode := `
CPU_SCALING=balance
[performance]
WIFI_PWR=off
CPU_SCALING=performance
[saving]
WIFI_PWR=on
`
	ioutil.WriteFile(filepath.Join(tmpRawDir, "10-mode.conf"), []byte(mode), 0644)

	// 3. Override config (Filename-based for performance)
	override := "CPU_MAX_PERF=80"
	ioutil.WriteFile(filepath.Join(tmpRawDir, "20_performance_custom.conf"), []byte(override), 0644)

	// Execute merge for 'performance' mode
	err := ApplyMode("performance")
	if err != nil {
		t.Fatalf("ApplyMode failed: %v", err)
	}

	// Verify the content of the generated file
	content, err := os.Open(targetFile)
	if err != nil {
		t.Fatalf("Could not open target file: %v", err)
	}
	defer content.Close()

	results := make(map[string]string)
	scanner := bufio.NewScanner(content)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "=") {
			kv := strings.Split(line, "=")
			results[kv[0]] = kv[1]
		}
	}

	// Assertions for 'performance' mode
	// WIFI_PWR should be 'off' from [performance] section in 10-mode.conf
	if results["WIFI_PWR"] != "off" {
		t.Errorf("WIFI_PWR mismatch: got %s, want off", results["WIFI_PWR"])
	}
	// CPU_SCALING should be 'performance' from [performance] section
	if results["CPU_SCALING"] != "performance" {
		t.Errorf("CPU_SCALING mismatch: got %s, want performance", results["CPU_SCALING"])
	}
	// CPU_MAX_PERF should be '80' from 20_performance_custom.conf
	if results["CPU_MAX_PERF"] != "80" {
		t.Errorf("CPU_MAX_PERF mismatch: got %s, want 80", results["CPU_MAX_PERF"])
	}
}
