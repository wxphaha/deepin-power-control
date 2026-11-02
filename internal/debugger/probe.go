// SPDX-FileCopyrightText: 2026 UnionTech Software Technology Co., Ltd.
//
// SPDX-License-Identifier: GPL-3.0-or-later

package debugger

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Checker defines how to verify if the current configuration is "Safe".
// Now returns error: nil means PASS, non-nil means FAIL with detailed info.
type Checker interface {
	Check() error
}

// ManualProbe waits for user input to determine the status.
type ManualProbe struct{}

func (p *ManualProbe) Check() error {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("\n[PROBE] Does the system work correctly? (y/n): ")
		input, _ := reader.ReadString('\n')
		input = strings.ToLower(strings.TrimSpace(input))

		if input == "y" {
			return nil // PASS
		} else if input == "n" {
			return fmt.Errorf("manual probe: user reported failure")
		}
		fmt.Println("Invalid input. Please enter 'y' for PASS or 'n' for FAIL.")
	}
}

// SysfsProbe automatically verifies a value from a given sysfs path.
type SysfsProbe struct {
	Path   string
	Expect string
}

func (p *SysfsProbe) Check() error {
	// Read raw data from the specified path
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return fmt.Errorf("sysfs probe error: failed to read path %s: %v", p.Path, err)
	}

	actual := strings.TrimSpace(string(data))

	// Compare actual value with expectation
	if actual != p.Expect {
		return fmt.Errorf("sysfs probe mismatch on [%s]: expected '%s', but got '%s'",
			p.Path, p.Expect, actual)
	}

	// If everything matches, return nil (PASS)
	return nil
}
