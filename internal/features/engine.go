package features

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"deepin-power-control/internal/global"

	"gopkg.in/yaml.v2"
)

//go:embed features.yaml
var featuresYAML []byte

// Def describes how to detect one hardware feature. It is pure data;
// evaluation logic lives in the engine below, so adding a feature that
// fits a declared type requires no code change.
//
// Parameterized types (e.g. pci_vendor_device) carry their parameters at
// match time, not in the declaration: the feature name is matched with a
// "base:param" suffix (e.g. "pci:1002:699f") and the param is passed to
// the detector. This lets OEM configs express arbitrary parameter values
// without embedding them in the compiled features.yaml.
type Def struct {
	Name       string   `yaml:"-"`
	Type       string   `yaml:"type"`
	Path       string   `yaml:"path"`
	Values     []string `yaml:"values"`
	Contains   []string `yaml:"contains"`
	OnError    *bool    `yaml:"on_error"`
	Conditions []Def    `yaml:"conditions"`
}

// Declared feature types
const (
	TypeFileValueIn     = "file_value_in"
	TypeFileContains    = "file_contains"
	TypePathExists      = "path_exists"
	TypeDirExists       = "dir_exists"
	TypeAny             = "any"
	TypeAll             = "all"
	TypePciVendorDevice = "pci_vendor_device"
)

// Load parses the embedded features.yaml into name-indexed definitions.
// A broken declaration only warns; the affected feature is unavailable
// (fail-closed) and falls back to the legacy registry.
func Load() map[string]Def {
	raw := struct {
		Features map[string]Def `yaml:"features"`
	}{}
	if err := yaml.Unmarshal(featuresYAML, &raw); err != nil {
		global.Warn("Failed to parse embedded features.yaml: %v", err)
		return nil
	}
	defs := make(map[string]Def, len(raw.Features))
	for name, def := range raw.Features {
		def.Name = name
		if err := validateDef(&def); err != nil {
			global.Warn("Invalid feature declaration [%s]: %v", name, err)
			continue
		}
		defs[name] = def
	}
	return defs
}

func validateDef(def *Def) error {
	switch def.Type {
	case TypeFileValueIn, TypeFileContains:
		if def.Path == "" {
			return fmt.Errorf("missing path")
		}
	case TypePathExists, TypeDirExists:
		if def.Path == "" {
			return fmt.Errorf("missing path")
		}
	case TypeAny, TypeAll:
		if len(def.Conditions) == 0 {
			return fmt.Errorf("missing conditions")
		}
		for i := range def.Conditions {
			if err := validateDef(&def.Conditions[i]); err != nil {
				return fmt.Errorf("condition %d: %v", i, err)
			}
		}
	case TypePciVendorDevice:
		// Parameterized: no declaration-time fields; the parameter
		// (vendor:device) arrives at match time.
	default:
		return fmt.Errorf("unknown type %q", def.Type)
	}
	return nil
}

// FeatureChecks is the code fallback registry for detection that cannot
// be expressed declaratively in features.yaml (including parameterized
// declarations). Every detector takes a parameter string; detectors
// without a parameter ignore it (pass ""). Currently empty - every
// detector is declared in features.yaml. Add a code detector here only
// when a detection truly cannot be declared; the filename parsing code
// never changes.
var FeatureChecks = map[string]func(param string) bool{
	"dev-idle-on": checkDevIdleOn,
}

// Engine evaluates declared features, with the code registry above as an
// implicit fallback for detection that cannot be expressed declaratively.
type Engine struct {
	defs map[string]Def
}

// New builds an Engine from the embedded declarations in features.yaml.
func New() *Engine {
	return &Engine{defs: Load()}
}

// Enabled reports whether the named feature is present on this machine.
// Parameterized names ("base:param", e.g. "pci:1002:699f") are split and
// evaluated as EnabledParam. Resolution order: declared config (exact
// name) -> parameterized declaration -> code registry -> false.
func (e *Engine) Enabled(name string) bool {
	// Parameterized names: "base:param" (e.g. "pci:1002:699f") first try
	// a parameterized declaration, then a parameterized code detector.
	if idx := strings.Index(name, ":"); idx > 0 {
		return e.EnabledParam(name[:idx], name[idx+1:])
	}
	return e.EnabledParam(name, "")
}

// EnabledParam evaluates a feature with an optional parameter. Features
// that do not take a parameter ignore it; parameterized types (e.g.
// pci_vendor_device) require it. Rules pass parameters separately; config
// filename tags use the colon form through Enabled.
func (e *Engine) EnabledParam(name, param string) bool {
	if def, ok := e.defs[name]; ok {
		return e.evaluate(&def, param)
	}
	if fn, ok := FeatureChecks[name]; ok {
		global.Debug("Feature [%s] evaluated by code registry", name)
		return fn(param)
	}
	global.Warn("Unknown feature [%s]", name)
	return false
}

// evaluate walks a declaration tree. Nested conditions (any/all) can go
// arbitrarily deep; the tree is bounded by the embedded file, not user
// input, so no recursion depth guard is needed. The param is forwarded to
// nested conditions; types that do not take a parameter ignore it.
func (e *Engine) evaluate(def *Def, param string) bool {
	switch def.Type {
	case TypeFileValueIn:
		return e.fileValueIn(def)
	case TypeFileContains:
		return e.fileContains(def)
	case TypePathExists:
		_, err := os.Stat(def.Path)
		if err != nil {
			global.Debug("Feature [%s]: path %s not present", def.Name, def.Path)
		}
		return err == nil
	case TypeDirExists:
		info, err := os.Stat(def.Path)
		if err != nil {
			global.Debug("Feature [%s]: dir %s not present", def.Name, def.Path)
			return false
		}
		return info.IsDir()
	case TypeAny:
		for i := range def.Conditions {
			if e.evaluate(&def.Conditions[i], param) {
				return true
			}
		}
		return false
	case TypeAll:
		for i := range def.Conditions {
			if !e.evaluate(&def.Conditions[i], param) {
				return false
			}
		}
		return true
	default:
		// Legacy (code-backed) types are dispatched here; adding a new
		// legacy type only touches evaluateLegacy in legacy.go.
		return evaluateLegacy(def, param)
	}
}

// onErrorValue returns the declared on_error value, or false when unset.
// A failed file read makes the feature return this value: fail-closed
// (false) by default, fail-safe (true) for security-sensitive features
// like portable (disable WOL when detection fails).
func onErrorValue(def *Def) bool {
	if def.OnError != nil {
		return *def.OnError
	}
	return false
}

func (e *Engine) fileValueIn(def *Def) bool {
	data, err := os.ReadFile(def.Path)
	if err != nil {
		global.Debug("Feature [%s]: failed to read %s: %v", def.Name, def.Path, err)
		return onErrorValue(def)
	}
	got := strings.TrimSpace(string(data))
	for _, v := range def.Values {
		if got == v {
			return true
		}
	}
	global.Debug("Feature [%s]: value %q not in %v", def.Name, got, def.Values)
	return false
}

func (e *Engine) fileContains(def *Def) bool {
	data, err := os.ReadFile(def.Path)
	if err != nil {
		global.Debug("Feature [%s]: failed to read %s: %v", def.Name, def.Path, err)
		return onErrorValue(def)
	}
	content := string(data)
	for _, sub := range def.Contains {
		if strings.Contains(content, sub) {
			return true
		}
	}
	global.Debug("Feature [%s]: none of %v found in %s", def.Name, def.Contains, filepath.Base(def.Path))
	return false
}
