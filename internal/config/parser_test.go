// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deepin-power-control/internal/features"
	"deepin-power-control/internal/global"
)

// TestShouldApplyFileWithPciTag verifies the dynamic @pci: feature tag handling.
func TestShouldApplyFileWithPciTag(t *testing.T) {
	tmpDir := t.TempDir()

	// Mock PCI devices path
	oldPath := features.PathPciDevices
	defer func() { features.PathPciDevices = oldPath }()
	features.PathPciDevices = tmpDir
	features.ResetPciCache()

	// Create a mock PCI device
	devPath := filepath.Join(tmpDir, "0000:01:00.0")
	os.MkdirAll(devPath, 0755)
	os.WriteFile(filepath.Join(devPath, "vendor"), []byte("0x1002\n"), 0644)
	os.WriteFile(filepath.Join(devPath, "device"), []byte("0x699f\n"), 0644)

	tests := []struct {
		name     string
		fileName string
		expected bool
	}{
		{
			name:     "Matching PCI device",
			fileName: "15-amd-gpu-dpm@pci:1002:699f.conf",
			expected: true,
		},
		{
			name:     "Non-matching PCI device",
			fileName: "15-nvidia@pci:10de:1234.conf",
			expected: false,
		},
		{
			name:     "No feature tag",
			fileName: "00-base.conf",
			expected: true,
		},
		{
			name:     "Static feature tag - supported",
			fileName: "20-epp@supported.conf",
			expected: true,
		},
		{
			name:     "Static feature tag - ignored",
			fileName: "20-epp@ignored.conf",
			expected: false,
		},
	}

	// Mock the feature registries for the non-PCI tests
	oldChecks := features.FeatureChecks
	defer func() { features.FeatureChecks = oldChecks }()
	features.FeatureChecks = map[string]func(param string) bool{
		"supported": func(string) bool { return true },
		"ignored":   func(string) bool { return false },
		"pci":       features.CheckPciDevice,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldApplyFile(tt.fileName)
			if got != tt.expected {
				t.Errorf("shouldApplyFile(%q) = %v, want %v", tt.fileName, got, tt.expected)
			}
		})
	}
}

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
		{"30-cpu@epp.conf", ""}, // Feature tags shouldn't interfere with mode extraction
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
// TestApplyModeWithTestdata verifies the merge logic using external configuration files.
// It simulates a real-world scenario by loading inputs from testdata/inputs/
// and comparing the result with testdata/performance.golden.
func TestApplyModeWithTestdata(t *testing.T) {
	// 1. Setup temporary environment
	tmpRawDir := "./debug_raw"
	tmpTlpDir := "./debug_tlp"
	os.MkdirAll(tmpRawDir, 0755)
	os.MkdirAll(tmpTlpDir, 0755)
	defer os.RemoveAll(tmpRawDir)
	defer os.RemoveAll(tmpTlpDir)

	global.RawConfigDir = tmpRawDir
	global.TlpConfigDir = tmpTlpDir
	global.TargetTlpFilePrefix = "dspc_test"

	// Isolate rules: point at an empty dir so installed rule files on the
	// test machine (e.g. /usr/share/deepin-power-control/rules) cannot
	// leak fragments into the golden comparison.
	oldRulesDir := global.RulesConfigDir
	defer func() { global.RulesConfigDir = oldRulesDir }()
	emptyRules := filepath.Join(tmpRawDir, "empty-rules")
	os.MkdirAll(emptyRules, 0755)
	global.RulesConfigDir = emptyRules

	// 2. Mock the feature registries (declared features + code fallback)
	oldChecks := features.FeatureChecks
	defer func() { features.FeatureChecks = oldChecks }()
	features.FeatureChecks = map[string]func(param string) bool{
		"supported": func(string) bool { return true },
		"ignored":   func(string) bool { return false },
		"pci":       features.CheckPciDevice,
	}

	// 3. Copy input files from testdata/inputs to the temporary RawConfigDir
	inputSrcDir := filepath.Join("testdata", "inputs")
	inputFiles, err := os.ReadDir(inputSrcDir)
	if err != nil {
		t.Fatalf("Failed to read testdata/inputs: %v", err)
	}

	for _, file := range inputFiles {
		srcPath := filepath.Join(inputSrcDir, file.Name())
		destPath := filepath.Join(tmpRawDir, file.Name())

		content, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("Failed to read input file %s: %v", srcPath, err)
		}
		if err := os.WriteFile(destPath, content, 0644); err != nil {
			t.Fatalf("Failed to copy input file to tmp dir: %v", err)
		}
	}

	// 4. Execute the merge logic for 'performance' mode
	mode := "performance"
	if err := ApplyMode(mode); err != nil {
		t.Fatalf("ApplyMode failed: %v", err)
	}

	// 5. Golden File Comparison
	generatedFile := filepath.Join(tmpTlpDir, "dspc_test_performance.conf")
	got, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatalf("Failed to read generated file: %v", err)
	}

	goldenPath := filepath.Join("testdata", "performance.golden")

	// Support automatic updates: UPDATE_GOLDEN=true go test -v ./internal/config
	if os.Getenv("UPDATE_GOLDEN") == "true" {
		if err := os.WriteFile(goldenPath, got, 0644); err != nil {
			t.Fatal(err)
		}
		t.Logf("Updated golden file: %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("Golden file missing. Run with UPDATE_GOLDEN=true to create it: %v", err)
	}

	if string(got) != string(want) {
		t.Errorf("Full output mismatch!\nCheck the difference between %s and %s", generatedFile, goldenPath)
	}
}

