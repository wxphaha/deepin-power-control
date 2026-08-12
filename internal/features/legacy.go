package features

// Code detectors backing parameterized declarations. The pci_vendor_device
// declaration in features.yaml dispatches here; the parameter (vendor:device)
// arrives at match time so OEM configs need no declaration changes.
// New detectors for parameterized types are added here, not in separate files.

import (
	"os"
	"path/filepath"
	"strings"

	"deepin-power-control/internal/global"
)

// PathPciDevices is a variable to allow override during unit tests.
var PathPciDevices = "/sys/bus/pci/devices"

// pciDeviceCache memoizes the scanned PCI device set for the process
// lifetime. A mode switch evaluates many parameterized pci checks (one per
// parameter); scanning /sys/bus/pci/devices/*/vendor+device once and
// querying an in-memory set avoids N x M file reads. The process is
// short-lived (one set invocation), so the cache never goes stale.
var pciDeviceCache struct {
	scanned bool
	devices map[string]bool // "vendor:device" -> present
}

// ResetPciCache clears the memoized scan; only needed by tests that
// override PathPciDevices between calls.
func ResetPciCache() {
	pciDeviceCache.scanned = false
	pciDeviceCache.devices = nil
}

// evaluateLegacy evaluates code-backed (legacy) feature types. engine.go
// dispatches here for any type not handled by the built-in evaluator;
// adding a new legacy type only requires a new case in this function.
func evaluateLegacy(def *Def, param string) bool {
	switch def.Type {
	case TypePciVendorDevice:
		// Parameterized type: the param (vendor:device) arrives at match
		// time; a bare evaluation (no param) is a config error.
		if param == "" {
			global.Warn("Feature [%s]: type %s requires a parameter (use base:param)", def.Name, def.Type)
			return false
		}
		return CheckPciDevice(param)
	default:
		global.Warn("Unknown feature type [%s] for feature %s", def.Type, def.Name)
		return false
	}
}

// CheckPciDevice checks if a PCI device with the given vendor:device ID exists.
// The pciID should be in "vendor:device" format (e.g. "1002:699f").
// Backs the parameterized "pci_vendor_device" feature type.
func CheckPciDevice(pciID string) bool {
	pciID = strings.ToLower(strings.TrimSpace(pciID))
	parts := strings.SplitN(pciID, ":", 2)
	if len(parts) != 2 {
		global.Warn("Invalid PCI ID format: %s (expected vendor:device)", pciID)
		return false
	}

	// Normalize each part (strip "0x" like the scan cache keys) so IDs
	// written from sysfs values (e.g. "0x1002:0x699f") match too.
	devices := scanPciDevices()
	key := normalizeHex([]byte(parts[0])) + ":" + normalizeHex([]byte(parts[1]))
	if devices[key] {
		global.Debug("Found PCI device %s", pciID)
		return true
	}
	global.Debug("PCI device %s not found", pciID)
	return false
}

// scanPciDevices walks /sys/bus/pci/devices once and returns the set of
// "vendor:device" IDs present. The result is memoized for the process.
func scanPciDevices() map[string]bool {
	if pciDeviceCache.scanned {
		return pciDeviceCache.devices
	}
	pciDeviceCache.devices = make(map[string]bool)

	entries, err := os.ReadDir(PathPciDevices)
	if err != nil {
		// Do not mark the cache as scanned on failure: a transient error
		// (e.g. sysfs not fully mounted early in boot) would otherwise
		// disable all PCI feature checks for the process lifetime.
		global.Warn("Failed to read PCI devices directory: %v", err)
		return pciDeviceCache.devices
	}

	for _, entry := range entries {
		devPath := filepath.Join(PathPciDevices, entry.Name())

		vendor, err := os.ReadFile(filepath.Join(devPath, "vendor"))
		if err != nil {
			continue
		}
		device, err := os.ReadFile(filepath.Join(devPath, "device"))
		if err != nil {
			continue
		}
		pciDeviceCache.devices[normalizeHex(vendor)+":"+normalizeHex(device)] = true
	}

	pciDeviceCache.scanned = true
	return pciDeviceCache.devices
}

// normalizeHex converts a sysfs hex value like "0x1002\n" to "1002".
func normalizeHex(b []byte) string {
	s := strings.ToLower(strings.TrimSpace(string(b)))
	return strings.TrimPrefix(s, "0x")
}
