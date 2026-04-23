// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// --- TestDurationUnmarshalYAML ---

func TestDurationUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{name: "seconds", input: "120s", want: 120 * time.Second},
		{name: "minutes", input: "5m", want: 5 * time.Minute},
		{name: "hours_and_minutes", input: "1h30m", want: 90 * time.Minute},
		{name: "milliseconds", input: "500ms", want: 500 * time.Millisecond},
		{name: "composite", input: "2h30m10s", want: 2*time.Hour + 30*time.Minute + 10*time.Second},
		{name: "invalid_string", input: "notaduration", wantErr: true},
		{name: "bare_unit", input: "s", wantErr: true},
		{name: "just_number", input: "120", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Wrap the input in a YAML document to test unmarshalling
			type wrapper struct {
				D Duration `yaml:"d"`
			}
			yamlStr := "d: " + tt.input
			var w wrapper
			err := yaml.Unmarshal([]byte(yamlStr), &w)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for input %q, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tt.input, err)
			}

			if w.D.Duration != tt.want {
				t.Errorf("got %v, want %v", w.D.Duration, tt.want)
			}
		})
	}
}

// --- TestLoadImageConfig ---

func TestLoadImageConfig(t *testing.T) {
	// Helper to create a temp qemu image file for validation
	createTempImage := func(t *testing.T) string {
		t.Helper()
		f, err := os.CreateTemp(t.TempDir(), "test-image-*.qcow2")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
		return f.Name()
	}

	t.Run("valid_config", func(t *testing.T) {
		imgPath := createTempImage(t)
		yamlContent := `
name: test-win11
os: windows
description: Test image
qemu:
  image: ` + imgPath + `
  firmware: /usr/share/OVMF/OVMF_CODE.fd
  memory: 4G
  cpus: 4
connection:
  username: testuser
  password: testpass
  port: 2222
setup:
  steps:
    - action: wait-boot
      timeout: 120s
`
		tmpFile := filepath.Join(t.TempDir(), "image.yaml")
		if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
			t.Fatal(err)
		}

		cfg, err := LoadImageConfig(tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.Name != "test-win11" {
			t.Errorf("Name = %q, want %q", cfg.Name, "test-win11")
		}
		if cfg.OS != "windows" {
			t.Errorf("OS = %q, want %q", cfg.OS, "windows")
		}
		if cfg.QEMU.Image != imgPath {
			t.Errorf("QEMU.Image = %q, want %q", cfg.QEMU.Image, imgPath)
		}
		if cfg.QEMU.Memory != "4G" {
			t.Errorf("QEMU.Memory = %q, want %q", cfg.QEMU.Memory, "4G")
		}
		if cfg.QEMU.CPUs != 4 {
			t.Errorf("QEMU.CPUs = %d, want %d", cfg.QEMU.CPUs, 4)
		}
		if cfg.Connection.Username != "testuser" {
			t.Errorf("SSH.Username = %q, want %q", cfg.Connection.Username, "testuser")
		}
		if cfg.Connection.SSHPort != 2222 {
			t.Errorf("SSHPort = %d, want %d", cfg.Connection.SSHPort, 2222)
		}
	})

	t.Run("preboot_defaults_timeout", func(t *testing.T) {
		imgPath := createTempImage(t)
		yamlContent := `
name: test-preboot
os: windows
qemu:
  image: ` + imgPath + `
connection:
  username: testuser
  password: testpass
preboot:
  steps:
    - action: efi-add-file
      params:
        src: ./bootx64.efi
        dst: /EFI/Boot/bootx64.efi
setup:
  steps:
    - action: wait-boot
      timeout: 120s
`
		tmpFile := filepath.Join(t.TempDir(), "image.yaml")
		if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
			t.Fatal(err)
		}

		cfg, err := LoadImageConfig(tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.Preboot.Steps) != 1 {
			t.Fatalf("Preboot.Steps len = %d, want 1", len(cfg.Preboot.Steps))
		}
		if cfg.Preboot.Steps[0].Timeout.Duration != 30*time.Second {
			t.Fatalf("Preboot timeout = %v, want %v", cfg.Preboot.Steps[0].Timeout.Duration, 30*time.Second)
		}
	})

	t.Run("missing_name", func(t *testing.T) {
		imgPath := createTempImage(t)
		yamlContent := `
os: linux
qemu:
  image: ` + imgPath + `
connection:
  username: user
  password: pass
`
		tmpFile := filepath.Join(t.TempDir(), "image.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		_, err := LoadImageConfig(tmpFile)
		if err == nil {
			t.Fatal("expected error for missing name")
		}
	})

	t.Run("missing_os", func(t *testing.T) {
		imgPath := createTempImage(t)
		yamlContent := `
name: test
qemu:
  image: ` + imgPath + `
connection:
  username: user
  password: pass
`
		tmpFile := filepath.Join(t.TempDir(), "image.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		_, err := LoadImageConfig(tmpFile)
		if err == nil {
			t.Fatal("expected error for missing os")
		}
	})

	t.Run("invalid_os", func(t *testing.T) {
		imgPath := createTempImage(t)
		yamlContent := `
name: test
os: macos
qemu:
  image: ` + imgPath + `
connection:
  username: user
  password: pass
`
		tmpFile := filepath.Join(t.TempDir(), "image.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		_, err := LoadImageConfig(tmpFile)
		if err == nil {
			t.Fatal("expected error for invalid os")
		}
	})

	t.Run("missing_qemu_image", func(t *testing.T) {
		yamlContent := `
name: test
os: linux
connection:
  username: user
  password: pass
`
		tmpFile := filepath.Join(t.TempDir(), "image.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		_, err := LoadImageConfig(tmpFile)
		if err == nil {
			t.Fatal("expected error for missing qemu.image")
		}
	})

	t.Run("missing_ssh_username", func(t *testing.T) {
		imgPath := createTempImage(t)
		yamlContent := `
name: test
os: linux
qemu:
  image: ` + imgPath + `
connection:
  password: pass
`
		tmpFile := filepath.Join(t.TempDir(), "image.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		_, err := LoadImageConfig(tmpFile)
		if err == nil {
			t.Fatal("expected error for missing connection.username")
		}
	})

	t.Run("missing_ssh_auth", func(t *testing.T) {
		imgPath := createTempImage(t)
		yamlContent := `
name: test
os: linux
qemu:
  image: ` + imgPath + `
connection:
  username: user
`
		tmpFile := filepath.Join(t.TempDir(), "image.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		_, err := LoadImageConfig(tmpFile)
		if err == nil {
			t.Fatal("expected error when neither connection.password nor connection.key_file is set")
		}
	})

	t.Run("defaults_applied", func(t *testing.T) {
		imgPath := createTempImage(t)
		yamlContent := `
name: test
os: linux
qemu:
  image: ` + imgPath + `
connection:
  username: user
  password: pass
`
		tmpFile := filepath.Join(t.TempDir(), "image.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		cfg, err := LoadImageConfig(tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.Connection.FileMethod != "ssh" {
			t.Errorf("default FileMethod = %q, want %q", cfg.Connection.FileMethod, "ssh")
		}
		if cfg.Connection.SSHPort != 22 {
			t.Errorf("default SSHPort = %d, want 22", cfg.Connection.SSHPort)
		}
		if cfg.QEMU.CPUs != 2 {
			t.Errorf("default QEMU.CPUs = %d, want 2", cfg.QEMU.CPUs)
		}
		if cfg.QEMU.Memory != "2G" {
			t.Errorf("default QEMU.Memory = %q, want %q", cfg.QEMU.Memory, "2G")
		}
	})

	t.Run("ssh_key_file_accepted", func(t *testing.T) {
		imgPath := createTempImage(t)
		yamlContent := `
name: test
os: linux
qemu:
  image: ` + imgPath + `
connection:
  username: user
  key_file: /path/to/key
`
		tmpFile := filepath.Join(t.TempDir(), "image.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		cfg, err := LoadImageConfig(tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Connection.KeyFile != "/path/to/key" {
			t.Errorf("SSH.KeyFile = %q, want %q", cfg.Connection.KeyFile, "/path/to/key")
		}
	})

	t.Run("split_exec_and_file_methods", func(t *testing.T) {
		imgPath := createTempImage(t)
		yamlContent := `
name: test
os: windows
qemu:
  image: ` + imgPath + `
connection:
  exec_method: winrm
  file_method: ssh
  username: user
  password: pass
`
		tmpFile := filepath.Join(t.TempDir(), "image.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		cfg, err := LoadImageConfig(tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Connection.ExecMethod != "winrm" {
			t.Errorf("ExecMethod = %q, want %q", cfg.Connection.ExecMethod, "winrm")
		}
		if cfg.Connection.FileMethod != "ssh" {
			t.Errorf("FileMethod = %q, want %q", cfg.Connection.FileMethod, "ssh")
		}
		if cfg.Connection.SSHPort != 22 {
			t.Errorf("SSHPort = %d, want 22", cfg.Connection.SSHPort)
		}
		if cfg.Connection.WinRMPort != 5985 {
			t.Errorf("WinRMPort = %d, want 5985", cfg.Connection.WinRMPort)
		}
	})

	t.Run("nonexistent_file", func(t *testing.T) {
		_, err := LoadImageConfig("/nonexistent/path/image.yaml")
		if err == nil {
			t.Fatal("expected error for nonexistent file")
		}
	})

	t.Run("qemu_image_not_found", func(t *testing.T) {
		yamlContent := `
name: test
os: linux
qemu:
  image: /nonexistent/image.qcow2
connection:
  username: user
  password: pass
`
		tmpFile := filepath.Join(t.TempDir(), "image.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		_, err := LoadImageConfig(tmpFile)
		if err == nil {
			t.Fatal("expected error for nonexistent qemu.image file")
		}
	})
}

// --- TestLoadTestConfig ---

func TestLoadTestConfig(t *testing.T) {
	t.Run("valid_config", func(t *testing.T) {
		yamlContent := `
name: basic-test
description: A basic test
steps:
  - action: wait-boot
    timeout: 120s
  - action: exec
    timeout: 30s
    params:
      cmd: echo
      args: ["hello"]
`
		tmpFile := filepath.Join(t.TempDir(), "test.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		cfg, err := LoadTestConfig(tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.Name != "basic-test" {
			t.Errorf("Name = %q, want %q", cfg.Name, "basic-test")
		}
		if len(cfg.Steps) != 2 {
			t.Fatalf("len(Steps) = %d, want 2", len(cfg.Steps))
		}
		if cfg.Steps[0].Action != "wait-boot" {
			t.Errorf("Steps[0].Action = %q, want %q", cfg.Steps[0].Action, "wait-boot")
		}
		if cfg.Steps[0].Timeout.Duration != 120*time.Second {
			t.Errorf("Steps[0].Timeout = %v, want 120s", cfg.Steps[0].Timeout.Duration)
		}
	})

	t.Run("empty_steps", func(t *testing.T) {
		yamlContent := `
name: empty-test
steps: []
`
		tmpFile := filepath.Join(t.TempDir(), "test.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		_, err := LoadTestConfig(tmpFile)
		if err == nil {
			t.Fatal("expected error for empty steps")
		}
	})

	t.Run("missing_action", func(t *testing.T) {
		yamlContent := `
name: bad-test
steps:
  - timeout: 30s
`
		tmpFile := filepath.Join(t.TempDir(), "test.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		_, err := LoadTestConfig(tmpFile)
		if err == nil {
			t.Fatal("expected error for step with missing action")
		}
	})

	t.Run("zero_timeout", func(t *testing.T) {
		yamlContent := `
name: bad-test
steps:
  - action: exec
    timeout: 0s
`
		tmpFile := filepath.Join(t.TempDir(), "test.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		_, err := LoadTestConfig(tmpFile)
		if err == nil {
			t.Fatal("expected error for step with zero timeout")
		}
	})

	t.Run("missing_timeout", func(t *testing.T) {
		yamlContent := `
name: bad-test
steps:
  - action: exec
`
		tmpFile := filepath.Join(t.TempDir(), "test.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		_, err := LoadTestConfig(tmpFile)
		if err == nil {
			t.Fatal("expected error for step with missing timeout (zero value)")
		}
	})

	t.Run("missing_name", func(t *testing.T) {
		yamlContent := `
steps:
  - action: exec
    timeout: 10s
`
		tmpFile := filepath.Join(t.TempDir(), "test.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		_, err := LoadTestConfig(tmpFile)
		if err == nil {
			t.Fatal("expected error for missing name")
		}
	})

	t.Run("params_preserve_expressions_until_runtime", func(t *testing.T) {
		dir := t.TempDir()
		yamlContent := `
name: expr-test
preboot:
  steps:
    - action: efi-add-file
      params:
        src: "${{ test.dir }}/bootx64.efi"
        dst: /EFI/Boot/bootx64.efi
steps:
  - action: file-upload
    timeout: 10s
    params:
      src: "${{ test.dir }}/fixtures/setup.ps1"
`
		tmpFile := filepath.Join(dir, "test.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		cfg, err := LoadTestConfig(tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got := cfg.Steps[0].Params["src"]; got != "${{ test.dir }}/fixtures/setup.ps1" {
			t.Fatalf("src = %v, want raw expression", got)
		}
		if got := cfg.Preboot.Steps[0].Params["src"]; got != "${{ test.dir }}/bootx64.efi" {
			t.Fatalf("preboot src = %v, want raw expression", got)
		}
	})

	t.Run("preboot_defaults_timeout", func(t *testing.T) {
		yamlContent := `
name: preboot-test
preboot:
  steps:
    - action: efi-add-file
      params:
        src: ./bootx64.efi
        dst: /EFI/Boot/bootx64.efi
steps:
  - action: exec
    timeout: 10s
    params:
      cmd: echo
`
		tmpFile := filepath.Join(t.TempDir(), "test.yaml")
		os.WriteFile(tmpFile, []byte(yamlContent), 0644)

		cfg, err := LoadTestConfig(tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.Preboot.Steps) != 1 {
			t.Fatalf("len(Preboot.Steps) = %d, want 1", len(cfg.Preboot.Steps))
		}
		if cfg.Preboot.Steps[0].Timeout.Duration != 30*time.Second {
			t.Fatalf("Preboot timeout = %v, want 30s", cfg.Preboot.Steps[0].Timeout.Duration)
		}
	})
}

// --- TestProcessIncludes ---

func TestProcessIncludes(t *testing.T) {
	t.Run("include_provides_panic_steps", func(t *testing.T) {
		dir := t.TempDir()

		includeContent := `
panic:
  steps:
    - action: screenshot
      timeout: 30s
      params:
        output: /tmp/panic.png
`
		os.WriteFile(filepath.Join(dir, "include.yaml"), []byte(includeContent), 0644)

		mainContent := `
name: test-with-include
include:
  - include.yaml
steps:
  - action: exec
    timeout: 10s
    params:
      cmd: echo
`
		mainFile := filepath.Join(dir, "test.yaml")
		os.WriteFile(mainFile, []byte(mainContent), 0644)

		cfg, err := LoadTestConfig(mainFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(cfg.Panic.Steps) != 1 {
			t.Fatalf("len(Panic.Steps) = %d, want 1", len(cfg.Panic.Steps))
		}
		if cfg.Panic.Steps[0].Action != "screenshot" {
			t.Errorf("Panic.Steps[0].Action = %q, want %q", cfg.Panic.Steps[0].Action, "screenshot")
		}
	})

	t.Run("include_provides_preboot_steps", func(t *testing.T) {
		dir := t.TempDir()

		includeContent := `
preboot:
  steps:
    - action: efi-add-file
      params:
        src: "${{ test.dir }}/bootx64.efi"
        dst: /EFI/Boot/bootx64.efi
`
		os.WriteFile(filepath.Join(dir, "include.yaml"), []byte(includeContent), 0644)

		mainContent := `
name: test-with-preboot-include
include:
  - include.yaml
steps:
  - action: exec
    timeout: 10s
    params:
      cmd: echo
`
		mainFile := filepath.Join(dir, "test.yaml")
		os.WriteFile(mainFile, []byte(mainContent), 0644)

		cfg, err := LoadTestConfig(mainFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(cfg.Preboot.Steps) != 1 {
			t.Fatalf("len(Preboot.Steps) = %d, want 1", len(cfg.Preboot.Steps))
		}
		if cfg.Preboot.Steps[0].Action != "efi-add-file" {
			t.Errorf("Preboot.Steps[0].Action = %q, want %q", cfg.Preboot.Steps[0].Action, "efi-add-file")
		}
	})

	t.Run("main_panic_steps_override_include", func(t *testing.T) {
		dir := t.TempDir()

		includeContent := `
panic:
  steps:
    - action: screenshot
      timeout: 30s
`
		os.WriteFile(filepath.Join(dir, "include.yaml"), []byte(includeContent), 0644)

		mainContent := `
name: test-override
include:
  - include.yaml
steps:
  - action: exec
    timeout: 10s
    params:
      cmd: echo
panic:
  steps:
    - action: dump
      timeout: 60s
      params:
        output: /tmp/dump.bin
`
		mainFile := filepath.Join(dir, "test.yaml")
		os.WriteFile(mainFile, []byte(mainContent), 0644)

		cfg, err := LoadTestConfig(mainFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(cfg.Panic.Steps) != 1 {
			t.Fatalf("len(Panic.Steps) = %d, want 1", len(cfg.Panic.Steps))
		}
		if cfg.Panic.Steps[0].Action != "dump" {
			t.Errorf("Panic.Steps[0].Action = %q, want %q (main should override include)", cfg.Panic.Steps[0].Action, "dump")
		}
	})

	t.Run("multiple_includes_last_wins", func(t *testing.T) {
		dir := t.TempDir()

		include1 := `
panic:
  steps:
    - action: screenshot
      timeout: 30s
`
		os.WriteFile(filepath.Join(dir, "include1.yaml"), []byte(include1), 0644)

		include2 := `
panic:
  steps:
    - action: dump
      timeout: 60s
      params:
        output: /tmp/dump.bin
`
		os.WriteFile(filepath.Join(dir, "include2.yaml"), []byte(include2), 0644)

		mainContent := `
name: test-multi-include
include:
  - include1.yaml
  - include2.yaml
steps:
  - action: exec
    timeout: 10s
    params:
      cmd: echo
`
		mainFile := filepath.Join(dir, "test.yaml")
		os.WriteFile(mainFile, []byte(mainContent), 0644)

		cfg, err := LoadTestConfig(mainFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(cfg.Panic.Steps) != 1 {
			t.Fatalf("len(Panic.Steps) = %d, want 1", len(cfg.Panic.Steps))
		}
		if cfg.Panic.Steps[0].Action != "dump" {
			t.Errorf("Panic.Steps[0].Action = %q, want %q (last include should win)", cfg.Panic.Steps[0].Action, "dump")
		}
	})

	t.Run("nonexistent_include_file", func(t *testing.T) {
		dir := t.TempDir()
		mainContent := `
name: test-bad-include
include:
  - nonexistent.yaml
steps:
  - action: exec
    timeout: 10s
    params:
      cmd: echo
`
		mainFile := filepath.Join(dir, "test.yaml")
		os.WriteFile(mainFile, []byte(mainContent), 0644)

		_, err := LoadTestConfig(mainFile)
		if err == nil {
			t.Fatal("expected error for nonexistent include file")
		}
	})
}
