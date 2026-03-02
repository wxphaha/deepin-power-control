package debugger

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"deepin-power-control/internal/global"
	"deepin-power-control/internal/utils"
)

type Engine struct {
	Probe      Checker
	tempConfig string
}

func NewEngine(probe Checker) *Engine {
	return &Engine{
		Probe:      probe,
		tempConfig: filepath.Join(global.TlpConfigDir, "99_dspc_debugtmp.conf"),
	}
}

func backupTlp() error {
	if _, err := os.Stat(global.CmdTlp); err == nil {
		global.Info("Moving tlp to hijack system calls")
		if err := os.Rename(global.CmdTlp, global.CmdTlpDebug); err != nil {
			return fmt.Errorf("failed to rename tlp: %v", err)
		}
		return nil
	} else {
		return err
	}
}

func restoreTlp() error {
	if _, err := os.Stat(global.CmdTlpDebug); err == nil {
		global.Info("Restore tlp")
		if err := os.Rename(global.CmdTlpDebug, global.CmdTlp); err != nil {
			return fmt.Errorf("failed to rename tlp: %v", err)
		}
	}
	return nil
}

// Init handles 'debug init' and performs a pre-check.
func (e *Engine) Init(target, expect string) error {
	if err := backupTlp(); err != nil {
		return fmt.Errorf("Cann't backup tlp")
	}
	if err := os.Remove(e.tempConfig); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Warning: Failed to remove temp config: %v", err)
	}

	state := &DebugState{
		Target: target,
		Expect: expect,
		Phase:  "PRECHECK",
	}

	global.Warn("Initial check failed. Environment prepared. System will reboot to start debugging.")
	if err := PrepareSystemForReboot(); err != nil {
		return err
	}

	return e.reboot(state)
}

