package rules

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"deepin-power-control/internal/features"
	"deepin-power-control/internal/global"

	"gopkg.in/yaml.v2"
)

// ---------- StringList YAML parsing ----------

func TestStringListSingleValue(t *testing.T) {
	var r Rule
	data := []byte("match:\n  mode: balance\nwrite:\n  path: /sys/test\n  value: 1\n")
	if err := yamlUnmarshal(data, &r); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !reflect.DeepEqual([]string(r.Match.Mode), []string{"balance"}) {
		t.Fatalf("expected [balance], got %v", r.Match.Mode)
	}
}

func TestStringListMultipleValues(t *testing.T) {
	var r Rule
	data := []byte("match:\n  mode: [balance, saving]\nwrite:\n  path: /sys/test\n  value: 1\n")
	if err := yamlUnmarshal(data, &r); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !reflect.DeepEqual([]string(r.Match.Mode), []string{"balance", "saving"}) {
		t.Fatalf("expected [balance saving], got %v", r.Match.Mode)
	}
}

// ---------- FeatureSet YAML forms ----------

func TestFeatureSetSingleForm(t *testing.T) {
	var r Rule
	data := []byte("match:\n  feature: portable\nwrite:\n  path: /sys/test\n  value: 1\n")
	if err := yamlUnmarshal(data, &r); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if _, ok := r.Match.Feature["portable"]; !ok {
		t.Fatalf("expected feature portable, got %v", r.Match.Feature)
	}
	if len(r.Match.Feature) != 1 {
		t.Fatalf("expected 1 feature, got %v", r.Match.Feature)
	}
}

func TestFeatureSetListForm(t *testing.T) {
	var r Rule
	data := []byte("match:\n  feature: [portable, zhaoxin]\nwrite:\n  path: /sys/test\n  value: 1\n")
	if err := yamlUnmarshal(data, &r); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(r.Match.Feature) != 2 {
		t.Fatalf("expected 2 features, got %v", r.Match.Feature)
	}
	if _, ok := r.Match.Feature["portable"]; !ok {
		t.Fatal("expected portable in feature set")
	}
	if _, ok := r.Match.Feature["zhaoxin"]; !ok {
		t.Fatal("expected zhaoxin in feature set")
	}
}

func TestFeatureSetMapFormWithParams(t *testing.T) {
	var r Rule
	data := []byte("match:\n  feature:\n    pci_vendor_device: [\"1002:6611\", \"1002:699f\"]\nwrite:\n  path: /sys/test\n  value: 1\n")
	if err := yamlUnmarshal(data, &r); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	params, ok := r.Match.Feature["pci_vendor_device"]
	if !ok {
		t.Fatalf("expected pci_vendor_device, got %v", r.Match.Feature)
	}
	if !reflect.DeepEqual(params, []string{"1002:6611", "1002:699f"}) {
		t.Fatalf("expected params, got %v", params)
	}
}

// ---------- Matches ----------

// testEngine uses the code-registry fallback with mock detectors.
// FeatureChecks is package-global but tests run in isolated processes per
// package, so swapping it here does not affect other packages.
func testEngine(t *testing.T) *features.Engine {
	t.Helper()
	old := features.FeatureChecks
	features.FeatureChecks = map[string]func(param string) bool{
		"mock-present": func(string) bool { return true },
		"mock-absent":  func(string) bool { return false },
	}
	t.Cleanup(func() { features.FeatureChecks = old })
	return features.New()
}

func TestMatchesModeAndFeature(t *testing.T) {
	r := &Rule{Match: Match{Mode: StringList{"balance"}, Feature: FeatureSet{"mock-present": nil}}}

	if !r.Matches("balance", testEngine(t)) {
		t.Fatal("expected match: balance + portable")
	}
	if r.Matches("performance", testEngine(t)) {
		t.Fatal("expected no match: wrong mode")
	}
	old := features.FeatureChecks
	features.FeatureChecks = map[string]func(param string) bool{
		"mock-present": func(string) bool { return false },
	}
	if r.Matches("balance", features.New()) {
		t.Fatal("expected no match: feature false")
	}
	features.FeatureChecks = old
}

