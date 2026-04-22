// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ImageConfig represents the top-level structure of an image definition YAML.
type ImageConfig struct {
	Name        string           `yaml:"name"`
	OS          string           `yaml:"os"`
	Description string           `yaml:"description"`
	QEMU        QEMUConfig       `yaml:"qemu"`
	Connection  ConnectionConfig `yaml:"connection"`
	Setup       SetupConfig      `yaml:"setup"`
}

// QEMUConfig holds QEMU-specific settings from the image YAML.
type QEMUConfig struct {
	Image        string   `yaml:"image"`
	Firmware     string   `yaml:"firmware"`
	FirmwareVars string   `yaml:"firmware_vars"`
	Memory       string   `yaml:"memory"`
	CPUs         int      `yaml:"cpus"`
	ExtraArgs    []string `yaml:"extra_args"`
}

// ConnectionConfig holds guest connection settings.
type ConnectionConfig struct {
	ExecMethod string `yaml:"exec_method"` // "ssh" (default) or "winrm"
	FileMethod string `yaml:"file_method"` // defaults to exec_method
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	KeyFile    string `yaml:"key_file"`   // SSH only
	Port       int    `yaml:"port"`       // deprecated compatibility field
	SSHPort    int    `yaml:"ssh_port"`   // default 22
	WinRMPort  int    `yaml:"winrm_port"` // default 5985 or 5986 with TLS
	UseTLS     bool   `yaml:"use_tls"`    // WinRM only: use HTTPS (port 5986)
}

// SetupConfig holds the setup phase configuration.
type SetupConfig struct {
	Steps []Step `yaml:"steps"`
}

// LoadImageConfig reads and parses an image definition YAML file.
func LoadImageConfig(path string) (*ImageConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read image config: %w", err)
	}

	var cfg ImageConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse image config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validate checks the ImageConfig for required fields and applies defaults.
func (c *ImageConfig) validate() error {
	if c.Name == "" {
		return fmt.Errorf("image config: 'name' is required")
	}

	switch c.OS {
	case "windows", "linux":
		// valid
	case "":
		return fmt.Errorf("image config: 'os' is required (windows or linux)")
	default:
		return fmt.Errorf("image config: unsupported os %q (must be windows or linux)", c.OS)
	}

	if c.QEMU.Image == "" {
		return fmt.Errorf("image config: 'qemu.image' is required")
	}

	if _, err := os.Stat(c.QEMU.Image); err != nil {
		return fmt.Errorf("image config: qemu.image not found: %w", err)
	}

	if c.QEMU.Memory == "" {
		c.QEMU.Memory = "2G"
	}

	if c.QEMU.CPUs <= 0 {
		c.QEMU.CPUs = 2
	}

	// Connection defaults
	switch c.Connection.ExecMethod {
	case "ssh", "":
		c.Connection.ExecMethod = "ssh"
	case "winrm":
		// valid
	default:
		return fmt.Errorf("image config: unsupported exec_method %q (must be ssh or winrm)", c.Connection.ExecMethod)
	}
	switch c.Connection.FileMethod {
	case "":
		c.Connection.FileMethod = c.Connection.ExecMethod
	case "ssh", "winrm":
		// valid
	default:
		return fmt.Errorf("image config: unsupported file_method %q (must be ssh or winrm)", c.Connection.FileMethod)
	}

	if c.Connection.Username == "" {
		return fmt.Errorf("image config: 'connection.username' is required")
	}

	if c.Connection.ExecMethod == "ssh" || c.Connection.FileMethod == "ssh" {
		if c.Connection.Password == "" && c.Connection.KeyFile == "" {
			return fmt.Errorf("image config: either 'connection.password' or 'connection.key_file' is required for SSH")
		}
	}
	if c.Connection.ExecMethod == "winrm" || c.Connection.FileMethod == "winrm" {
		if c.Connection.Password == "" {
			return fmt.Errorf("image config: 'connection.password' is required for WinRM")
		}
	}

	if c.Connection.Port > 0 {
		if c.Connection.ExecMethod == "ssh" && c.Connection.SSHPort == 0 {
			c.Connection.SSHPort = c.Connection.Port
		}
		if c.Connection.ExecMethod == "winrm" && c.Connection.WinRMPort == 0 {
			c.Connection.WinRMPort = c.Connection.Port
		}
	}

	if c.Connection.SSHPort <= 0 {
		c.Connection.SSHPort = 22
	}
	if c.Connection.WinRMPort <= 0 {
		if c.Connection.UseTLS {
			c.Connection.WinRMPort = 5986
		} else {
			c.Connection.WinRMPort = 5985
		}
	}

	return nil
}
