// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package rules

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"deepin-power-control/internal/global"
	"deepin-power-control/internal/utils"

	"gopkg.in/yaml.v2"
)

// Rule defines a single rule: match conditions + one action.
// Evaluation: when the conditions match, the action runs. No override, no merge.
type Rule struct {
	Match  Match         `yaml:"match"`
	Write  *WriteAction  `yaml:"write"`
	Exec   *ExecAction   `yaml:"exec"`
	Config *ConfigAction `yaml:"config"`

	source string // source file, filled at load time, not part of YAML
}

// Match holds the conditions: fields are ANDed, lists within a field are ORed, empty fields are unrestricted.
type Match struct {
	Mode    StringList `yaml:"mode"`
	Feature FeatureSet `yaml:"feature"`
}

// StringList accepts a single YAML string or a list of strings (mode: balance or mode: [a, b]).
type StringList []string

// UnmarshalYAML implements yaml.v2 custom parsing.
func (s *StringList) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var single string
	if err := unmarshal(&single); err == nil {
		*s = StringList{single}
		return nil
	}

	var list []string
	if err := unmarshal(&list); err != nil {
		return err
	}
	*s = StringList(list)
	return nil
}

// FeatureSet is a set of feature conditions: key = feature name, value =
// parameter list (empty for parameterless features). Multiple keys are
// ORed; parameters within a key are ORed.
//
// Accepts three YAML forms (for backward compatibility):
//
//	feature: portable                          - single feature
//	feature: [portable, zhaoxin]               - multiple features (OR)
//	feature: {pci: ["1002:6611"]} - feature with parameters
type FeatureSet map[string][]string

// UnmarshalYAML implements yaml.v2 custom parsing for all three forms.
func (f *FeatureSet) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var single string
	if err := unmarshal(&single); err == nil {
		*f = FeatureSet{single: nil}
		return nil
	}

	var list []string
	if err := unmarshal(&list); err == nil {
		m := make(FeatureSet, len(list))
		for _, name := range list {
			m[name] = nil
		}
		*f = m
		return nil
	}

	var m map[string][]string
	if err := unmarshal(&m); err != nil {
		return err
	}
	*f = FeatureSet(m)
	return nil
}

// LoadRules scans the given directories for *.yaml rule files, ordered by
// directory argument first, then by depth and alphabetically within each
// directory. Later rules override earlier config fragments, so callers
// should pass base directories first and customization directories last.
// A failed file only warns and is skipped; other files are unaffected.
func LoadRules(baseDirs ...string) []*Rule {
	var rules []*Rule
	for _, baseDir := range baseDirs {
		if !isSafeDir(baseDir) {
			global.Warn("Rules directory %s skipped: must be root-owned and not writable by group/other", baseDir)
			continue
		}

		files, err := utils.GetFilesBySuffix(baseDir, ".yaml")
		if err != nil {
			global.Warn("Rule scan failed: %v", err)
			continue
		}

		for _, file := range files {
			// Check the parent directory too: a writable subdirectory (e.g.
			// oem/) would let an attacker drop new rule files that
			// pass the per-file checks.
			if !isSafeDir(filepath.Dir(file)) {
				global.Warn("Rule directory %s skipped: must be root-owned and not writable by group/other",
					filepath.Dir(file))
				continue
			}

			f, err := openSafeRuleFile(file)
			if err != nil {
				global.Warn("Rule file %s skipped: %v", file, err)
				continue
			}
			data, readErr := io.ReadAll(f)
			closeErr := f.Close()
			if readErr != nil {
				global.Warn("Failed to read rule file %s: %v", file, readErr)
				continue
			}
			if closeErr != nil {
				global.Warn("Failed to close rule file %s: %v", file, closeErr)
				continue
			}

			var fileRules []*Rule
			if err := yaml.Unmarshal(data, &fileRules); err != nil {
				global.Warn("Failed to parse rule file %s: %v", file, err)
				continue
			}

			for _, r := range fileRules {
				// A YAML null entry (e.g. a stray "- " list item) unmarshals
				// to a nil *Rule; skip it instead of dereferencing nil.
				if r == nil {
					continue
				}
				r.source = file
				if err := r.validate(); err != nil {
					global.Warn("Invalid rule in %s: %v", file, err)
					continue
				}
				rules = append(rules, r)
			}
		}
	}
	return rules
}

// openSafeRuleFile opens a rule file without following symlinks and verifies
// the opened inode itself via fstat. Checking and reading share the same fd,
// so a file swapped between a path-based check and the read cannot slip in.
func openSafeRuleFile(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		// os.NewFile only returns nil for an invalid fd; close the
		// descriptor anyway so this path never leaks it.
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to create file handle")
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !isSafeRuleFileInfo(info) {
		f.Close()
		return nil, fmt.Errorf("must be root-owned and not writable by group/other")
	}
	return f, nil
}

// isSafeRuleFileInfo reports whether an opened rule file is safe to load:
// owned by root (when running as root) and not writable by group/other.
// Defense-in-depth: dpkg installs these files as root:root 0644, this
// guards against a directory that accidentally became writable.
func isSafeRuleFileInfo(info os.FileInfo) bool {
	// Reject symlinks explicitly rather than relying on their 0777 mode bits
	// (an OS-specific detail): a rule file must be a plain regular file.
	// O_NOFOLLOW in openSafeRuleFile already rejects symlinks at open time.
	if info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	if info.Mode().Perm()&0022 != 0 {
		return false
	}
	if os.Geteuid() == 0 {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid != 0 {
			return false
		}
	}
	return true
}

// isSafeDir reports whether a directory is root-owned and not writable by
// group/other, so non-root users cannot drop or replace files inside it.
func isSafeDir(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	if !info.IsDir() {
		return false
	}
	if info.Mode().Perm()&0022 != 0 {
		return false
	}
	if os.Geteuid() == 0 {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid != 0 {
			return false
		}
	}
	return true
}

// validate checks rule validity: exactly one action, and the action itself is valid.
func (r *Rule) validate() error {
	n := 0
	if r.Write != nil {
		n++
	}
	if r.Exec != nil {
		n++
	}
	if r.Config != nil {
		n++
	}
	if n != 1 {
		return fmt.Errorf("rule must have exactly one action (write, exec or config)")
	}
	if r.Write != nil {
		return r.Write.validate()
	}
	if r.Exec != nil {
		return r.Exec.validate()
	}
	return r.Config.validate()
}

// sourceFile returns the rule source file (for logging).
func (r *Rule) sourceFile() string {
	if r.source == "" {
		return "(unknown)"
	}
	return r.source
}

// trimSource returns the basename for log output.
func trimSource(file string) string {
	idx := strings.LastIndex(file, "/")
	if idx < 0 {
		return file
	}
	return file[idx+1:]
}
