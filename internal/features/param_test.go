// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package features

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPciParamViaDeclaration verifies "pci:xxx" resolves through the
// parameterized DECLARATION (features.yaml), not the code registry.
func TestPciParamViaDeclaration(t *testing.T) {
	// mock sysfs
	oldPath := PathPciDevices
	defer func() { PathPciDevices = oldPath }()
	dir := t.TempDir()
	PathPciDevices = dir
	ResetPciCache()
	dev := filepath.Join(dir, "0000:01:00.0")
	os.MkdirAll(dev, 0755)
	os.WriteFile(filepath.Join(dev, "vendor"), []byte("0x1002\n"), 0644)
	os.WriteFile(filepath.Join(dev, "device"), []byte("0x699f\n"), 0644)

	e := New() // defs from embedded features.yaml (has pci declaration)
	if !e.Enabled("pci:1002:699f") {
		t.Fatal("expected pci param match via declaration")
	}
	if e.Enabled("pci:10de:1234") {
		t.Fatal("expected no match for absent device")
	}
	// pci must not be resolved via the code registry (it is declared in
	// features.yaml); the registry only holds code-backed features.
	if _, ok := FeatureChecks["pci"]; ok {
		t.Fatalf("pci should not be in the code registry: %v", FeatureChecks)
	}
}

// TestPciBareCallWarns verifies a bare "pci" evaluation (no parameter)
// returns false instead of silently matching.
func TestPciBareCallWarns(t *testing.T) {
	e := New()
	if e.Enabled("pci") {
		t.Fatal("expected bare pci (no param) to be false")
	}
	if e.EnabledParam("pci", "") {
		t.Fatal("expected pci with empty param to be false")
	}
}
