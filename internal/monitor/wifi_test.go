// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package monitor

import (
	"strings"
	"testing"
	"time"
)

// TestGrowInterval verifies the exponential backoff logic
func TestGrowInterval(t *testing.T) {
	min := 30 * time.Second
	max := 300 * time.Second

	tests := []struct {
		current  time.Duration
		expected time.Duration
	}{
		{min, 60 * time.Second},
		{60 * time.Second, 120 * time.Second},
		{200 * time.Second, 300 * time.Second}, // Should cap at max
		{max, max},                             // Stay at max
	}

	for _, tt := range tests {
		got := growInterval(tt.current, max)
		if got != tt.expected {
			t.Errorf("growInterval(%v) = %v; want %v", tt.current, got, tt.expected)
		}
	}
}

// TestFormatDiffDetails verifies the human-readable string generation
func TestFormatDiffDetails(t *testing.T) {
	old := NetSample{
		Details: map[string][2]uint64{
			"wlan0": {100 * 1024, 50 * 1024},
		},
	}
	new := NetSample{
		Details: map[string][2]uint64{
			"wlan0": {110 * 1024, 52 * 1024},
		},
	}

	result := formatDiffDetails(old, new)

	// Check if it contains expected diffs (rx+10KB, tx+2KB)
	if !strings.Contains(result, "rx+10KB") || !strings.Contains(result, "tx+2KB") {
		t.Errorf("Format output incorrect: %s", result)
	}
}

func TestDecideNextAction(t *testing.T) {
	min := 30 * time.Second
	max := 300 * time.Second
	threshold := 100.0
	multiplier := 3.0 // Performance starts at 300KB/s

	tests := []struct {
		name        string
		rate        float64
		lastRate    float64
		current     PowerState
		interval    time.Duration
		expectState PowerState
		expectInter time.Duration
		expectSpike bool
	}{
		{
			name:        "Enter Performance Mode",
			rate:        350.0,
			lastRate:    340.0,
			current:     StateOn,
			interval:    max,
			expectState: StateOff,
			expectInter: min,
			expectSpike: false,
		},
		{
			name:        "Exponential Backoff in Power-Save",
			rate:        10.0,
			lastRate:    10.0,
			current:     StateOn,
			interval:    min,
			expectState: StateOn,
			expectInter: 60 * time.Second, // min * 2
			expectSpike: false,
		},
		{
			name:        "Traffic Spike Detection",
			rate:        250.0, // Not enough for performance (300) but it's a spike (10->250)
			lastRate:    10.0,
			current:     StateOn,
			interval:    max,
			expectState: StateOn,
			expectInter: min, // Interval reset to min due to spike
			expectSpike: true,
		},
		{
			name:        "Low Traffic Noise - No Spike",
			rate:        5.0,
			lastRate:    0.1,
			current:     StateOn,
			interval:    min,
			expectState: StateOn,
			expectInter: 60 * time.Second,
			expectSpike: false, // Multiplier is 50x but rate < 10KB/s
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := decideNextAction(tt.rate, tt.lastRate, tt.current, tt.interval, min, max, threshold, multiplier)

			if action.NextState != tt.expectState {
				t.Errorf("State: got %v, want %v", action.NextState, tt.expectState)
			}
			if action.NextInterval != tt.expectInter {
				t.Errorf("Interval: got %v, want %v", action.NextInterval, tt.expectInter)
			}
			if action.IsSpike != tt.expectSpike {
				t.Errorf("Spike: got %v, want %v", action.IsSpike, tt.expectSpike)
			}
		})
	}
}
