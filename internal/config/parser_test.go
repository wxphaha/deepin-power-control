package config

import (
	"os"
	"path/filepath"
	"testing"

	"deepin-power-control/internal/global"
)

// TestShouldApplyFileWithPciTag verifies the dynamic @pci: feature tag handling.
func TestShouldApplyFileWithPciTag(t *testing.T) {
	tmpDir := t.TempDir()

	// Mock PCI devices path
	oldPath := pathPciDevices
	defer func() { pathPciDevices = oldPath }()
	pathPciDevices = tmpDir

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

	// Mock static feature checks for the non-PCI tests
	oldChecks := FeatureChecks
	defer func() { FeatureChecks = oldChecks }()
	FeatureChecks = map[string]func() bool{
		"supported": func() bool { return true },
		"ignored":   func() bool { return false },
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

	// 2. Mock Hardware Feature Registry
	oldChecks := FeatureChecks
	defer func() { FeatureChecks = oldChecks }()
	FeatureChecks = map[string]func() bool{
		"supported": func() bool { return true },
		"ignored":   func() bool { return false },
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
