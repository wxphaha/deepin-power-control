package utils

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GetConfigFilesInDir finds .conf files and sorts them by depth then alphabet
func GetConfigFilesInDir(baseDir string) ([]string, error) {
	var files []string

	// WalkDir recursively finds all files in the directory tree
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Filter: only include regular files with the .conf extension
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".conf") {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort files based on directory depth and then alphabetical order
	sort.Slice(files, func(i, j int) bool {
		// Use the indices i and j to get the actual paths
		pathI := files[i]
		pathJ := files[j]

		// Get relative paths to calculate depth independent of the absolute root
		relI, _ := filepath.Rel(baseDir, pathI)
		relJ, _ := filepath.Rel(baseDir, pathJ)

		// Count the number of path separators to determine nesting level
		depthI := strings.Count(relI, string(os.PathSeparator))
		depthJ := strings.Count(relJ, string(os.PathSeparator))

		// Priority 1: Shorter depth (root files) comes first
		if depthI != depthJ {
			return depthI < depthJ
		}

		// Priority 2: Same depth, sort alphabetically by relative path
		return relI < relJ
	})

	return files, nil
}

// Internal helper to check if a path is a directory
func IsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Internal helper to check if a file contains a specific string (efficiently)
func FileContains(path, target string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), target) {
			return true
		}
	}
	return false
}
