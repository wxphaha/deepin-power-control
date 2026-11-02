// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGetConfigFilesInDir verifies the lexicographic path ordering:
// the numeric prefix decides priority globally (directories included);
// directories without a numeric prefix sort after digit-prefixed entries.
func TestGetConfigFilesInDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Mixed structure: root files, a digit-prefixed subdirectory (its
	// contents join the global ordering), and a non-prefixed subdir
	// (sorts after all digit-prefixed entries).
	filesToCreate := []string{
		"20-override.conf",
		"00-base.conf",
		filepath.Join("15-sub", "01-nested.conf"),
		"10-mode.conf",
		filepath.Join("subdir", "01-sub.conf"),
	}

	for _, f := range filesToCreate {
		path := filepath.Join(tmpDir, f)
		os.MkdirAll(filepath.Dir(path), 0750)
		os.WriteFile(path, []byte(""), 0600)
	}

	files, err := GetConfigFilesInDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to scan files: %v", err)
	}

	// Lexicographic order of relative paths:
	//   "00-base.conf" < "10-mode.conf" < "15-sub/01-nested.conf" <
	//   "20-override.conf" < "subdir/01-sub.conf"
	expected := []string{
		"00-base.conf",
		"10-mode.conf",
		"01-nested.conf",
		"20-override.conf",
		"01-sub.conf",
	}

	if len(files) != len(expected) {
		t.Fatalf("Expected %d files, got %d: %v", len(expected), len(files), files)
	}

	for i, f := range files {
		if !strings.HasSuffix(f, expected[i]) {
			t.Errorf("Order mismatch at index %d: got %s, want %s", i, f, expected[i])
		}
	}
}

// TestGetFilesBySuffixTwoDigitEdge verifies the lexicographic trap:
// "100-xxx" sorts BEFORE "99-xxx" because '1' < '9' as characters.
func TestGetFilesBySuffixTwoDigitEdge(t *testing.T) {
	tmpDir := t.TempDir()
	for _, f := range []string{"99-last.yaml", "100-first.yaml"} {
		os.WriteFile(filepath.Join(tmpDir, f), []byte(""), 0600)
	}

	files, err := GetFilesBySuffix(tmpDir, ".yaml")
	if err != nil {
		t.Fatalf("Failed to scan files: %v", err)
	}

	// "100-first.yaml" < "99-last.yaml" ('1' < '9').
	if len(files) != 2 || !strings.HasSuffix(files[0], "100-first.yaml") {
		t.Fatalf("expected 100-first.yaml first (lexicographic), got %v", files)
	}
}
