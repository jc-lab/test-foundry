// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jc-lab/test-foundry/internal/qemu"
)

// --- TestNewLayout ---

func TestNewLayout(t *testing.T) {
	layout := NewLayout("/work", "myvm")

	if layout.Root != filepath.Join("/work", "myvm") {
		t.Errorf("Root = %q, want %q", layout.Root, filepath.Join("/work", "myvm"))
	}

	tests := []struct {
		name   string
		got    string
		suffix string
	}{
		{"ConfigFile", layout.ConfigFile(), "config.json"},
		{"OverlayImage", layout.OverlayImage(), "overlay.qcow2"},
		{"EFIVars", layout.EFIVars(), "efivars.fd"},
		{"DaemonPID", layout.DaemonPID(), "daemon.pid"},
		{"DaemonAddr", layout.DaemonAddr(), "daemon.addr"},
		{"SSHPort", layout.SSHPort(), "ssh.port"},
		{"VNCPort", layout.VNCPort(), "vnc.port"},
		{"QMPSocket", layout.QMPSocket(), "qmp.sock"},
		{"SerialLog", layout.SerialLog(), "serial.log"},
		{"TPMDir", layout.TPMDir(), "tpm"},
		{"TPMSocket", layout.TPMSocket(), filepath.Join("tpm", "swtpm.sock")},
		{"TPMLog", layout.TPMLog(), filepath.Join("tpm", "swtpm.log")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := filepath.Join(layout.Root, tt.suffix)
			if tt.got != want {
				t.Errorf("%s() = %q, want %q", tt.name, tt.got, want)
			}
		})
	}
}

func TestLayout_TestContext(t *testing.T) {
	layout := NewLayout("/work", "myvm")
	testLayout := layout.TestContext("case-01")

	want := filepath.Join("/work", "myvm", "test.case-01")
	if testLayout.Root != want {
		t.Fatalf("Root = %q, want %q", testLayout.Root, want)
	}
	if testLayout.OverlayImage() != filepath.Join(want, "overlay.qcow2") {
		t.Fatalf("OverlayImage() = %q", testLayout.OverlayImage())
	}
}

// --- TestCreateContext_AlreadyExists ---

func TestCreateContext_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	vmName := "existing-vm"
	layout := NewLayout(dir, vmName)

	// Pre-create the directory
	if err := os.MkdirAll(layout.Root, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &VMConfig{
		ImagePath: "/some/image.qcow2",
		ImageName: "test",
		OS:        "linux",
	}

	err := CreateContext(layout, qemu.ResolveTools("qemu-system-x86_64"), cfg, "/fake/base.qcow2", "")
	if err == nil {
		t.Fatal("expected error when context directory already exists")
	}
}

// --- TestLoadContext ---