func TestMatchesEmptyConditions(t *testing.T) {
	r := &Rule{Match: Match{}}
	if !r.Matches("any-mode", testEngine(t)) {
		t.Fatal("expected match: empty conditions match everything")
	}
}

func TestMatchesFeatureOR(t *testing.T) {
	r := &Rule{Match: Match{Feature: FeatureSet{"mock-absent": nil, "mock-present": nil}}}
	if !r.Matches("balance", testEngine(t)) {
		t.Fatal("expected match: portable in OR list")
	}

	r2 := &Rule{Match: Match{Feature: FeatureSet{"mock-absent": nil, "nonexistent": nil}}}
	if r2.Matches("balance", testEngine(t)) {
		t.Fatal("expected no match: all features false/unknown")
	}
}

func TestMatchesModeOR(t *testing.T) {
	r := &Rule{Match: Match{Mode: StringList{"balance", "saving"}}}
	if !r.Matches("saving", testEngine(t)) {
		t.Fatal("expected match: saving in OR list")
	}
}

// ---------- WriteAction ----------

// withSysfsRoot redirects sysfsRoot to a temp dir for the test duration.
func withSysfsRoot(t *testing.T) string {
	t.Helper()
	old := sysfsRoot
	root := t.TempDir()
	sysfsRoot = root
	t.Cleanup(func() { sysfsRoot = old })
	return root
}

func TestWriteActionApplyAndVerify(t *testing.T) {
	root := withSysfsRoot(t)
	target := filepath.Join(root, "devfreq0", "governor")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	w := &WriteAction{Path: filepath.Join(root, "devfreq*", "governor"), Value: "powersave"}
	if err := w.Apply(); err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "powersave" {
		t.Fatalf("expected powersave, got %q", string(data))
	}
}

func TestWriteActionWriteError(t *testing.T) {
	root := withSysfsRoot(t)
	// Target is a directory: writes fail for every user (EISDIR)
	target := filepath.Join(root, "devfreq0", "governor")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}

	w := &WriteAction{Path: target, Value: "powersave"}
	if err := w.Apply(); err == nil {
		t.Fatal("expected error on write failure")
	}
}

func TestWriteActionVerifyMismatch(t *testing.T) {
	root := withSysfsRoot(t)
	target := filepath.Join(root, "devfreq0", "governor")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("fixed"), 0644); err != nil {
		t.Fatal(err)
	}

	// Simulate the kernel accepting the write but normalizing the value
	// (e.g. clamping to a different string): the read-back differs, so
	// verification must fail.
	oldRead := readFile
	readFile = func(string) ([]byte, error) { return []byte("normalized"), nil }
	defer func() { readFile = oldRead }()

	w := &WriteAction{Path: target, Value: "powersave"}
	if err := w.Apply(); err == nil {
		t.Fatal("expected error on verify mismatch")
	}
}

func TestWriteActionVerifyWriteOnlyNode(t *testing.T) {
	root := withSysfsRoot(t)
	target := filepath.Join(root, "devfreq0", "trigger")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// Write-only sysfs nodes reject reads (EPERM/EACCES) even though the
	// write succeeded: read-back failure must be treated as success.
	oldRead := readFile
	readFile = func(string) ([]byte, error) { return nil, os.ErrPermission }
	defer func() { readFile = oldRead }()

	w := &WriteAction{Path: target, Value: "1"}
	if err := w.Apply(); err != nil {
		t.Fatalf("expected write-only node to pass without read-back: %v", err)
	}
}

func TestWriteActionVerifyTrimsValue(t *testing.T) {
	root := withSysfsRoot(t)
	target := filepath.Join(root, "devfreq0", "governor")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("powersave"), 0644); err != nil {
		t.Fatal(err)
	}

	// A YAML value with trailing whitespace must not make verification fail:
	// both sides are trimmed before comparison.
	w := &WriteAction{Path: target, Value: "powersave "}
	if err := w.Apply(); err != nil {
		t.Fatalf("expected trailing-space value to pass: %v", err)
	}
}

func TestWriteActionNoMatch(t *testing.T) {
	w := &WriteAction{Path: "/sys/nonexistent/path*/governor", Value: "powersave"}
	if err := w.Apply(); err != nil {
		t.Fatalf("expected nil on no glob match, got %v", err)
	}
}