// waitCheckOK polls the probe and ensures the state is stable for consecutive seconds.
func (e *Engine) waitCheckOK(timeoutSec int) error {
	var lastErr error
	consecutiveOK := 0
	const requiredOK = 5 // Must be stable for 5 seconds

	global.Info("Waiting for environment to settle (timeout: %ds, required stable: %ds)...", timeoutSec, requiredOK)

	for i := 1; i <= timeoutSec; i++ {
		lastErr = e.Probe.Check()

		if lastErr == nil {
			consecutiveOK++
			global.Debug("Check OK (%d/%d)...", consecutiveOK, requiredOK)
		} else {
			if consecutiveOK > 0 {
				global.Debug("State jitter detected! Resetting stability counter.")
			}
			consecutiveOK = 0
		}

		// Only return nil if we have been stable for the required duration
		if consecutiveOK >= requiredOK {
			global.Info("Environment settled and stable after %ds.", i)
			return nil
		}

		// Log context if we are still failing
		if (i == 1 || i%5 == 0 || i == timeoutSec) && lastErr != nil {
			global.Debug("Waiting... Attempt %d/%d: %v", i, timeoutSec, lastErr)
		}

		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("timeout: environment failed to stabilize within %ds (last error: %v)", timeoutSec, lastErr)
}

// Resume is the state machine executor called after reboot.
func (e *Engine) Resume() error {
	state, err := LoadState()
	if err != nil {
		return fmt.Errorf("failed to load state after reboot: %v", err)
	}

	e.Probe = &SysfsProbe{
		Path:   state.Target,
		Expect: state.Expect,
	}

	global.Info("=== Debug Cycle [Reboot #%d] | Phase: %s | Range: [%d,%d] ===", state.RebootRetryCount, state.Phase, state.CurrentRange.Low, state.CurrentRange.High)
	if err = e.waitCheckOK(60); err != nil {
		return fmt.Errorf("environment corrupted: %v", err)
	}

	for {
		if e.isFinished(state) {
			global.Info("Finished.")
			e.finalize(state, nil)
			return nil
		}

		var (
			mustReboot bool
			err        error
		)
		switch state.Phase {
		case "PRECHECK":
			mustReboot, err = e.handlePrecheckStep(state)
		case "BINARY":
			mustReboot, err = e.handleBinaryStep(state)
		case "MINIMIZE":
			mustReboot, err = e.handleMinimizeStep(state)
		default:
			return fmt.Errorf("unknown debug phase: %s", state.Phase)
		}

		if err != nil {
			e.finalize(state, err)
			return err
		}

		if err := state.Save(); err != nil {
			return fmt.Errorf("failed to save state during loop: %v", err)
		}

		if mustReboot {
			return e.reboot(state)
		}
		global.Info("Step passed. Moving to next calculation within current session...")
	}
}

func (e *Engine) Abort() {
	e.finalize(nil, nil)
}

func (e *Engine) setSystemPowerMode(mode string) {
	global.Info("Attempting to switch system power mode to: %s", mode)

	// 1. Define possible control binaries
	controls := []string{
		"/sbin/deepin-power-control",
		"/sbin/deepin-system-power-control",
	}

	executed := false
	for _, bin := range controls {
		if _, err := os.Stat(bin); err == nil {
			global.Debug("Executing %s set %s", bin, mode)
			// We ignore the error here because the binary might "fail"
			// due to the missing (hijacked) /usr/sbin/tlp.
			_ = utils.RunCmd(bin, "set", mode)
			executed = true
			break
		}
	}

	if !executed {
		global.Warn("No power control binary found. Mode switch might have failed.")
	}
}

func (e *Engine) handlePrecheckStep(state *DebugState) (bool, error) {
	global.Info(">>> Phase [PRECHECK]: Verifying system behavior...")

	// 1. Zero config check (Must be PASS)
	passed, err := e.testSet(nil)
	if err != nil || !passed {
		return false, fmt.Errorf("zero config check failed (system unstable or already broken): %v", err)
	}

	// 2. Iterate modes to find the trigger
	modes := []string{"performance", "balanced", "power-saver", "supersaver"}
	var faultyMode string
	for _, m := range modes {
		global.Info("Testing Mode: %s", m)
		e.setSystemPowerMode(m)

		_ = utils.RunCmd(global.CmdTlpDebug, "init", "start")
		time.Sleep(1 * time.Second)

		if err := e.Probe.Check(); err != nil {
			global.Warn("Found faulty mode: %s | Reason: %v", m, err)
			faultyMode = m
			break
		}
	}

	if faultyMode == "" {
		return false, fmt.Errorf("could not reproduce the issue in any power mode")
	}

	// 3. Flatten configs of the faulty mode
	items, err := FlattenConfigs(global.TlpConfigDir)
	if err != nil || len(items) == 0 {
		return false, fmt.Errorf("failed to load configs for mode %s: %v", faultyMode, err)
	}

	if err := e.backupConfigs(); err != nil {
		return false, fmt.Errorf("failed to backup configs: %v", err)
	}

	// 4. Update state to start BINARY search
	state.AllItems = items
	state.CurrentRange = Range{Low: 0, High: len(items) - 1}
	state.Phase = "BINARY"

	global.Info("Precheck finished. Faulty mode [%s] found with %d items. Rebooting for binary search...", faultyMode, len(items))
	return true, nil // mustReboot = true
}

// handleBinaryStep implements: Test Left -> (pass) Test Right -> (both pass) Combo.
func (e *Engine) handleBinaryStep(state *DebugState) (bool, error) {
	low, high := state.CurrentRange.Low, state.CurrentRange.High

	if state.CurrentRange.IsSingle() {
		item := state.AllItems[low]
		global.Warn("[Found Culprit] Single item: %s=%s", item.Key, item.Value)
		state.FoundCulprits = append(state.FoundCulprits, Culprit{Items: []ConfigPair{item}, IsCombo: false})
		return e.popAndPrepareNext(state), nil
	}

	mid := state.CurrentRange.Mid()

	// 1. Test Left Half
	global.Info("Testing Left Half: items [%d-%d]", low, mid)
	passed, err := e.testSet(state.AllItems[low : mid+1])
	if !passed || err != nil {
		global.Error("Result: FAILED. Focusing on this sub-range.")
		state.PushRange(Range{Low: mid + 1, High: high})
		state.CurrentRange = Range{Low: low, High: mid}
		return true, err
	}

	// 2. Test Right Half
	global.Info("Result: PASSED. Now testing Right Half: items [%d-%d]", mid+1, high)
	passed, err = e.testSet(state.AllItems[mid+1 : high+1])
	if !passed || err != nil {
		global.Error("Result: FAILED. Focusing on this sub-range.")
		state.CurrentRange = Range{Low: mid + 1, High: high}
		return true, err
	}

	// 3. Both Passed -> Combo detected
	global.Warn("Result: BOTH HALVES PASSED but parent failed. Combo detected in range [%d-%d].", low, high)
	e.enterMinimizePhase(state, low, high)
	return false, nil
}

// handleMinimizeStep tries to find the smallest subset of the combo.
func (e *Engine) handleMinimizeStep(state *DebugState) (bool, error) {
	ctx := state.MinCtx
	global.Info("Minimizing Combo: Testing set without item index %d (%s)", ctx.Index, ctx.CurrentItems[ctx.Index].Key)

	batch := append([]ConfigPair{}, ctx.CurrentItems[:ctx.Index]...)
	batch = append(batch, ctx.CurrentItems[ctx.Index+1:]...)

	passed, err := e.testSet(batch)
	if !passed || err != nil {
		global.Error("Result: STILL FAILS. Removed item was redundant.")
		ctx.CurrentItems = append(ctx.CurrentItems[:ctx.Index], ctx.CurrentItems[ctx.Index+1:]...)
		return true, err
	}

	global.Info("Result: PASSED. Removed item is essential for this combo.")
	ctx.Index++
	if e.checkMinFinished(state) {
		return true, nil
	}
	return false, nil
}

// --- Logic & Control ---

func (e *Engine) enterMinimizePhase(state *DebugState, low, high int) {
	items := state.AllItems[low : high+1]
	state.Phase = "MINIMIZE"
	state.MinCtx = &MinContext{
		OriginalItems: items,
		CurrentItems:  append([]ConfigPair{}, items...),
		Index:         0,
	}
}

func (e *Engine) checkMinFinished(state *DebugState) bool {
	ctx := state.MinCtx
	if ctx.Index >= len(ctx.CurrentItems) {
		global.Warn("[Found Culprit] Minimal Combo set found (%d items)", len(ctx.CurrentItems))
		state.FoundCulprits = append(state.FoundCulprits, Culprit{Items: ctx.CurrentItems, IsCombo: true})
		state.Phase = "BINARY"
		state.MinCtx = nil
		return e.popAndPrepareNext(state)
	}
	return false
}

// popAndPrepareNext tries to find the next faulty range from the stack.
// Returns true if a NEW faulty CurrentRange is identified and needs a reboot to settle.
// Returns false if the stack is exhausted (session finished).
func (e *Engine) popAndPrepareNext(state *DebugState) bool {
	for {
		r, ok := state.PopRange()
		if !ok {
			state.CurrentRange = Range{Low: 1, High: 0}
			return false
		}

		global.Info("Validating popped range [%d-%d] from stack...", r.Low, r.High)

		passed, err := e.testSet(state.AllItems[r.Low : r.High+1])
		if err != nil {
			global.Error("Error during stack validation: %v. Assuming faulty to be safe.", err)
		}

		if passed {
			global.Info("Range [%d-%d] is PASSED. Skipping this branch.", r.Low, r.High)
			continue
		}

		global.Warn("Range [%d-%d] confirmed FAULTY. Setting as CurrentRange and requesting reboot.", r.Low, r.High)
		state.CurrentRange = r
		return true
	}
}

func (e *Engine) isFinished(state *DebugState) bool {
	return state.CurrentRange.Low > state.CurrentRange.High
}

func (e *Engine) reboot(state *DebugState) error {
	state.RebootRetryCount++
	if state.RebootRetryCount > global.AppConfig.RebootRetryCount {
		e.finalize(state, fmt.Errorf("reboot safety limit exceeded"))
		return nil
	}
	if err := state.Save(); err != nil {
		return err
	}
	global.Warn("Requesting reboot to reset hardware and apply next batch...")
	time.Sleep(1 * time.Second)
	return utils.RunCmd("reboot")
}

// --- System & File Operations ---

// StartBackgroundMonitor starts a goroutine to log probe status every second.
func (e *Engine) StartBackgroundMonitor(stopChan chan struct{}) {
	global.Info("Background Monitor Started: Polling hardware state every 1s...")
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopChan:
				global.Info("Background Monitor Stopped.")
				return
			case <-ticker.C:
				err := e.Probe.Check()
				if err != nil {
					global.Info("[MONITOR] State: BAD | %v", err)
				} else {
					global.Info("[MONITOR] State: OK  | suspended")
				}
			}
		}
	}()
}

