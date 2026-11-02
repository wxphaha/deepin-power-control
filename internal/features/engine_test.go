// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package features

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempFile creates a temp file with the given content and returns its path.
func withTempFile(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "probe")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestFileValueIn(t *testing.T) {
	p := withTempFile(t, "9\n")
	e := &Engine{defs: map[string]Def{
		"x": {Name: "x", Type: TypeFileValueIn, Path: p, Values: []string{"8", "9", "10"}},
	}}
	if !e.Enabled("x") {
		t.Fatal("expected match for value 9")
	}

	e2 := &Engine{defs: map[string]Def{
		"x": {Name: "x", Type: TypeFileValueIn, Path: p, Values: []string{"3", "4"}},
	}}
	if e2.Enabled("x") {
		t.Fatal("expected no match for value 9")
	}
}

func TestFileValueInOnError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	e := &Engine{defs: map[string]Def{
		"x": {Name: "x", Type: TypeFileValueIn, Path: missing, Values: []string{"1"}, OnError: boolPtr(true)},
	}}
	if !e.Enabled("x") {
		t.Fatal("expected on_error=true fallback")
	}

	e2 := &Engine{defs: map[string]Def{
		"x": {Name: "x", Type: TypeFileValueIn, Path: missing, Values: []string{"1"}},
	}}
	if e2.Enabled("x") {
		t.Fatal("expected fail-closed when on_error unset")
	}
}

func TestFileContains(t *testing.T) {
	p := withTempFile(t, "Shanghai CentaurHauls")
	e := &Engine{defs: map[string]Def{
		"x": {Name: "x", Type: TypeFileContains, Path: p, Contains: []string{"Shanghai"}},
	}}
	if !e.Enabled("x") {
		t.Fatal("expected contains match")
	}

	e2 := &Engine{defs: map[string]Def{
		"x": {Name: "x", Type: TypeFileContains, Path: p, Contains: []string{"nope"}},
	}}
	if e2.Enabled("x") {
		t.Fatal("expected no contains match")
	}
}

func TestPathExistsAndDirExists(t *testing.T) {
	dir := t.TempDir()
	file := withTempFile(t, "x")

	e := &Engine{defs: map[string]Def{
		"d": {Name: "d", Type: TypeDirExists, Path: dir},
		"f": {Name: "f", Type: TypePathExists, Path: file},
	}}
	if !e.Enabled("d") || !e.Enabled("f") {
		t.Fatal("expected existing paths to match")
	}

	e2 := &Engine{defs: map[string]Def{
		"d": {Name: "d", Type: TypeDirExists, Path: file}, // file is not a dir
	}}
	if e2.Enabled("d") {
		t.Fatal("expected file to fail dir_exists")
	}
}

func TestAnyAllNested(t *testing.T) {
	fileA := withTempFile(t, "aaa")
	fileB := withTempFile(t, "bbb")

	e := &Engine{defs: map[string]Def{
		"combo": {Name: "combo", Type: TypeAny, Conditions: []Def{
			{Type: TypePathExists, Path: "/nonexistent/xyz"},
			{Type: TypeAll, Conditions: []Def{
				{Type: TypePathExists, Path: fileA},
				{Type: TypePathExists, Path: fileB},
			}},
		}},
		"nested": {Name: "nested", Type: TypeAll, Conditions: []Def{
			{Type: TypePathExists, Path: fileA},
			{Type: TypeAny, Conditions: []Def{
				{Type: TypePathExists, Path: fileB},
				{Type: TypePathExists, Path: "/nonexistent/abc"},
			}},
		}},
		"fail": {Name: "fail", Type: TypeAll, Conditions: []Def{
			{Type: TypePathExists, Path: fileA},
			{Type: TypePathExists, Path: "/nonexistent/zzz"},
		}},
		"empty": {Name: "empty", Type: TypeAny, Conditions: []Def{
			{Type: TypePathExists, Path: "/nonexistent/1"},
		}},
	}}

	if !e.Enabled("combo") {
		t.Fatal("expected any+all combo to match")
	}
	if !e.Enabled("nested") {
		t.Fatal("expected nested all+any to match")
	}
	if e.Enabled("fail") {
		t.Fatal("expected all with missing path to fail")
	}
	if e.Enabled("empty") {
		t.Fatal("expected any with all-false conditions to fail")
	}
}

func TestLegacyFallback(t *testing.T) {
	old := FeatureChecks
	defer func() { FeatureChecks = old }()
	FeatureChecks = map[string]func(param string) bool{
		"legacy-on":  func(string) bool { return true },
		"legacy-off": func(string) bool { return false },
	}
	e := &Engine{defs: map[string]Def{}}
	if !e.Enabled("legacy-on") {
		t.Fatal("expected legacy fallback match")
	}
	if e.Enabled("legacy-off") {
		t.Fatal("expected legacy fallback false")
	}
	if e.Enabled("unknown") {
		t.Fatal("expected unknown feature to be false")
	}
}

func TestParamLegacyFallback(t *testing.T) {
	old := FeatureChecks
	defer func() { FeatureChecks = old }()
	FeatureChecks = map[string]func(param string) bool{
		"pci": func(param string) bool { return param == "1002:699f" },
	}
	e := &Engine{defs: map[string]Def{}}
	if !e.Enabled("pci:1002:699f") {
		t.Fatal("expected param legacy match")
	}
	if e.Enabled("pci:10de:1234") {
		t.Fatal("expected param legacy false")
	}
	if e.Enabled("pci") {
		t.Fatal("expected bare param name to be unknown (no param legacy without colon)")
	}
	if e.Enabled("nousb:1234") {
		t.Fatal("expected unknown param base to be false")
	}
}

// TestLoadEmbeddedDeclarations verifies the embedded features.yaml parses
// into valid, non-empty declarations. It intentionally does not assert
// specific feature names - the declaration set grows over time, and the
// format must not.
func TestLoadEmbeddedDeclarations(t *testing.T) {
	defs := Load()
	if len(defs) == 0 {
		t.Fatal("expected embedded features.yaml to declare features")
	}
	// Load() already validates each declaration; ensure the name index
	// is filled consistently so Enabled() lookups work.
	for name, def := range defs {
		if def.Name != name {
			t.Fatalf("feature index mismatch: def.Name %q != key %q", def.Name, name)
		}
	}
}

func boolPtr(b bool) *bool { return &b }

// TestReadFailureFailClosed verifies that a failed file read with
// on_error unset (default false) returns false - it must NOT fall through
// to match empty content (regression: readFileWithFallback returned
// (nil, false) and callers kept matching against empty data).
func TestReadFailureFailClosed(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	e := &Engine{defs: map[string]Def{
		// on_error unset: read failure must yield false, even though the
		// empty string would match if the nil data were processed.
		"x": {Name: "x", Type: TypeFileValueIn, Path: missing, Values: []string{""}},
		"y": {Name: "y", Type: TypeFileContains, Path: missing, Contains: []string{""}},
	}}
	if e.Enabled("x") {
		t.Fatal("file_value_in with missing file must fail-closed (false), not match empty value")
	}
	if e.Enabled("y") {
		t.Fatal("file_contains with missing file must fail-closed (false), not match empty substring")
	}
}
