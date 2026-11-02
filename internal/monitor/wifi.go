// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package monitor

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"deepin-power-control/internal/global"
	"deepin-power-control/internal/service"
	"deepin-power-control/internal/utils"
)

type PowerState int

const (
	StateUnknown PowerState = iota
	StateOn
	StateOff
)

type NetSample struct {
	TotalBytes uint64
	Time       time.Time
	Details    map[string][2]uint64 // interface -> [rx, tx]
}

func (s PowerState) String() string {
	switch s {
	case StateOn:
		return "POWER-SAVE (ON)"
	case StateOff:
		return "PERFORMANCE (OFF)"
	case StateUnknown:
		return "UNKNOWN"
	default:
		return "INVALID"
	}
}

// Action represents the decision made by the engine
type Action struct {
	NextState    PowerState
	NextInterval time.Duration
	IsSpike      bool
	Reason       string
}

func RunWifiEngine() error {
	ctx, err := service.StartDaemonize()
	if err != nil {
		return err
	}

	global.Info("WiFi monitor daemon started")
	currentState := StateUnknown

	defer func() {
		global.Info("WiFi monitor exiting. Restoring performance mode...")
		if currentState != StateOff {
			_ = setPowerSave(getValidTargets(), "off")
		}
	}()

	// Load initial configs
	minInterval := time.Duration(global.AppConfig.MinCheckInterval) * time.Second
	maxInterval := time.Duration(global.AppConfig.MaxCheckInterval) * time.Second
	threshold := float64(global.AppConfig.WifiThreshold)
	multiplier := float64(global.AppConfig.WifiMultiplier)

	currentInterval := minInterval
	lastSample := getNetSample(getValidTargets())
	lastRate := 0.0

	timer := time.NewTimer(currentInterval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-timer.C:
			targets := getValidTargets()
			if len(targets) == 0 {
				currentInterval = maxInterval
				timer.Reset(currentInterval)
				continue
			}

			currentSample := getNetSample(targets)
			duration := currentSample.Time.Sub(lastSample.Time).Seconds()
			if duration <= 0 {
				duration = 1
			}

			rate := float64(currentSample.TotalBytes-lastSample.TotalBytes) / 1024.0 / duration

			// --- Core Decision Logic (Decoupled) ---
			action := decideNextAction(rate, lastRate, currentState, currentInterval, minInterval, maxInterval, threshold, multiplier)

			// Execute IO Side Effects
			stateChanged := false
			if action.NextState != currentState && action.NextState != StateUnknown {
				modeStr := "on"
				if action.NextState == StateOff {
					modeStr = "off"
				}

				if err := setPowerSave(targets, modeStr); err != nil {
					global.Warn("Failed to set %s: %v", modeStr, err)
					currentState = StateUnknown
				} else {
					currentState = action.NextState
					stateChanged = true
				}
			}

			// Logging
			logFunc := global.Debug
			if stateChanged || action.IsSpike {
				logFunc = global.Info
			}
			logFunc("MONITOR: Rate:%.2fKB/s, State:%v, NextInterval:%v, Spike:%v, Details:[%s]",
				rate, currentState, action.NextInterval, action.IsSpike, formatDiffDetails(lastSample, currentSample))

			// Update state for next iteration
			lastSample = currentSample
			lastRate = rate
			currentInterval = action.NextInterval
			timer.Reset(currentInterval)
		}
	}
}

// decideNextAction is a pure function that calculates the next state and interval.
// It is the "Brain" of the monitor and is easily testable.
func decideNextAction(rate, lastRate float64, current PowerState, currentInterval, min, max time.Duration, threshold, multiplier float64) Action {
	action := Action{
		NextState:    current,
		NextInterval: currentInterval,
		IsSpike:      lastRate > 0 && rate > (lastRate*10) && rate > 10,
	}

	switch {
	case rate > (threshold * multiplier):
		action.NextState = StateOff
		action.NextInterval = min
		action.Reason = "High traffic"

	case rate < threshold:
		action.NextState = StateOn
		if current == StateOn {
			action.NextInterval = growInterval(currentInterval, max)
		} else {
			action.NextInterval = min // Start backing off from min when entering power-save
		}
		action.Reason = "Low traffic"

	default:
		action.Reason = "Normal traffic"
	}

	// Spikes override interval to ensure responsiveness
	if action.IsSpike {
		action.NextInterval = min
	}

	return action
}

func growInterval(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}

// --- IO Helpers---
func getNetSample(targets []string) NetSample {
	sample := NetSample{Time: time.Now(), Details: make(map[string][2]uint64)}
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return sample
	}
	defer file.Close()

	targetMap := make(map[string]bool)
	for _, t := range targets {
		targetMap[t] = true
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		name := strings.TrimSpace(parts[0])
		if targetMap[name] {
			fields := strings.Fields(parts[1])
			if len(fields) >= 9 {
				rx, _ := strconv.ParseUint(fields[0], 10, 64)
				tx, _ := strconv.ParseUint(fields[8], 10, 64)
				sample.TotalBytes += (rx + tx)
				sample.Details[name] = [2]uint64{rx, tx}
			}
		}
	}
	return sample
}

// formatDiffDetails creates a human-readable diff: "wlan0(rx+10KB,tx+2KB)"
func formatDiffDetails(old, new NetSample) string {
	var parts []string
	for iface, newVals := range new.Details {
		oldVals, ok := old.Details[iface]
		if !ok {
			parts = append(parts, fmt.Sprintf("%s(new)", iface))
			continue
		}
		rxDiff := (newVals[0] - oldVals[0]) / 1024
		txDiff := (newVals[1] - oldVals[1]) / 1024
		parts = append(parts, fmt.Sprintf("%s(rx+%dKB,tx+%dKB)(cur r:%dKB,t:%dKB)", iface,
			rxDiff, txDiff, newVals[0]/1024, newVals[1]/1024))
	}
	return strings.Join(parts, "; ")
}

// getValidTargets scans /sys/class/net to find wireless interfaces not in the ignore list.
func getValidTargets() []string {
	var targets []string
	ignoreMap := make(map[string]bool)
	for _, d := range global.AppConfig.WifiIgnoreDevices {
		ignoreMap[d] = true
	}
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil
	}
	for _, e := range entries {
		name := e.Name()
		if ignoreMap[name] {
			continue
		}
		// Wireless devices usually have a 'wireless' directory or 'phy80211' link.
		// Also fallback to common naming conventions.
		isWifi := false
		if _, err := os.Stat(fmt.Sprintf("/sys/class/net/%s/wireless", name)); err == nil {
			isWifi = true
		} else if strings.HasPrefix(name, "wlan") || strings.HasPrefix(name, "wlp") {
			isWifi = true
		}
		if isWifi {
			targets = append(targets, name)
		}
	}
	return targets
}

// setPowerSave executes the hardware command and returns an error if any interface fails.
func setPowerSave(ifaces []string, mode string) error {
	var errs []string
	for _, iface := range ifaces {
		status := "success"
		err := utils.RunCmd(global.CmdIw, "dev", iface, "set", "power_save", mode)
		if err != nil {
			status = "fail"
			errs = append(errs, fmt.Sprintf("%s: %v", iface, err))
		}
		global.Info("Set %s power_save %s %s", iface, mode, status)
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed: %s", strings.Join(errs, ", "))
	}
	return nil
}

func StopWifiEngine() error {
	return service.StopDaemonize()
}