func TestWriteActionValidate(t *testing.T) {
	w := &WriteAction{Path: "/etc/passwd", Value: "1"}
	if err := w.validate(); err == nil {
		t.Fatal("expected validation error for non-/sys path")
	}

	// Path traversal: /sys/../../etc/passwd cleans to /etc/passwd and must be rejected
	traversal := &WriteAction{Path: "/sys/../../etc/passwd", Value: "1"}
	if err := traversal.validate(); err == nil {
		t.Fatal("expected validation error for path traversal")
	}

	ok := &WriteAction{Path: "/sys/class/devfreq/devfreq*/governor", Value: "powersave"}
	if err := ok.validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestWriteActionSkipsSymlinkEscape(t *testing.T) {
	root := withSysfsRoot(t)
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim")
	if err := os.WriteFile(victim, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}

	// A symlink inside sysfsRoot pointing outside it
	link := filepath.Join(root, "devfreq0", "governor")
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}

	w := &WriteAction{Path: filepath.Join(root, "devfreq*", "governor"), Value: "hacked"}
	if err := w.Apply(); err != nil {
		t.Fatalf("expected symlink escape to be skipped, got error: %v", err)
	}

	data, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Fatalf("victim must stay untouched, got %q", string(data))
	}
}

// ---------- ExecAction ----------

func TestExecActionApply(t *testing.T) {
	e := &ExecAction{Command: "/bin/echo hello"}
	if err := e.Apply(); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
}

func TestExecActionFailure(t *testing.T) {
	e := &ExecAction{Command: "/bin/false"}
	if err := e.Apply(); err == nil {
		t.Fatal("expected error for failing command")
	}
}

func TestExecActionTimeout(t *testing.T) {
	old := execTimeout
	execTimeout = 200 * time.Millisecond
	defer func() { execTimeout = old }()

	e := &ExecAction{Command: "/bin/sleep 5"}
	if err := e.Apply(); err == nil {
		t.Fatal("expected error for timed-out command")
	}
}

// ---------- splitCommand ----------

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		in       string
		expected []string
	}{
		{"", nil},
		{"/bin/echo hello", []string{"/bin/echo", "hello"}},
		{"/bin/echo 'hello world'", []string{"/bin/echo", "hello world"}},
		{`/bin/echo "a b" c`, []string{"/bin/echo", "a b", "c"}},
		{"/bin/echo a\\ b", []string{"/bin/echo", "a b"}},
		{"/bin/sh -c 'echo x | cat'", []string{"/bin/sh", "-c", "echo x | cat"}},
		{"  /bin/echo   spaced  ", []string{"/bin/echo", "spaced"}},
	}
	for _, tt := range tests {
		got, err := splitCommand(tt.in)
		if err != nil {
			t.Fatalf("splitCommand(%q) error: %v", tt.in, err)
		}
		if !reflect.DeepEqual(got, tt.expected) {
			t.Fatalf("splitCommand(%q) = %v, want %v", tt.in, got, tt.expected)
		}
	}
}

func TestSplitCommandUnbalanced(t *testing.T) {
	if _, err := splitCommand("/bin/echo 'unclosed"); err == nil {
		t.Fatal("expected error for unbalanced quote")
	}
	if _, err := splitCommand("/bin/echo \\"); err == nil {
		t.Fatal("expected error for dangling escape")
	}
}

func TestSplitCommandNullByte(t *testing.T) {
	if _, err := splitCommand("/bin/echo \x00--dangerous"); err == nil {
		t.Fatal("expected error for null byte in command")
	}
}

// ---------- LoadRules ----------