func TestLoadContext(t *testing.T) {
	dir := t.TempDir()
	vmName := "loadtest"
	layout := NewLayout(dir, vmName)

	// Create the context directory and write config.json manually
	if err := os.MkdirAll(layout.Root, 0755); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Truncate(time.Second)
	original := &VMConfig{
		ImagePath: "/images/test.qcow2",
		ImageName: "test-image",
		OS:        "windows",
		TPM:       true,
		CreatedAt: now,
	}

	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(layout.ConfigFile(), data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadContext(layout)
	if err != nil {
		t.Fatalf("LoadContext failed: %v", err)
	}

	if loaded.ImagePath != original.ImagePath {
		t.Errorf("ImagePath = %q, want %q", loaded.ImagePath, original.ImagePath)
	}
	if loaded.ImageName != original.ImageName {
		t.Errorf("ImageName = %q, want %q", loaded.ImageName, original.ImageName)
	}
	if loaded.OS != original.OS {
		t.Errorf("OS = %q, want %q", loaded.OS, original.OS)
	}
	if loaded.TPM != original.TPM {
		t.Errorf("TPM = %v, want %v", loaded.TPM, original.TPM)
	}
	if loaded.SSHPort != 0 {
		t.Errorf("SSHPort = %d, want 0", loaded.SSHPort)
	}
	if loaded.VNCDisplay != 0 {
		t.Errorf("VNCDisplay = %d, want 0", loaded.VNCDisplay)
	}
}

func TestLoadContext_NotExists(t *testing.T) {
	dir := t.TempDir()
	layout := NewLayout(dir, "nonexistent")

	_, err := LoadContext(layout)
	if err == nil {
		t.Fatal("expected error when config.json does not exist")
	}
}

// --- TestDestroyContext ---

func TestDestroyContext(t *testing.T) {
	dir := t.TempDir()
	vmName := "destroyme"
	layout := NewLayout(dir, vmName)

	// Create the directory
	if err := os.MkdirAll(layout.Root, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a dummy file inside
	dummyFile := filepath.Join(layout.Root, "dummy.txt")
	if err := os.WriteFile(dummyFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	err := DestroyContext(layout)
	if err != nil {
		t.Fatalf("DestroyContext failed: %v", err)
	}

	if _, err := os.Stat(layout.Root); !os.IsNotExist(err) {
		t.Errorf("expected directory to be removed, but it still exists")
	}
}

func TestDestroyContext_NotExists(t *testing.T) {
	dir := t.TempDir()
	layout := NewLayout(dir, "nonexistent")

	err := DestroyContext(layout)
	if err == nil {
		t.Fatal("expected error when destroying nonexistent context")
	}
}

// --- TestFindFreePort ---

func TestFindFreePort(t *testing.T) {
	port, err := findFreePort()
	if err != nil {
		if errors.Is(err, syscall.EPERM) || strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("socket bind is not permitted in this environment: %v", err)
		}
		t.Fatalf("findFreePort failed: %v", err)
	}
	if port <= 0 {
		t.Errorf("expected port > 0, got %d", port)
	}

	// Verify we can get another port
	port2, err := findFreePort()
	if err != nil {
		t.Fatalf("second findFreePort failed: %v", err)
	}
	if port2 <= 0 {
		t.Errorf("expected port2 > 0, got %d", port2)
	}
}

// --- TestCreateContext_Success ---

func TestCreateContext_Success(t *testing.T) {
	// Check if qemu-img is available
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not found, skipping CreateContext test")
	}

	dir := t.TempDir()
	vmName := "create-test"
	layout := NewLayout(dir, vmName)

	// Create a minimal base image using qemu-img
	baseImage := filepath.Join(dir, "base.qcow2")
	cmd := exec.Command("qemu-img", "create", "-f", "qcow2", baseImage, "1G")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create base image: %v\n%s", err, string(output))
	}

	cfg := &VMConfig{
		ImagePath: baseImage,
		ImageName: "test-image",
		OS:        "linux",
		TPM:       true,
	}

	err := CreateContext(layout, qemu.ResolveTools("qemu-system-x86_64"), cfg, baseImage, "")
	if err != nil {
		t.Fatalf("CreateContext failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(layout.Root); os.IsNotExist(err) {
		t.Fatal("context directory was not created")
	}

	// Verify config.json is written and readable
	loaded, err := LoadContext(layout)
	if err != nil {
		t.Fatalf("LoadContext after CreateContext failed: %v", err)
	}

	if loaded.SSHPort != 0 {
		t.Errorf("SSHPort = %d, expected 0 in persisted config", loaded.SSHPort)
	}
	if loaded.VNCDisplay != 0 {
		t.Errorf("VNCDisplay = %d, expected 0 in persisted config", loaded.VNCDisplay)
	}

	// Verify TPM directory was created
	if _, err := os.Stat(layout.TPMDir()); os.IsNotExist(err) {
		t.Error("TPM directory was not created despite TPM=true")
	}
}

// --- TestCreateContext_WithoutTPM ---

func TestCreateContext_WithoutTPM(t *testing.T) {
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not found, skipping test")
	}

	dir := t.TempDir()
	vmName := "no-tpm-test"
	layout := NewLayout(dir, vmName)

	baseImage := filepath.Join(dir, "base.qcow2")
	cmd := exec.Command("qemu-img", "create", "-f", "qcow2", baseImage, "1G")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create base image: %v\n%s", err, string(output))
	}

	cfg := &VMConfig{
		ImagePath: baseImage,
		ImageName: "test-image",
		OS:        "linux",
		TPM:       false,
	}

	err := CreateContext(layout, qemu.ResolveTools("qemu-system-x86_64"), cfg, baseImage, "")
	if err != nil {
		t.Fatalf("CreateContext failed: %v", err)
	}

	// Verify TPM directory was NOT created
	if _, err := os.Stat(layout.TPMDir()); !os.IsNotExist(err) {
		t.Error("TPM directory should not exist when TPM=false")
	}
}

