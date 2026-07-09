package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckPciDevice(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore original path
	oldPath := pathPciDevices
	defer func() { pathPciDevices = oldPath }()
	pathPciDevices = tmpDir

	// Helper to create a mock PCI device directory
	createMockDevice := func(dirName, vendor, device string) {
		devPath := filepath.Join(tmpDir, dirName)
		os.MkdirAll(devPath, 0755)
		os.WriteFile(filepath.Join(devPath, "vendor"), []byte(vendor+"\n"), 0644)
		os.WriteFile(filepath.Join(devPath, "device"), []byte(device+"\n"), 0644)
	}

	tests := []struct {
		name     string
		setup    func()
		pciID    string
		expected bool
	}{
		{
			name: "Device Found",
			setup: func() {
				createMockDevice("0000:01:00.0", "0x1002", "0x699f")
			},
			pciID:    "1002:699f",
			expected: true,
		},
		{
			name: "Device Found - case insensitive",
			setup: func() {
				createMockDevice("0000:01:00.0", "0x1002", "0x699F")
			},
			pciID:    "1002:699f",
			expected: true,
		},
		{
			name: "Device Not Found",
			setup: func() {
				createMockDevice("0000:01:00.0", "0x1002", "0x699f")
			},
			pciID:    "1002:1234",
			expected: false,
		},
		{
			name:     "Empty PCI ID",
			setup:    func() {},
			pciID:    "",
			expected: false,
		},
		{
			name:     "Invalid PCI ID format",
			setup:    func() {},
			pciID:    "1002699f",
			expected: false,
		},
		{
			name: "Multiple Devices - match second",
			setup: func() {
				createMockDevice("0000:00:02.0", "0x8086", "0x1234")
				createMockDevice("0000:01:00.0", "0x1002", "0x699f")
			},
			pciID:    "1002:699f",
			expected: true,
		},
		{
			name:     "No devices directory",
			setup:    func() { pathPciDevices = "/non/existent/path" },
			pciID:    "1002:699f",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset to tmpDir before each test (except "No devices directory")
			pathPciDevices = tmpDir
			tt.setup()
			if got := CheckPciDevice(tt.pciID); got != tt.expected {
				t.Errorf("CheckPciDevice(%q) = %v, want %v", tt.pciID, got, tt.expected)
			}
		})
	}
}

func TestCheckEppSupport(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Store original paths to restore them after the test
	oldEpp := pathEppAvailable
	oldIntel := pathIntelPstate
	oldCpuInfo := pathCpuInfo
	oldAmd := pathAmdPstate
	oldAmdStatus := pathAmdStatus

	defer func() {
		pathEppAvailable = oldEpp
		pathIntelPstate = oldIntel
		pathCpuInfo = oldCpuInfo
		pathAmdPstate = oldAmd
		pathAmdStatus = oldAmdStatus
	}()

	// 2. Define test cases
	tests := []struct {
		name     string
		setup    func()
		expected bool
	}{
		{
			name: "Detect via Direct Sysfs",
			setup: func() {
				path := filepath.Join(tmpDir, "epp_pref")
				os.MkdirAll(filepath.Dir(path), 0755)
				os.Create(path)
				pathEppAvailable = path // Mock the path
			},
			expected: true,
		},
		{
			name: "Detect via AMD P-State",
			setup: func() {
				pathEppAvailable = "/non/existent"

				amdDir := filepath.Join(tmpDir, "amd_pstate")
				os.MkdirAll(amdDir, 0755)
				pathAmdPstate = amdDir

				statusFile := filepath.Join(amdDir, "status")
				os.WriteFile(statusFile, []byte("active\n"), 0644)
				pathAmdStatus = statusFile
			},
			expected: true,
		},
		{
			name: "No Support Found",
			setup: func() {
				pathEppAvailable = "/non/existent"
				pathIntelPstate = "/non/existent"
				pathAmdPstate = "/non/existent"
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			if got := CheckEppSupport(); got != tt.expected {
				t.Errorf("CheckEppSupport() = %v, want %v", got, tt.expected)
			}
		})
	}
}
