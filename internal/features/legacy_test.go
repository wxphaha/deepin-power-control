package features

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckPciDevice(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore original path
	oldPath := PathPciDevices
	defer func() { PathPciDevices = oldPath }()
	PathPciDevices = tmpDir

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
			setup:    func() { PathPciDevices = "/non/existent/path" },
			pciID:    "1002:699f",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset to tmpDir before each test (except "No devices directory")
			PathPciDevices = tmpDir
			tt.setup()
			ResetPciCache()
			if got := CheckPciDevice(tt.pciID); got != tt.expected {
				t.Errorf("CheckPciDevice(%q) = %v, want %v", tt.pciID, got, tt.expected)
			}
		})
	}
}

// TestCheckPciDeviceOxPrefix verifies IDs written from sysfs values
// (with "0x" prefix) match the scan cache (stored without prefix).
func TestCheckPciDeviceOxPrefix(t *testing.T) {
	oldPath := PathPciDevices
	defer func() { PathPciDevices = oldPath }()
	dir := t.TempDir()
	PathPciDevices = dir
	ResetPciCache()

	devPath := filepath.Join(dir, "0000:01:00.0")
	os.MkdirAll(devPath, 0755)
	os.WriteFile(filepath.Join(devPath, "vendor"), []byte("0x1002\n"), 0644)
	os.WriteFile(filepath.Join(devPath, "device"), []byte("0x699f\n"), 0644)

	if !CheckPciDevice("0x1002:0x699f") {
		t.Fatal("expected 0x-prefixed ID to match")
	}
	if !CheckPciDevice("1002:699f") {
		t.Fatal("expected bare ID to match")
	}
}
