package debugger

import (
	"fmt"
	"path/filepath"
	"sort"

	"deepin-power-control/internal/global"
	"deepin-power-control/internal/utils"

	"gopkg.in/ini.v1"
)

// ConfigPair represents a simple key-value configuration
type ConfigPair struct {
	Key   string
	Value string
}

// FlattenConfigs scans the TLP configuration directory and merges all .conf files
// into a single slice of ConfigPair. It follows alphabetical and depth-based override rules.
func FlattenConfigs(tlpConfigDir string) ([]ConfigPair, error) {
	global.Info("Flattening all configurations in %s for debugging", tlpConfigDir)

	// 1. Collect and sort all .conf files in the target directory
	files, err := utils.GetConfigFilesInDir(tlpConfigDir)
	if err != nil {
		return nil, fmt.Errorf("failed to scan config directory: %w", err)
	}

	// Use a map to handle overrides (later files overwrite earlier ones)
	mergedMap := make(map[string]string)

	// 2. Load each file as a simple key-value store (ignoring sections)
	for _, file := range files {
		global.Debug("Processing file for debug: %s", filepath.Base(file))

		// Load with loose options to handle TLP's shell-like syntax
		cfg, err := ini.LoadSources(ini.LoadOptions{
			IgnoreInlineComment: true,
		}, file)

		if err != nil {
			global.Warn("Failed to parse %s, skipping: %v", file, err)
			continue
		}

		// Iterate through all sections (usually just DEFAULT in simple files)
		// and collect every key to the mergedMap
		for _, section := range cfg.Sections() {
			for _, key := range section.Keys() {
				mergedMap[key.Name()] = key.String()
			}
		}
	}

	// 3. Convert map to a sorted slice for deterministic binary search
	return mapToSortedSlice(mergedMap), nil
}

// mapToSortedSlice converts the merged map into a slice sorted by Key
// This ensures that the binary search always operates on the same sequence
func mapToSortedSlice(m map[string]string) []ConfigPair {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]ConfigPair, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, ConfigPair{
			Key:   k,
			Value: m[k],
		})
	}
	return pairs
}
