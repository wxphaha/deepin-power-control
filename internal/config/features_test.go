package config

import (
	"os"
	"path/filepath"
	"testing"
)

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
