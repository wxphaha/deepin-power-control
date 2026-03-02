package utils

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGetConfigFilesInDir verifies the file sorting logic:
// 1. Root files first (shorter depth).
// 2. Alphabetical order within the same depth.
func TestGetConfigFilesInDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mixed structure of files
	filesToCreate := []string{
		"20-override.conf",
		"00-base.conf",
		filepath.Join("subdir", "01-sub.conf"),
		"10-mode.conf",
	}

	for _, f := range filesToCreate {
		path := filepath.Join(tmpDir, f)
		os.MkdirAll(filepath.Dir(path), 0750)
		ioutil.WriteFile(path, []byte(""), 0600)
	}

	files, err := GetConfigFilesInDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to scan files: %v", err)
	}

	// Expected order based on depth then name
	expected := []string{
		"00-base.conf",
		"10-mode.conf",
		"20-override.conf",
		"01-sub.conf",
	}

	if len(files) != len(expected) {
		t.Fatalf("Expected %d files, got %d", len(expected), len(files))
	}

	for i, f := range files {
		if !strings.HasSuffix(f, expected[i]) {
			t.Errorf("Order mismatch at index %d: got %s, want %s", i, f, expected[i])
		}
	}
}