func (e *Engine) testSet(items []ConfigPair) (bool, error) {
	// 1. Apply our specific test configuration
	if err := e.applyTemporaryConfig(items); err != nil {
		return false, fmt.Errorf("failed to apply temporary config: %v", err)
	}

	// 12. Isolate environment by renaming existing configs
	if err := e.purgeConfigs(); err != nil {
		return false, err
	}

	// 3. Trigger TLP using the hijacked binary
	if err := utils.RunCmd(global.CmdTlpDebug, "init", "start"); err != nil {
		return false, fmt.Errorf("failed to execute tlp.debug: %v", err)
	}

	// 4. Settle time
	time.Sleep(1 * time.Second)

	// 5. Final Probe
	if err := e.Probe.Check(); err != nil {
		global.Info("Probe result: MISMATCH (%v)", err)
		// 6. IMMEDIATE AUDIT: Check if any other program snuck in a config
		if err := e.verifyConfigPurity(); err != nil {
			// If this fails, we return a hard error to trigger "Abort & Preserve Scene"
			return false, err
		}
		return false, nil // Test ran OK, but result is not what we want
	}

	global.Info("Probe result: MATCH")
	return true, nil
}

func (e *Engine) applyTemporaryConfig(items []ConfigPair) error {
	f, err := os.Create(e.tempConfig)
	if err != nil {
		return fmt.Errorf("os.Create failed: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	if _, err := fmt.Fprintln(w, "# Generated by deepin-power-control debugger"); err != nil {
		return err
	}
	for _, it := range items {
		if _, err := fmt.Fprintf(w, "%s=\"%s\"\n", it.Key, it.Value); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "TLP_ENABLE=\"1\""); err != nil {
		return err
	}
	return w.Flush()
}

func (e *Engine) backupConfigs() error {
	files, err := filepath.Glob(filepath.Join(global.TlpConfigDir, "*.conf"))
	if err != nil {
		return err
	}
	for _, f := range files {
		if f == e.tempConfig {
			continue
		}
		if err := os.Rename(f, f+".debugtmp"); err != nil {
			global.Error("Warning: Failed to rename %s: %v", f, err)
		}
	}
	return nil
}

func (e *Engine) purgeConfigs() error {
	files, err := filepath.Glob(filepath.Join(global.TlpConfigDir, "*.conf"))
	if err != nil {
		return err
	}
	for _, f := range files {
		if f == e.tempConfig {
			continue
		}
		if err := os.Remove(f); err != nil {
			global.Error("Warning: Failed to rename %s: %v", f, err)
		}
	}
	return nil
}

// verifyConfigPurity ensures NO other .conf files were created during execution.
func (e *Engine) verifyConfigPurity() error {
	files, _ := filepath.Glob(filepath.Join(global.TlpConfigDir, "*.conf"))
	for _, f := range files {
		if f != e.tempConfig {
			return fmt.Errorf("concurrency pollution detected: unauthorized file [%s] found", f)
		}
	}
	return nil
}

func (e *Engine) Restore() {
	if err := os.Remove(e.tempConfig); err != nil && !os.IsNotExist(err) {
		global.Error("Warning: Failed to remove temp config: %v", err)
	}
	_ = filepath.Walk(global.TlpConfigDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(info.Name(), ".debugtmp") {
			oldPath := strings.TrimSuffix(path, ".debugtmp")
			_ = os.Rename(path, oldPath)
		}
		return nil
	})
	_ = restoreTlp()
}

