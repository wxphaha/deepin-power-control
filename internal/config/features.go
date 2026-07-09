package config

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"deepin-power-control/internal/global"
	"deepin-power-control/internal/utils"
)

// FeatureChecks maps filename tags (like "epp") to their detection functions.
var FeatureChecks = map[string]func() bool{
	"epp":      CheckEppSupport,
	"portable": CheckPortable,
	"zhaoxin":  CheckZhaoxin,
}

// Internal paths for hardware detection.
// Defined as variables instead of constants to allow override during unit tests.
var (
	pathChassisType  = "/sys/class/dmi/id/chassis_type"
	pathEppAvailable = "/sys/devices/system/cpu/cpu0/cpufreq/energy_performance_available_preferences"
	pathIntelPstate  = "/sys/devices/system/cpu/intel_pstate"
	pathCpuInfo      = "/proc/cpuinfo"
	pathAmdPstate    = "/sys/devices/system/cpu/amd_pstate"
	pathAmdStatus    = "/sys/devices/system/cpu/amd_pstate/status"
	pathPciDevices   = "/sys/bus/pci/devices"
)

// portableChassisTypes are chassis type values that indicate portable/laptop devices.
// Reference: SMBIOS specification 3.5, Table 9.
var portableChassisTypes = map[int]bool{
	8:  true, // Portable
	9:  true, // Laptop
	10: true, // Notebook
	11: true, // Hand Held
	14: true, // Sub Notebook
}

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

// CheckPortable determines if the machine is a portable device (laptop/tablet).
// Returns true on detection failure (fail-safe: assume portable to disable WOL).
func CheckPortable() bool {
	data, err := ioutil.ReadFile(pathChassisType)
	if err != nil {
		global.Warn("Failed to read chassis_type: %v, assuming portable", err)
		return true
	}

	chassisType, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		global.Warn("Failed to parse chassis_type: %v, assuming portable", err)
		return true
	}

	isPortable := portableChassisTypes[chassisType]
	global.Debug("Chassis type: %d, portable: %v", chassisType, isPortable)
	return isPortable
}

// CheckZhaoxin detects Zhaoxin CPUs by checking vendor_id in /proc/cpuinfo.
// Zhaoxin CPUs report vendor_id as "Shanghai" or "CentaurHauls".
func CheckZhaoxin() bool {
	return utils.FileContains(pathCpuInfo, "Shanghai") || utils.FileContains(pathCpuInfo, "CentaurHauls")
}

// CheckPciDevice checks if a PCI device with the given vendor:device ID exists.
// The pciID should be in "vendor:device" format (e.g. "1002:699f").
func CheckPciDevice(pciID string) bool {
	entries, err := os.ReadDir(pathPciDevices)
	if err != nil {
		global.Warn("Failed to read PCI devices directory: %v", err)
		return false
	}

	pciID = strings.ToLower(pciID)
	parts := strings.SplitN(pciID, ":", 2)
	if len(parts) != 2 {
		global.Warn("Invalid PCI ID format: %s (expected vendor:device)", pciID)
		return false
	}
	targetVendor, targetDevice := parts[0], parts[1]

	for _, entry := range entries {
		devPath := filepath.Join(pathPciDevices, entry.Name())

		vendor, err := os.ReadFile(filepath.Join(devPath, "vendor"))
		if err != nil {
			continue
		}
		device, err := os.ReadFile(filepath.Join(devPath, "device"))
		if err != nil {
			continue
		}

		// sysfs vendor/device files are formatted as "0x1002\n"
		vendorStr := strings.ToLower(strings.TrimSpace(string(vendor)))
		deviceStr := strings.ToLower(strings.TrimSpace(string(device)))

		// Strip "0x" prefix if present
		vendorStr = strings.TrimPrefix(vendorStr, "0x")
		deviceStr = strings.TrimPrefix(deviceStr, "0x")

		if vendorStr == targetVendor && deviceStr == targetDevice {
			global.Debug("Found PCI device %s:%s at %s", targetVendor, targetDevice, entry.Name())
			return true
		}
	}

	global.Debug("PCI device %s not found", pciID)
	return false
}
