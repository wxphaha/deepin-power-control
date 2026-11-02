// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

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
	return GetFilesBySuffix(baseDir, ".conf")
}

// GetFilesBySuffix finds files with the given suffix and sorts them by the
// full relative path (lexicographic). Directory names participate in the
// ordering, so "20-intel/1.yaml" sorts between "15-xxx.yaml" and "99.yaml":
// the numeric prefix is the priority everywhere, directories are just
// categorization containers. A trailing convention: directories without a
// numeric prefix (e.g. oem/) sort after all digit-prefixed entries.
func GetFilesBySuffix(baseDir string, suffix string) ([]string, error) {
	var files []string

	// WalkDir recursively finds all files in the directory tree
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Filter: only include regular files with the given suffix
		if !info.IsDir() && strings.HasSuffix(info.Name(), suffix) {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort by the full relative path: the numeric prefix decides priority
	// globally (directories included), later entries override earlier ones.
	sort.Slice(files, func(i, j int) bool {
		relI, _ := filepath.Rel(baseDir, files[i])
		relJ, _ := filepath.Rel(baseDir, files[j])
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
