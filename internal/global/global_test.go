package global

import (
	"io/ioutil"
	"path/filepath"
	"testing"
)

// TestInitDefaultConfig verifies that default values are correctly applied
func TestInitDefaultConfig(t *testing.T) {
	InitDefaultConfig()

	if AppConfig.WifiThreshold != 100 {
		t.Errorf("Expected default threshold 100, got %d", AppConfig.WifiThreshold)
	}
	if AppConfig.LogLevel != LevelInfo {
		t.Errorf("Expected default log level Info, got %s", AppConfig.LogLevel)
	}
}

// TestLogLevel_UnmarshalYAML tests the custom YAML unmarshaling logic for LogLevel
func TestLogLevel_UnmarshalYAML(t *testing.T) {
	// Table-driven tests for different YAML inputs
	tests := []struct {
		name     string
		yamlData string
		expected LogLevel
	}{
		{"Lower Debug", "debug", LevelDebug},
		{"Upper Info", "INFO", LevelInfo},
		{"Mixed Warn", "WaRn", LevelWarn},
		{"Error", "error", LevelError},
		{"Unknown fallback", "something_else", LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// In reality, yaml.Unmarshal handles the node decoding
			// We can test LoadConfig with a temporary file to trigger this
			tmpFile := filepath.Join(t.TempDir(), "config.yaml")
			content := "log_level: " + tt.yamlData
			ioutil.WriteFile(tmpFile, []byte(content), 0644)

			LoadConfig(tmpFile)
			if AppConfig.LogLevel != tt.expected {
				t.Errorf("For input %s, expected %v, got %v", tt.yamlData, tt.expected, AppConfig.LogLevel)
			}
		})
	}
}

// TestLoadConfig_Validation tests the post-load boundary checks
func TestLoadConfig_Validation(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("Invalid Intervals", func(t *testing.T) {
		// Min is 60, Max is 30. Max should be corrected to Min.
		content := `
min_check_interval: 60
max_check_interval: 30
wifi_multiplier: 0
`
		confPath := filepath.Join(tmpDir, "invalid_val.yaml")
		ioutil.WriteFile(confPath, []byte(content), 0644)

		LoadConfig(confPath)

		if AppConfig.MaxCheckInterval != 60 {
			t.Errorf("MaxCheckInterval should be capped to Min (60), got %d", AppConfig.MaxCheckInterval)
		}
		if AppConfig.WifiMultiplier != 3 {
			t.Errorf("WifiMultiplier 0 should fallback to 3, got %d", AppConfig.WifiMultiplier)
		}
	})

	t.Run("Zero Threshold", func(t *testing.T) {
		content := "wifi_threshold: 0"
		confPath := filepath.Join(tmpDir, "zero_thresh.yaml")
		ioutil.WriteFile(confPath, []byte(content), 0644)

		LoadConfig(confPath)

		if AppConfig.WifiThreshold != 100 {
			t.Errorf("Zero threshold should fallback to 100, got %d", AppConfig.WifiThreshold)
		}
	})
}

// TestParseLogLevel verifies the string-to-type conversion function
func TestParseLogLevel(t *testing.T) {
	if ParseLogLevel("DEBUG") != LevelDebug {
		t.Error("ParseLogLevel DEBUG failed")
	}
	if ParseLogLevel("invalid") != LevelInfo {
		t.Error("ParseLogLevel fallback failed")
	}
}
