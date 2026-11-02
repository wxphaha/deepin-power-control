// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package rules

import (
	"os"
	"path/filepath"
	"testing"

	"deepin-power-control/internal/features"

	"gopkg.in/yaml.v2"
)

// TestRuleTestdataParse guards the rule format contract: every yaml file
// under testdata/ must parse and validate. These files exercise boundary
// cases (no match, mode-only, feature forms, mixed actions). If a future
// change breaks the format, this test fails before any runtime issue.
func TestRuleTestdataParse(t *testing.T) {
	files, err := filepath.Glob("testdata/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no testdata yaml files found")
	}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}

		var fileRules []*Rule
		if err := yaml.Unmarshal(data, &fileRules); err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		if len(fileRules) == 0 {
			t.Fatalf("parse %s: expected at least one rule", file)
		}
		for _, r := range fileRules {
			if r == nil {
				continue
			}
			if err := r.validate(); err != nil {
				t.Fatalf("validate %s: %v", file, err)
			}
		}
	}
}

// TestRuleTestdataSemantics asserts the behavioral contract of the
// boundary-case files, so a future semantic change is caught.
func TestRuleTestdataSemantics(t *testing.T) {
	engine := features.New() // real engine; nil would panic on feature evaluation

	// 01-no-match.yaml: rule without match must match every mode.
	rules := loadTestdata(t, "01-no-match.yaml")
	for _, mode := range []string{"performance", "balance", "saving", "lowbat"} {
		if !rules[0].Matches(mode, engine) {
			t.Fatalf("no-match rule should match %s", mode)
		}
	}

	// 02-mode-only.yaml: rule with mode balance must only match balance.
	rules = loadTestdata(t, "02-mode-only.yaml")
	if !rules[0].Matches("balance", engine) {
		t.Fatal("mode-only rule should match balance")
	}
	if rules[0].Matches("performance", engine) {
		t.Fatal("mode-only rule should NOT match performance")
	}

	// 03-feature-forms.yaml: four rules, each with one config key.
	rules = loadTestdata(t, "03-feature-forms.yaml")
	if len(rules) != 4 {
		t.Fatalf("expected 4 rules, got %d", len(rules))
	}
	for i, r := range rules {
		if r.Config == nil {
			t.Fatalf("rule %d should be a config action", i)
		}
		if len(*r.Config) != 1 {
			t.Fatalf("rule %d should have exactly 1 config item, got %v", i, *r.Config)
		}
	}

	// 04-mode-list.yaml: mode list OR semantics.
	rules = loadTestdata(t, "04-mode-list.yaml")
	if !rules[0].Matches("balance", engine) || !rules[0].Matches("saving", engine) {
		t.Fatal("mode list should match balance and saving")
	}
	if rules[0].Matches("performance", engine) {
		t.Fatal("mode list should NOT match performance")
	}

	// 05-mixed-actions.yaml: one rule per action type.
	rules = loadTestdata(t, "05-mixed-actions.yaml")
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}
	if rules[0].Config == nil || rules[1].Write == nil || rules[2].Exec == nil {
		t.Fatal("expected config/write/exec rules in order")
	}
}

// loadTestdata parses a single testdata file and returns its rules.
func loadTestdata(t *testing.T, name string) []*Rule {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var rules []*Rule
	if err := yaml.Unmarshal(data, &rules); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return rules
}
