package utils

import (
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
		os.WriteFile(path, []byte(""), 0600)
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

func TestIsDir(t *testing.T) {
	tmpDir := t.TempDir()

	tmpFile := filepath.Join(tmpDir, "testfile.txt")
	os.WriteFile(tmpFile, []byte("hello"), 0644)

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"Existing directory", tmpDir, true},
		{"Existing file", tmpFile, false},
		{"Non-existing path", "/tmp/path/not/exists", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDir(tt.path); got != tt.want {
				t.Errorf("IsDir() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFileContains(t *testing.T) {
	tmpDir := t.TempDir()

	content := "cpu_feature: hwp_epp\nvendor_id: AuthenticAMD\n"
	tmpFile := filepath.Join(tmpDir, "cpuinfo")
	os.WriteFile(tmpFile, []byte(content), 0644)

	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{"Exact match", "hwp_epp", true},
		{"Partial match", "AMD", true},
		{"No match", "intel", false},
		{"Empty string", "", true},              // strings.Contains("", "") is true
		{"Non-existing file", "hwp_epp", false}, // we will handle this in the run
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tmpFile
			if tt.name == "Non-existing file" {
				path = "/tmp/no-such-file"
			}
			if got := FileContains(path, tt.target); got != tt.want {
				t.Errorf("FileContains() = %v, want %v", got, tt.want)
			}
		})
	}
}