// TestApplyModeWithRuleConfigFragment verifies that config fragments
// generated by rules are merged after filesystem configs (rules win) and
// that the write/exec actions still run afterwards.
func TestApplyModeWithRuleConfigFragment(t *testing.T) {
	// 1. Setup temporary environment
	tmpRawDir := "./debug_raw"
	tmpTlpDir := "./debug_tlp"
	os.MkdirAll(tmpRawDir, 0755)
	os.MkdirAll(tmpTlpDir, 0755)
	defer os.RemoveAll(tmpRawDir)
	defer os.RemoveAll(tmpTlpDir)

	oldRaw, oldTlp, oldPrefix := global.RawConfigDir, global.TlpConfigDir, global.TargetTlpFilePrefix
	oldRulesDir := global.RulesConfigDir
	defer func() {
		global.RawConfigDir, global.TlpConfigDir, global.TargetTlpFilePrefix = oldRaw, oldTlp, oldPrefix
		global.RulesConfigDir = oldRulesDir
	}()
	global.RawConfigDir = tmpRawDir
	global.TlpConfigDir = tmpTlpDir
	global.TargetTlpFilePrefix = "dspc_test"

	// 2. Mock feature registries (empty code fallback)
	oldChecks := features.FeatureChecks
	defer func() { features.FeatureChecks = oldChecks }()
	features.FeatureChecks = map[string]func(param string) bool{}

	// 3. Base config file
	base := filepath.Join(tmpRawDir, "00-base.conf")
	if err := os.WriteFile(base, []byte("CPU_SCALING_GOVERNOR_ON_AC=ondemand\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Rules directory with a balance-mode config fragment
	ruleDir := filepath.Join(tmpRawDir, "rules")
	oemDir := filepath.Join(ruleDir, "oem")
	os.MkdirAll(oemDir, 0755)
	global.RulesConfigDir = ruleDir
	rule := filepath.Join(oemDir, "10-test.yaml")
	ruleContent := `
- match:
    mode: balance
  config:
    CPU_SCALING_GOVERNOR_ON_AC: powersave
    WOL_DISABLE: Y
`
	if err := os.WriteFile(rule, []byte(ruleContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 5. Apply balance mode
	if err := ApplyMode("balance"); err != nil {
		t.Fatalf("ApplyMode failed: %v", err)
	}

	// 6. The generated conf must contain the rule fragment values,
	// overriding the base config value.
	generated := filepath.Join(tmpTlpDir, "dspc_test_balance.conf")
	data, err := os.ReadFile(generated)
	if err != nil {
		t.Fatalf("Failed to read generated file: %v", err)
	}
	got := string(data)
	for _, want := range []string{"CPU_SCALING_GOVERNOR_ON_AC=powersave", "WOL_DISABLE=Y"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in generated config, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "ondemand") {
		t.Fatalf("base value should be overridden by rule fragment, got:\n%s", got)
	}
	if !strings.Contains(got, "rule: 10-test.yaml") {
		t.Fatalf("expected rule source annotation in generated config, got:\n%s", got)
	}

	// 7. Performance mode: rule must not match, base value stays
	if err := ApplyMode("performance"); err != nil {
		t.Fatalf("ApplyMode failed: %v", err)
	}
	perf := filepath.Join(tmpTlpDir, "dspc_test_performance.conf")
	data, err = os.ReadFile(perf)
	if err != nil {
		t.Fatalf("Failed to read generated file: %v", err)
	}
	if !strings.Contains(string(data), "CPU_SCALING_GOVERNOR_ON_AC=ondemand") {
		t.Fatalf("performance mode must not include rule fragment, got:\n%s", string(data))
	}
	if strings.Contains(string(data), "WOL_DISABLE") {
		t.Fatalf("performance mode must not include rule fragment, got:\n%s", string(data))
	}
}

func TestIsValidMode(t *testing.T) {
	for _, mode := range ValidModes {
		if !IsValidMode(mode) {
			t.Fatalf("expected %q to be valid", mode)
		}
	}
	for _, mode := range []string{"foo", "", "BALANCE", "performance "} {
		if IsValidMode(mode) {
			t.Fatalf("expected %q to be invalid", mode)
		}
	}
}