func (e *Engine) finalize(state *DebugState, err error) {
	if err != nil {
		// Situation A: Critical Error / Environment Corruption
		global.Error("!!! Session Aborted due to error: %v", err)
		global.Error("SYSTEM NOT RESTORED to preserve debug environment.")
		global.Error("To fix system manually, run: deepin-power-control debug abort")
		// We stop here. Do NOT call Restore() or RestoreSystem().
		return
	}

	// Situation B: Normal search completion
	global.Info("=== Debug Session Successfully Finished ===")
	if state != nil {
		e.printFinalReport(state.FoundCulprits)
	}

	// Only restore when everything finished correctly
	global.Info("Restoring system to original state...")
	e.Restore()
	RestoreSystem()
}

func (e *Engine) printFinalReport(culprits []Culprit) {
	global.Info("\n%s\n", strings.Repeat("=", 50))
	global.Info("%-10s | %s\n", "TYPE", "CONFIGURATION")
	global.Info("%s\n", strings.Repeat("-", 50))
	for _, c := range culprits {
		label := "SINGLE"
		if c.IsCombo {
			label = "COMBO"
		}
		for _, it := range c.Items {
			global.Info("%-10s | %s=%s\n", label, it.Key, it.Value)
		}
	}
	global.Info("%s\n", strings.Repeat("=", 50))
}
