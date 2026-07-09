package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"deepin-power-control/internal/global"
	"deepin-power-control/internal/utils"

	"gopkg.in/ini.v1"
)

// ConfigValue stores the value and its origin file for tracing
type ConfigValue struct {
	Value   string
	Source  string
	Section string
}

// ApplyMode merges configurations and applies them to TLP
func ApplyMode(mode string) error {
	global.Info("Starting configuration merge for mode: %s", mode)

	// 1. Scan and retrieve all configuration files
	files, err := utils.GetConfigFilesInDir(global.RawConfigDir)
	if err != nil {
		return fmt.Errorf("configuration scan failed: %w", err)
	}

	mergedMap := make(map[string]ConfigValue)

	// 2. Iterate through files and merge content
	for _, file := range files {
		if err := processSingleFile(file, mode, mergedMap); err != nil {
			global.Warn("Skipping file %s: %v", file, err)
			continue
		}
	}

	// 3. Persist the merged configuration to TLP directory
	return writeMergedConfig(mergedMap, mode)
}

// processSingleFile handles the logic for a single configuration file
func processSingleFile(path string, mode string, mergedMap map[string]ConfigValue) error {
	fileName := filepath.Base(path)

	// Step A: Hardware feature check (The Interceptor)
	if !shouldApplyFile(fileName) {
		global.Debug("File %s skipped: Hardware feature not supported", fileName)
		return nil
	}

	// Step B: Load INI content
	cfg, err := ini.Load(path)
	if err != nil {
		return fmt.Errorf("load failed: %v", err)
	}

	// Step C: Determine format and merge
	if hasSections(cfg) {
		// Format A: Section-based ([DEFAULT] + [mode])
		global.Debug("File %s: Section-based format detected", fileName)
		mergeSection(cfg.Section(""), fileName, mergedMap)
		if sec, err := cfg.GetSection(mode); err == nil {
			mergeSection(sec, fileName, mergedMap)
		}
	} else {
		// Format B: Filename-based or Global
		global.Debug("File %s: No sections detected, checking filename...", fileName)
		targetMode := getModeFromFilename(fileName)
		if targetMode == "" || targetMode == mode {
			mergeSection(cfg.Section(""), fileName, mergedMap)
		} else {
			global.Debug("File %s skipped: Mode mismatch (%s != %s)", fileName, targetMode, mode)
		}
	}

	return nil
}

// shouldApplyFile parses the @feature tag and checks hardware compatibility
func shouldApplyFile(fileName string) bool {
	// Pattern: filename@feature.conf
	if !strings.Contains(fileName, "@") {
		return true
	}

	// Extract feature name between '@' and '.conf'
	parts := strings.Split(fileName, "@")
	if len(parts) < 2 {
		return true
	}
	featurePart := strings.TrimSuffix(parts[1], ".conf")

	// Support dynamic pci: prefix for PCI device matching (e.g. @pci:1002:699f)
	if strings.HasPrefix(featurePart, "pci:") {
		pciID := strings.TrimPrefix(featurePart, "pci:")
		return CheckPciDevice(pciID)
	}

	// Consult the feature registry
	if checkFunc, ok := FeatureChecks[featurePart]; ok {
		return checkFunc()
	}

	global.Warn("Unknown feature tag [@%s] in file %s", featurePart, fileName)
	return false
}

// hasSections checks if the INI file contains any non-default sections
func hasSections(cfg *ini.File) bool {
	return len(cfg.Sections()) > 1
}

// getModeFromFilename extracts mode from name like "xxx_performance.conf" "xxx-performance.conf" "performance.conf"
func getModeFromFilename(name string) string {
	baseName := strings.TrimSuffix(name, ".conf")
	modes := []string{"performance", "saving", "balance", "lowbat"}
	for _, m := range modes {
		if baseName == m || strings.Contains(baseName, "_"+m) || strings.Contains(baseName, "-"+m) {
			return m
		}
	}
	return ""
}

// mergeSection copies keys from an INI section to our result map
func mergeSection(section *ini.Section, sourceName string, res map[string]ConfigValue) {
	sectionName := section.Name()
	for _, key := range section.Keys() {
		res[key.Name()] = ConfigValue{
			Value:   key.String(),
			Source:  sourceName,
			Section: sectionName,
		}
	}
}

// writeMergedConfig handles file cleanup and generation of the final .conf file
func writeMergedConfig(mergedMap map[string]ConfigValue, mode string) error {
	// 1. Clean up old dspc-related config files in /etc/tlp.d/
	existingFiles, _ := filepath.Glob(filepath.Join(global.TlpConfigDir, "*.conf"))
	for _, f := range existingFiles {
		base := filepath.Base(f)
		if strings.Contains(base, "deepin-system-power-control") || strings.Contains(base, "dspc") || strings.Contains(base, "deepin-power-control") {
			global.Debug("Removing old config: %s", base)
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				// Key point: Deletion failure should be treated as an error, preventing subsequent writes.
				return fmt.Errorf("failed to remove old config file %s: %w", f, err)
			}
		}
	}

	// 2. Create the target file
	targetFileName := fmt.Sprintf("%s_%s.conf", global.TargetTlpFilePrefix, mode)
	targetPath := filepath.Join(global.TlpConfigDir, targetFileName)
	f, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create target file: %w", err)
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	_, _ = writer.WriteString("# This file is auto-generated by deepin-power-control. DO NOT EDIT.\n")

	// 3. Group keys by source to minimize redundant comments
	// First, get sorted keys to ensure stable output
	keys := make([]string, 0, len(mergedMap))
	for k := range mergedMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Source -> Section -> []Keys
	hierarchy := make(map[string]map[string][]string)

	for k, cv := range mergedMap {
		if hierarchy[cv.Source] == nil {
			hierarchy[cv.Source] = make(map[string][]string)
		}
		hierarchy[cv.Source][cv.Section] = append(hierarchy[cv.Source][cv.Section], k)
	}

	// A. sort Source (file name)
	sources := make([]string, 0, len(hierarchy))
	for src := range hierarchy {
		sources = append(sources, src)
	}
	sort.Strings(sources)

	for _, src := range sources {
		_, _ = writer.WriteString(fmt.Sprintf("\n# >>> From File: %s\n", src))

		// B. sort Section
		sections := make([]string, 0, len(hierarchy[src]))
		for sec := range hierarchy[src] {
			sections = append(sections, sec)
		}

		sort.Slice(sections, func(i, j int) bool {
			if sections[i] == "DEFAULT" {
				return true
			}
			if sections[j] == "DEFAULT" {
				return false
			}
			return sections[i] < sections[j]
		})

		for _, sec := range sections {
			_, _ = writer.WriteString(fmt.Sprintf("# [%s]\n", sec))

			keys := hierarchy[src][sec]
			sort.Strings(keys)
			for _, k := range keys {
				_, _ = writer.WriteString(fmt.Sprintf("%s=%s\n", k, mergedMap[k].Value))
			}
			_, _ = writer.WriteString("\n")
		}
	}

	return writer.Flush()
}

// InitStart calls 'tlp init start' to reload TLP without config modification
func InitStart() error {
	global.Info("Running TLP init-start...")
	// Use the utility to run: /usr/sbin/tlp init start
	return utils.RunCmd(global.CmdTlp, "init", "start")
}