// safeDir returns a temp dir that is never group/world-writable:
// t.TempDir() inherits the process umask (0775 under umask 002) and would
// be rejected by isSafeDir. Chmod 0700 explicitly.
func safeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadRules(t *testing.T) {
	dir := safeDir(t)
	ruleFile := filepath.Join(dir, "10-test.yaml")
	content := `
- match:
    mode: balance
  write:
    path: /sys/class/devfreq/devfreq*/governor
    value: powersave

- match:
    mode: performance
  exec:
    command: /bin/echo hello
`
	if err := os.WriteFile(ruleFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rules := LoadRules(dir)
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Write == nil || rules[0].Write.Value != "powersave" {
		t.Fatal("rule 0 should be write powersave")
	}
	if rules[1].Exec == nil {
		t.Fatal("rule 1 should be exec")
	}
}

func TestLoadRulesSkipsInvalid(t *testing.T) {
	dir := safeDir(t)
	badFile := filepath.Join(dir, "10-bad.yaml")
	content := `
- match:
    mode: balance
  write:
    path: /etc/evil
    value: 1

- match:
    mode: balance
`
	if err := os.WriteFile(badFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rules := LoadRules(dir)
	if len(rules) != 0 {
		t.Fatalf("expected 0 valid rules, got %d", len(rules))
	}
}

func TestLoadRulesSkipsNilEntries(t *testing.T) {
	dir := safeDir(t)
	// A stray "- " list item unmarshals to a nil *Rule and must not panic.
	content := `
- match:
    mode: balance
  exec:
    command: /bin/true

-

- match:
    mode: performance
  exec:
    command: /bin/false
`
	if err := os.WriteFile(filepath.Join(dir, "10-nil.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rules := LoadRules(dir)
	if len(rules) != 2 {
		t.Fatalf("expected 2 valid rules (nil entry skipped), got %d", len(rules))
	}
}

func TestLoadRulesMalformedYAML(t *testing.T) {
	dir := safeDir(t)
	if err := os.WriteFile(filepath.Join(dir, "10-broken.yaml"), []byte(": : :"), 0644); err != nil {
		t.Fatal(err)
	}
	if rules := LoadRules(dir); len(rules) != 0 {
		t.Fatalf("expected no rules from malformed file, got %d", len(rules))
	}
}

func TestLoadRulesSkipsGroupWritable(t *testing.T) {
	dir := safeDir(t)
	// 0664: group-writable file must be rejected even though YAML is valid.
	// Chmod explicitly: umask (e.g. 022 in build chroots) would strip the
	// group-write bit from WriteFile's perm argument.
	path := filepath.Join(dir, "10-insecure.yaml")
	content := "- match:\n    mode: balance\n  exec:\n    command: /bin/true\n"
	if err := os.WriteFile(path, []byte(content), 0664); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0664); err != nil {
		t.Fatal(err)
	}

	if rules := LoadRules(dir); len(rules) != 0 {
		t.Fatalf("expected insecure file to be skipped, got %d rules", len(rules))
	}
}

func TestOpenSafeRuleFile(t *testing.T) {
	dir := safeDir(t)
	secure := filepath.Join(dir, "secure.yaml")
	if err := os.WriteFile(secure, []byte("[]"), 0644); err != nil {
		t.Fatal(err)
	}
	f, err := openSafeRuleFile(secure)
	if err != nil {
		t.Fatalf("expected 0644 file to open safely: %v", err)
	}
	f.Close()

	groupWritable := filepath.Join(dir, "gw.yaml")
	if err := os.WriteFile(groupWritable, []byte("[]"), 0664); err != nil {
		t.Fatal(err)
	}
	// Chmod explicitly: umask would strip the group-write bit otherwise.
	if err := os.Chmod(groupWritable, 0664); err != nil {
		t.Fatal(err)
	}
	if _, err := openSafeRuleFile(groupWritable); err == nil {
		t.Fatal("expected group-writable file to be rejected")
	}

	worldWritable := filepath.Join(dir, "ww.yaml")
	if err := os.WriteFile(worldWritable, []byte("[]"), 0666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(worldWritable, 0666); err != nil {
		t.Fatal(err)
	}
	if _, err := openSafeRuleFile(worldWritable); err == nil {
		t.Fatal("expected world-writable file to be rejected")
	}

	missing := filepath.Join(dir, "missing.yaml")
	if _, err := openSafeRuleFile(missing); err == nil {
		t.Fatal("expected missing file to be rejected")
	}
}

func TestOpenSafeRuleFileRejectsSymlink(t *testing.T) {
	dir := safeDir(t)
	target := filepath.Join(dir, "real.yaml")
	if err := os.WriteFile(target, []byte("[]"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := openSafeRuleFile(link); err == nil {
		t.Fatal("expected symlink rule file to be rejected")
	}
}

func TestLoadRulesSkipsUnsafeBaseDir(t *testing.T) {
	dir := t.TempDir()
	content := "- match:\n    mode: balance\n  exec:\n    command: /bin/true\n"
	if err := os.WriteFile(filepath.Join(dir, "10-valid.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	// Chmod explicitly: umask would strip the group-write bit otherwise.
	if err := os.Chmod(dir, 0775); err != nil {
		t.Fatal(err)
	}

	if rules := LoadRules(dir); len(rules) != 0 {
		t.Fatalf("expected rules from group-writable base dir to be rejected, got %d", len(rules))
	}
}

func TestLoadRulesSkipsUnsafeSubdir(t *testing.T) {
	root := safeDir(t)
	subdir := filepath.Join(root, "oem")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "- match:\n    mode: balance\n  exec:\n    command: /bin/true\n"
	// Valid rule in the unsafe subdirectory: must be skipped
	if err := os.WriteFile(filepath.Join(subdir, "10-insecure.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(subdir, 0775); err != nil {
		t.Fatal(err)
	}
	// Valid rule in the safe root: must still load
	if err := os.WriteFile(filepath.Join(root, "10-valid.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rules := LoadRules(root)
	if len(rules) != 1 {
		t.Fatalf("expected only the rule from the safe dir to load, got %d", len(rules))
	}
	if !strings.HasSuffix(rules[0].sourceFile(), "10-valid.yaml") {
		t.Fatalf("expected 10-valid.yaml to be loaded, got %s", rules[0].sourceFile())
	}
}

func TestLoadRulesNonYAMLIgnored(t *testing.T) {
	dir := safeDir(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("not a rule"), 0644); err != nil {
		t.Fatal(err)
	}
	if rules := LoadRules(dir); len(rules) != 0 {
		t.Fatalf("expected no rules from .md, got %d", len(rules))
	}
}

// ---------- ApplyRules end-to-end ----------

// TestApplyRulesEndToEnd verifies the full load -> match -> execute chain.
// Uses exec rules (write rules are restricted to /sys/ and already covered by unit tests).
func TestApplyRulesEndToEnd(t *testing.T) {
	old := global.RulesConfigDir
	defer func() { global.RulesConfigDir = old }()

	dir := safeDir(t)
	global.RulesConfigDir = dir

	marker := filepath.Join(dir, "executed.marker")
	content := `
- match:
    mode: balance
  exec:
    command: /bin/touch ` + marker + `

- match:
    mode: performance
  exec:
    command: /bin/false
`
	if err := os.WriteFile(filepath.Join(dir, "10-test.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	matched := MatchedRules("balance", features.New())
	ExecuteActions(matched)

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected balance rule to execute and create marker: %v", err)
	}
}

// yamlUnmarshal parses YAML directly into a rule struct
func yamlUnmarshal(data []byte, r *Rule) error {
	return yaml.Unmarshal(data, r)
}

// ---------- Real rule file format validation ----------

// TestRuleFilesParse verifies every rule file shipped in rules/ parses and
// validates structurally (exactly one action, valid match/action fields).
// It intentionally does NOT assert specific key/value content - rule
// content changes over time, format must not.
func TestRuleFilesParse(t *testing.T) {
	matches, err := filepath.Glob("../../rules/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no rule files found in ../../rules/")
	}

	// Known feature names: declared in features.yaml + code registry.
	// A rule referencing an unknown feature silently never matches, so
	// this format check catches typos/mismatches early.
	known := make(map[string]bool)
	for name := range features.Load() {
		known[name] = true
	}
	for name := range features.FeatureChecks {
		known[name] = true
	}

	for _, file := range matches {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}

		var fileRules []*Rule
		if err := yaml.Unmarshal(data, &fileRules); err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}

		for _, r := range fileRules {
			if r == nil {
				continue
			}
			if err := r.validate(); err != nil {
				t.Fatalf("validate %s: %v", file, err)
			}
			for name := range r.Match.Feature {
				if !known[name] {
					t.Fatalf("rule %s references unknown feature %q", file, name)
				}
			}
		}
	}
}
