package rules

import (
	"deepin-power-control/internal/features"
	"deepin-power-control/internal/global"
)

// ConfigFragment is a TLP config fragment produced by a matched config
// action. It is merged by the config package, not executed.
type ConfigFragment struct {
	Source string            // rule file the fragment came from (logging)
	Items  map[string]string // TLP key=value pairs
}

// MatchedRules loads all rule files and returns the rules matching the
// given mode, in execution order. Matching runs exactly once per mode
// switch; the result is passed to CollectConfigFragments and
// ExecuteActions, so feature evaluation and its logs are not repeated.
func MatchedRules(mode string, engine *features.Engine) []*Rule {
	rules := LoadRules(global.RulesConfigDir)
	if len(rules) == 0 {
		global.Debug("No rules loaded from %s", global.RulesConfigDir)
		return nil
	}

	var matched []*Rule
	for _, r := range rules {
		if r.Matches(mode, engine) {
			matched = append(matched, r)
		}
	}
	return matched
}

// CollectConfigFragments extracts the config fragments from the matched
// rules, in order. Fragments are merged later (by the caller) so later
// rules override earlier ones. This runs BEFORE the TLP config merge.
func CollectConfigFragments(matched []*Rule) []ConfigFragment {
	var fragments []ConfigFragment
	for _, r := range matched {
		if r.Config == nil {
			continue
		}
		fragments = append(fragments, ConfigFragment{
			Source: trimSource(r.sourceFile()),
			Items:  map[string]string(*r.Config),
		})
	}
	if len(fragments) > 0 {
		global.Info("Collected %d config fragment(s) from rules", len(fragments))
	}
	return fragments
}

// ExecuteActions runs the write/exec actions of the matched rules.
// Config actions are ignored here (they were already collected and
// merged). This runs AFTER the TLP config merge, so actions can rely on
// TLP being applied.
func ExecuteActions(matched []*Rule) {
	if len(matched) == 0 {
		return
	}

	applied := 0
	for _, r := range matched {
		var action Action
		switch {
		case r.Write != nil:
			action = r.Write
		case r.Exec != nil:
			action = r.Exec
		}
		if action == nil {
			continue
		}

		global.Info("Applying rule from %s: %s", trimSource(r.sourceFile()), action.Describe())
		if err := action.Apply(); err != nil {
			global.Warn("Rule from %s failed: %v", trimSource(r.sourceFile()), err)
			continue
		}
		applied++
	}
	global.Info("Actions applied: %d/%d", applied, len(matched))
}
