// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// TestConfig represents the top-level structure of a test definition YAML.
type TestConfig struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Include     []string       `yaml:"include"`
	QEMU        TestQEMUConfig `yaml:"qemu"`
	Preboot     SetupConfig    `yaml:"preboot"`
	Steps       []Step         `yaml:"steps"`
	Panic       PanicConfig    `yaml:"panic"`
}

// TestQEMUConfig holds test-specific QEMU overrides.
type TestQEMUConfig struct {
	Serial string `yaml:"serial,omitempty"`
}

// PanicConfig holds panic handling configuration.
type PanicConfig struct {
	Steps []Step `yaml:"steps"`
}

// LoadTestConfig reads and parses a test definition YAML file, processing includes.
func LoadTestConfig(path string) (*TestConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read test config: %w", err)
	}

	var cfg TestConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse test config: %w", err)
	}

	baseDir := filepath.Dir(path)

	if err := cfg.processIncludes(baseDir); err != nil {
		return nil, err
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// processIncludes loads and merges include files into the TestConfig.
// Merge rules:
//   - Include files are loaded in order.
//   - If the main YAML defines a field, it takes precedence (override).
//   - For array fields (preboot.steps, steps, panic.steps), if the main defines them, includes are ignored.
//   - Only 1-depth includes (nested includes in include files are not processed).
func (c *TestConfig) processIncludes(baseDir string) error {
	if len(c.Include) == 0 {
		return nil
	}

	// Save main config's arrays to detect if they were explicitly defined
	mainHasPrebootSteps := len(c.Preboot.Steps) > 0
	mainHasSteps := len(c.Steps) > 0
	mainHasPanicSteps := len(c.Panic.Steps) > 0
	mainHasQEMUSerial := c.QEMU.Serial != ""

	// Merged values from includes (last wins among includes)
	var mergedPrebootSteps []Step
	var mergedSteps []Step
	var mergedPanicSteps []Step
	var mergedQEMUSerial string

	for _, includePath := range c.Include {
		fullPath := filepath.Join(baseDir, includePath)

		data, err := os.ReadFile(fullPath)
		if err != nil {
			return fmt.Errorf("failed to read include file %q: %w", includePath, err)
		}

		var inc TestConfig
		if err := yaml.Unmarshal(data, &inc); err != nil {
			return fmt.Errorf("failed to parse include file %q: %w", includePath, err)
		}

		if len(inc.Preboot.Steps) > 0 {
			mergedPrebootSteps = inc.Preboot.Steps
		}
		if len(inc.Steps) > 0 {
			mergedSteps = inc.Steps
		}
		if len(inc.Panic.Steps) > 0 {
			mergedPanicSteps = inc.Panic.Steps
		}
		if inc.QEMU.Serial != "" {
			mergedQEMUSerial = inc.QEMU.Serial
		}
	}

	// Apply merge: main overrides includes
	if !mainHasPrebootSteps && len(mergedPrebootSteps) > 0 {
		c.Preboot.Steps = mergedPrebootSteps
	}
	if !mainHasSteps && len(mergedSteps) > 0 {
		c.Steps = mergedSteps
	}
	if !mainHasPanicSteps && len(mergedPanicSteps) > 0 {
		c.Panic.Steps = mergedPanicSteps
	}
	if !mainHasQEMUSerial && mergedQEMUSerial != "" {
		c.QEMU.Serial = mergedQEMUSerial
	}

	return nil
}

// validate checks the TestConfig for required fields.
func (c *TestConfig) validate() error {
	if c.Name == "" {
		return fmt.Errorf("test config: 'name' is required")
	}

	if len(c.Steps) == 0 {
		return fmt.Errorf("test config: at least one step is required")
	}

	for i, step := range c.Preboot.Steps {
		if step.Action == "" {
			return fmt.Errorf("test config: preboot.steps[%d] has empty action", i)
		}
		if step.Timeout.Duration <= 0 {
			c.Preboot.Steps[i].Timeout.Duration = 30 * time.Second
		}
	}

	for i, step := range c.Steps {
		if step.Action == "" {
			return fmt.Errorf("test config: step[%d] has empty action", i)
		}
		if step.Timeout.Duration <= 0 {
			return fmt.Errorf("test config: step[%d] (%s) has invalid timeout", i, step.Action)
		}
	}

	for i, step := range c.Panic.Steps {
		if step.Action == "" {
			return fmt.Errorf("test config: panic.steps[%d] has empty action", i)
		}
		if step.Timeout.Duration <= 0 {
			return fmt.Errorf("test config: panic.steps[%d] (%s) has invalid timeout", i, step.Action)
		}
	}

	return nil
}