func TestAllocateRuntimeResources_ForSplitMethods(t *testing.T) {
	cfg := &VMConfig{
		OS:         "windows",
		ExecMethod: "winrm",
		FileMethod: "ssh",
	}

	if err := AllocateRuntimeResources(cfg); err != nil {
		if errors.Is(err, syscall.EPERM) || strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("runtime resource allocation is not permitted in this environment: %v", err)
		}
		t.Fatalf("AllocateRuntimeResources failed: %v", err)
	}

	if cfg.SSHPort <= 0 {
		t.Errorf("SSHPort = %d, expected > 0", cfg.SSHPort)
	}
	if cfg.WinRMPort <= 0 {
		t.Errorf("WinRMPort = %d, expected > 0", cfg.WinRMPort)
	}
	if cfg.VNCDisplay < 0 {
		t.Errorf("VNCDisplay = %d, expected >= 0", cfg.VNCDisplay)
	}
	if qemu.HostUsesQMPTCP() {
		if cfg.QMPPort <= 0 {
			t.Errorf("QMPPort = %d, expected > 0", cfg.QMPPort)
		}
	} else if cfg.QMPPort != 0 {
		t.Errorf("QMPPort = %d, expected 0 on non-Windows hosts", cfg.QMPPort)
	}
}

func TestCreateContext_DoesNotPersistRuntimePortsForSplitMethods(t *testing.T) {
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not found, skipping test")
	}

	dir := t.TempDir()
	layout := NewLayout(dir, "split-methods")

	baseImage := filepath.Join(dir, "base.qcow2")
	cmd := exec.Command("qemu-img", "create", "-f", "qcow2", baseImage, "1G")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create base image: %v\n%s", err, string(output))
	}

	cfg := &VMConfig{
		ImagePath:  baseImage,
		ImageName:  "test-image",
		OS:         "windows",
		ExecMethod: "winrm",
		FileMethod: "ssh",
	}

	if err := CreateContext(layout, qemu.ResolveTools("qemu-system-x86_64"), cfg, baseImage, ""); err != nil {
		t.Fatalf("CreateContext failed: %v", err)
	}

	loaded, err := LoadContext(layout)
	if err != nil {
		t.Fatalf("LoadContext after CreateContext failed: %v", err)
	}

	if loaded.SSHPort != 0 {
		t.Errorf("SSHPort = %d, expected 0 in persisted config", loaded.SSHPort)
	}
	if loaded.WinRMPort != 0 {
		t.Errorf("WinRMPort = %d, expected 0 in persisted config", loaded.WinRMPort)
	}
}

func TestCreateTestContext(t *testing.T) {
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not found, skipping test")
	}

	dir := t.TempDir()
	layout := NewLayout(dir, "base")
	baseImage := filepath.Join(dir, "base.qcow2")
	cmd := exec.Command("qemu-img", "create", "-f", "qcow2", baseImage, "1G")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create base image: %v\n%s", err, string(output))
	}

	cfg := &VMConfig{
		ImagePath: baseImage,
		ImageName: "test-image",
		OS:        "windows",
		TPM:       true,
	}
	if err := CreateContext(layout, qemu.ResolveTools("qemu-system-x86_64"), cfg, baseImage, ""); err != nil {
		t.Fatalf("CreateContext failed: %v", err)
	}

	if err := os.WriteFile(layout.EFIVars(), []byte("efi-vars"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.TPMDir(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.TPMDir(), "state.bin"), []byte("tpm-state"), 0644); err != nil {
		t.Fatal(err)
	}

	testLayout, err := CreateTestContext(layout, "case-01", qemu.ResolveTools("qemu-system-x86_64"), cfg)
	if err != nil {
		t.Fatalf("CreateTestContext failed: %v", err)
	}

	if _, err := os.Stat(testLayout.OverlayImage()); err != nil {
		t.Fatalf("test overlay image missing: %v", err)
	}
	absBaseOverlay, err := filepath.Abs(layout.OverlayImage())
	if err != nil {
		t.Fatalf("failed to resolve absolute base overlay path: %v", err)
	}
	backingCmd := exec.Command("qemu-img", "info", "--output=json", testLayout.OverlayImage())
	info, err := backingCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("qemu-img info failed: %v\n%s", err, string(info))
	}
	if !strings.Contains(string(info), absBaseOverlay) {
		t.Fatalf("external snapshot is not backed by base overlay: %s", string(info))
	}

	efiData, err := os.ReadFile(testLayout.EFIVars())
	if err != nil {
		t.Fatalf("failed to read copied EFI vars: %v", err)
	}
	if string(efiData) != "efi-vars" {
		t.Fatalf("EFI vars = %q, want %q", string(efiData), "efi-vars")
	}

	tpmData, err := os.ReadFile(filepath.Join(testLayout.TPMDir(), "state.bin"))
	if err != nil {
		t.Fatalf("failed to read copied TPM state: %v", err)
	}
	if string(tpmData) != "tpm-state" {
		t.Fatalf("TPM state = %q, want %q", string(tpmData), "tpm-state")
	}
}
