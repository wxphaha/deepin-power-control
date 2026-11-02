// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package rules

import (
	"slices"

	"deepin-power-control/internal/features"
	"deepin-power-control/internal/global"
)

// Matches evaluates whether the rule matches the given mode.
// Semantics:
//   - the mode and feature fields are ANDed: all must match
//   - feature keys are ORed; parameters within a key are ORed
//   - empty fields are unrestricted
func (r *Rule) Matches(mode string, engine *features.Engine) bool {
	if len(r.Match.Mode) > 0 && !slices.Contains(r.Match.Mode, mode) {
		global.Debug("Rule %s skipped: mode %s not in %v", r.sourceFile(), mode, r.Match.Mode)
		return false
	}

	if len(r.Match.Feature) > 0 {
		matched := false
		for name, params := range r.Match.Feature {
			if len(params) == 0 {
				if engine.Enabled(name) {
					matched = true
					break
				}
				continue
			}
			for _, param := range params {
				if engine.EnabledParam(name, param) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			global.Debug("Rule %s skipped: no feature matched %v", r.sourceFile(), r.Match.Feature)
			return false
		}
	}

	return true
}
