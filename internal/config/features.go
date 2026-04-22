package config

import (
	"os"
	"strings"

	"deepin-power-control/internal/utils"
)

// FeatureChecks maps filename tags (like "epp") to their detection functions.
var FeatureChecks = map[string]func() bool{
	"epp": CheckEppSupport,
	// You can easily add more here, e.g., "nv" : CheckNvidiaGpu
}

// Internal paths for hardware detection.
// Defined as variables instead of constants to allow override during unit tests.
var (
	pathEppAvailable = "/sys/devices/system/cpu/cpu0/cpufreq/energy_performance_available_preferences"
	pathIntelPstate  = "/sys/devices/system/cpu/intel_pstate"
	pathCpuInfo      = "/proc/cpuinfo"
	pathAmdPstate    = "/sys/devices/system/cpu/amd_pstate"
	pathAmdStatus    = "/sys/devices/system/cpu/amd_pstate/status"
)

// CheckEppSupport determines if the CPU supports Energy Performance Preference (EPP).
func CheckEppSupport() bool {
	// 1. Check direct sysfs availability
	if _, err := os.Stat(pathEppAvailable); err == nil {
		return true
	}

	// 2. Check Intel P-State HWP EPP
	if utils.IsDir(pathIntelPstate) {
		if utils.FileContains(pathCpuInfo, "hwp_epp") {
			return true
		}
	}

	// 3. Check AMD P-State active status
	if utils.IsDir(pathAmdPstate) {
		content, err := os.ReadFile(pathAmdStatus)
		if err == nil && strings.TrimSpace(string(content)) == "active" {
			return true
		}
	}

	return false
}
