// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package debugger

import (
	"encoding/json"
	"os"
	"path/filepath"

	"deepin-power-control/internal/global"
)

// Range defines the start and end indices in the AllItems slice.
type Range struct {
	Low  int `json:"low"`
	High int `json:"high"`
}

// IsSingle returns true if the range contains only one item.
func (r Range) IsSingle() bool {
	return r.Low == r.High
}

// Mid calculates the middle index of the range.
func (r Range) Mid() int {
	return (r.Low + r.High) / 2
}

// Culprit represents a problematic configuration found by the engine.
type Culprit struct {
	Items   []ConfigPair `json:"items"`
	IsCombo bool         `json:"is_combo"`
}

// MinContext stores the specific state for the minimization phase.
type MinContext struct {
	OriginalItems []ConfigPair `json:"original_items"` // The set that triggered the combo
	CurrentItems  []ConfigPair `json:"current_items"`  // The items remaining in the minimized set
	Index         int          `json:"index"`          // Current item index being tested for removal
}

// DebugState is the main persistence object for the debugging session.
type DebugState struct {
	// Static Configuration
	AllItems []ConfigPair `json:"all_items"` // Flattened original configuration pairs
	Target   string       `json:"target"`    // The sysfs path to probe
	Expect   string       `json:"expect"`    // The expected value for a "PASS" result

	// Dynamic Search Progress
	CurrentRange Range   `json:"current_range"` // Current range being processed
	PendingStack []Range `json:"pending_stack"` // Explicit stack for DFS traversal
	Phase        string  `json:"phase"`         // "BINARY" or "MINIMIZE"

	// Results and Safety
	FoundCulprits    []Culprit `json:"found_culprits"`
	RebootRetryCount uint      `json:"reboot_retry_count"`

	// Minimization context (only used when Phase == "MINIMIZE")
	MinCtx *MinContext `json:"min_ctx,omitempty"`
}

// Save serializes the state to global.StateFile.
func (s *DebugState) Save() error {
	dir := filepath.Dir(global.StateFile)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(global.StateFile, data, 0644)
}

// LoadState reads the state from global.StateFile.
func LoadState() (*DebugState, error) {
	data, err := os.ReadFile(global.StateFile)
	if err != nil {
		return nil, err
	}

	var state DebugState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// ClearState removes the persistent state file.
func ClearState() error {
	if _, err := os.Stat(global.StateFile); err == nil {
		return os.Remove(global.StateFile)
	}
	return nil
}

// PushRange adds a new range to the pending stack.
func (s *DebugState) PushRange(r Range) {
	s.PendingStack = append(s.PendingStack, r)
}

// PopRange removes and returns the last range from the stack.
// Returns a zero Range and false if stack is empty.
func (s *DebugState) PopRange() (Range, bool) {
	if len(s.PendingStack) == 0 {
		return Range{}, false
	}
	lastIdx := len(s.PendingStack) - 1
	r := s.PendingStack[lastIdx]
	s.PendingStack = s.PendingStack[:lastIdx]
	return r, true
}
