package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"deepin-power-control/internal/global"
	"deepin-power-control/internal/utils"
)

// sysfsRoot is the only directory write actions may touch.
// Kept as a var so tests can redirect it to a temp dir.
var sysfsRoot = "/sys"

// execTimeout bounds how long an exec action may run before being killed,
// preventing a hanging command from blocking the whole mode switch.
var execTimeout = 60 * time.Second

// readFile is injectable for tests simulating write-only sysfs nodes.
var readFile = os.ReadFile

// Action is the interface for rule actions. Future action types (modprobe/systemctl,
// etc.) just implement this interface and register it in the rule parser.
type Action interface {
	Describe() string
	Apply() error
}

// WriteAction writes a value to sysfs paths.
// Path supports glob (e.g. /sys/class/devfreq/devfreq*/governor); every match is written.
type WriteAction struct {
	Path  string `yaml:"path"`
	Value string `yaml:"value"`
}

// isWithinSysfs reports whether a normalized (cleaned/resolved) path
// stays inside sysfsRoot. Defense-in-depth: the caller must normalize
// the path (filepath.Clean / EvalSymlinks) before checking.
func isWithinSysfs(path string) bool {
	return path == sysfsRoot || strings.HasPrefix(path, sysfsRoot+"/")
}

func (w *WriteAction) validate() error {
	if w.Path == "" {
		return fmt.Errorf("write action: empty path")
	}
	// Normalize before the prefix check: /sys/../../etc/passwd cleans to /etc/passwd
	// and must be rejected instead of passing the raw "/sys/" prefix test.
	cleaned := filepath.Clean(w.Path)
	if !isWithinSysfs(cleaned) {
		return fmt.Errorf("write action: path %s resolves outside %s", w.Path, sysfsRoot)
	}
	return nil
}

func (w *WriteAction) Describe() string {
	return fmt.Sprintf("write %s = %s", w.Path, w.Value)
}

// Apply expands the glob, writes every matched path and verifies by reading back.
// A glob without matches is skipped (device absent on this platform), not an error.
func (w *WriteAction) Apply() error {
	targets, err := filepath.Glob(w.Path)
	if err != nil {
		return fmt.Errorf("invalid glob %s: %v", w.Path, err)
	}
	if len(targets) == 0 {
		global.Debug("Write rule: no device matched %s, skip", w.Path)
		return nil
	}

	var lastErr error
	for _, target := range targets {
		// Resolve symlinks first: a link under /sys/ may point outside it
		// (e.g. to /etc or /var). Write only when the resolved target stays
		// inside sysfsRoot, otherwise skip. Writing the resolved path also
		// removes the TOCTOU window between check and write.
		resolved, err := filepath.EvalSymlinks(target)
		if err != nil {
			global.Warn("Write rule failed on %s: %v", target, err)
			lastErr = err
			continue
		}
		if !isWithinSysfs(resolved) {
			global.Warn("Write rule skipped %s: resolves to %s outside %s",
				target, resolved, sysfsRoot)
			continue
		}

		if err := writeAndVerify(resolved, w.Value); err != nil {
			global.Warn("Write rule failed on %s: %v", target, err)
			lastErr = err
			continue
		}
		global.Info("Write rule applied: %s = %s", target, w.Value)
	}
	return lastErr
}

// writeAndVerify writes and reads back to ensure the kernel accepted the value.
// Read-back failure is treated as success: some sysfs nodes are write-only
// (e.g. trigger files) and reject reads even though the write succeeded.
// Comparison trims whitespace on both sides: sysfs often normalizes values
// (e.g. appends a newline), and YAML values may carry trailing spaces.
func writeAndVerify(path, value string) error {
	if err := os.WriteFile(path, []byte(value), 0); err != nil {
		return err
	}

	data, err := readFile(path)
	if err != nil {
		return nil
	}
	got := strings.TrimSpace(string(data))
	if got != strings.TrimSpace(value) {
		return fmt.Errorf("verify mismatch: expected %q, got %q", value, got)
	}
	return nil
}

// ConfigAction generates a TLP config fragment. Unlike write/exec, config
// fragments are NOT executed - they are merged into the TLP configuration
// (later fragments override earlier ones), so the matched mode is the only
// condition needed; no [mode] section is required inside the fragment.
type ConfigAction map[string]string

func (c *ConfigAction) validate() error {
	if len(*c) == 0 {
		return fmt.Errorf("config action: empty items")
	}
	return nil
}

func (c *ConfigAction) Describe() string {
	keys := make([]string, 0, len(*c))
	for k := range *c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return fmt.Sprintf("config %v", keys)
}

// ExecAction runs a program: the command never goes through a shell,
// it is parsed into argv and executed directly. For shell features (pipes/
// redirection) write the command explicitly as /bin/sh -c '...'.
type ExecAction struct {
	Command string `yaml:"command"`
}

func (e *ExecAction) validate() error {
	if e.Command == "" {
		return fmt.Errorf("exec action: empty command")
	}
	return nil
}

func (e *ExecAction) Describe() string {
	return fmt.Sprintf("exec %s", e.Command)
}

// Apply parses the command into argv and runs it with a timeout,
// so a hanging command cannot block the mode switch forever.
func (e *ExecAction) Apply() error {
	args, err := splitCommand(e.Command)
	if err != nil {
		return fmt.Errorf("failed to parse command %q: %v", e.Command, err)
	}
	if len(args) == 0 {
		return fmt.Errorf("exec action: empty command")
	}

	if err := utils.RunCmdWithTimeout(execTimeout, args[0], args[1:]...); err != nil {
		return fmt.Errorf("exec %s failed: %v", e.Command, err)
	}
	global.Info("Exec rule applied: %s", e.Command)
	return nil
}

// splitCommand parses a command line into an argv list.
// Supports single/double quotes and backslash escapes; no shell means no injection risk.
func splitCommand(s string) ([]string, error) {
	// Null bytes are valid in Go strings but terminate argv at execve time,
	// silently truncating the command. Reject them explicitly.
	if strings.ContainsRune(s, 0) {
		return nil, fmt.Errorf("null byte in command")
	}

	var args []string
	var cur strings.Builder
	inQuote := byte(0) // 0=none, '\''=single, '"'=double
	escaped := false
	started := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			cur.WriteByte(c)
			escaped = false
			started = true
			continue
		}
		switch {
		case c == '\\' && inQuote != '\'':
			escaped = true
			started = true
		case inQuote != 0:
			if c == inQuote {
				inQuote = 0
			} else {
				cur.WriteByte(c)
			}
			started = true
		case c == '\'' || c == '"':
			inQuote = c
			started = true
		case c == ' ' || c == '\t':
			if started {
				args = append(args, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteByte(c)
			started = true
		}
	}
	if inQuote != 0 {
		return nil, fmt.Errorf("unbalanced quote")
	}
	if escaped {
		return nil, fmt.Errorf("dangling escape")
	}
	if started {
		args = append(args, cur.String())
	}
	return args, nil
}
