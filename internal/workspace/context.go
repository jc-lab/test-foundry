// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package workspace

import (
	"encoding/json"
	"fmt"
	"io"
	"net"

	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jc-lab/test-foundry/internal/logging"
)

// VMConfig is the metadata stored in config.json within a VM context.
type VMConfig struct {
	// Image metadata
	ImagePath string    `json:"image_path"`
	ImageName string    `json:"image_name"`
	OS        string    `json:"os"`
	CreatedAt time.Time `json:"created_at"`

	// QEMU settings (copied from image config at vm-setup time)
	Firmware     string   `json:"firmware,omitempty"`
	FirmwareVars string   `json:"firmware_vars,omitempty"` // original path; actual file is layout.EFIVars()
	Memory       string   `json:"memory"`
	CPUs         int      `json:"cpus"`
	ExtraArgs    []string `json:"extra_args,omitempty"`

	// Feature flags
	TPM bool `json:"tpm"`

	// Allocated ports/display
	SSHPort    int `json:"ssh_port"`
	WinRMPort  int `json:"winrm_port,omitempty"`
	VNCDisplay int `json:"vnc_display"`

	// Connection settings (copied from image config)
	ExecMethod     string `json:"exec_method"`
	FileMethod     string `json:"file_method"`
	Username       string `json:"username"`
	Password       string `json:"password,omitempty"`
	KeyFile        string `json:"key_file,omitempty"`
	UseTLS         bool   `json:"use_tls,omitempty"`
	SSHGuestPort   int    `json:"ssh_guest_port"`
	WinRMGuestPort int    `json:"winrm_guest_port,omitempty"`
}

// CreateContext creates a new VM context directory and initializes it.
func CreateContext(layout *Layout, qemuPath string, cfg *VMConfig, baseImage string, firmwareVars string) error {
	// Check if already exists
	if _, err := os.Stat(layout.Root); err == nil {
		return fmt.Errorf("VM context already exists: %s", layout.Root)
	}

	// Create root directory
	if err := os.MkdirAll(layout.Root, 0755); err != nil {
		return fmt.Errorf("failed to create context directory: %w", err)
	}

	// Create overlay qcow2 using qemu-img
	absBase, err := filepath.Abs(baseImage)
	if err != nil {
		return fmt.Errorf("failed to resolve base image path: %w", err)
	}

	qemuImg := filepath.Join(filepath.Dir(qemuPath), "qemu-img")
	if _, err := exec.LookPath(qemuImg); err != nil {
		// Fallback: try "qemu-img" directly from PATH
		qemuImg = "qemu-img"
	}

	cmd := exec.Command(qemuImg, "create", "-f", "qcow2",
		"-b", absBase, "-F", "qcow2",
		layout.OverlayImage())
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create overlay image: %w\n%s", err, string(output))
	}

	// Copy UEFI firmware vars if specified
	if firmwareVars != "" {
		if err := copyFile(firmwareVars, layout.EFIVars()); err != nil {
			return fmt.Errorf("failed to copy firmware vars: %w", err)
		}
	}

	// Create TPM directory if enabled
	if cfg.TPM {
		if err := os.MkdirAll(layout.TPMDir(), 0755); err != nil {
			return fmt.Errorf("failed to create TPM directory: %w", err)
		}
	}

	if cfg.usesSSH() {
		sshPort, err := findFreePort()
		if err != nil {
			return fmt.Errorf("failed to allocate SSH port: %w", err)
		}
		cfg.SSHPort = sshPort
	}

	if cfg.usesWinRM() {
		winrmPort, err := findFreePort()
		if err != nil {
			return fmt.Errorf("failed to allocate WinRM port: %w", err)
		}
		cfg.WinRMPort = winrmPort
	}

	vncDisplay, err := findFreeVNCDisplay()
	if err != nil {
		return fmt.Errorf("failed to allocate VNC display: %w", err)
	}
	cfg.VNCDisplay = vncDisplay

	cfg.CreatedAt = time.Now()

	// Write config.json
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal VM config: %w", err)
	}

	if err := os.WriteFile(layout.ConfigFile(), data, 0644); err != nil {
		return fmt.Errorf("failed to write VM config: %w", err)
	}

	return nil
}

// LoadContext loads an existing VM context from config.json.
func LoadContext(layout *Layout) (*VMConfig, error) {
	data, err := os.ReadFile(layout.ConfigFile())
	if err != nil {
		return nil, fmt.Errorf("failed to load VM context (does it exist?): %w", err)
	}

	var cfg VMConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse VM config: %w", err)
	}

	return &cfg, nil
}

// DestroyContext removes the entire VM context directory.
func DestroyContext(layout *Layout) error {
	// Check if the directory exists
	if _, err := os.Stat(layout.Root); os.IsNotExist(err) {
		return fmt.Errorf("VM context does not exist: %s", layout.Root)
	}

	// Check for running daemon
	pidData, err := os.ReadFile(layout.DaemonPID())
	if err == nil {
		pid := strings.TrimSpace(string(pidData))
		logging.Warn("Daemon PID file found, attempting cleanup", "pid", pid)
		if p, err := strconv.Atoi(pid); err == nil {
			if proc, err := os.FindProcess(p); err == nil {
				_ = proc.Kill()
			}
		}
	}

	if err := os.RemoveAll(layout.Root); err != nil {
		return fmt.Errorf("failed to destroy VM context: %w", err)
	}

	return nil
}

// findFreePort finds an available TCP port.
func findFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("failed to find free port: %w", err)
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

// findFreeVNCDisplay finds an available VNC display number (port 5900+N).
func findFreeVNCDisplay() (int, error) {
	for display := 0; display < 100; display++ {
		port := 5900 + display
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			continue
		}
		listener.Close()
		return display, nil
	}
	return 0, fmt.Errorf("no free VNC display available (tried ports 5900-5999)")
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Close()
}

func (cfg *VMConfig) usesSSH() bool {
	execMethod := cfg.ExecMethod
	if execMethod == "" {
		execMethod = "ssh"
	}
	fileMethod := cfg.FileMethod
	if fileMethod == "" {
		fileMethod = execMethod
	}
	return execMethod == "ssh" || fileMethod == "ssh"
}

func (cfg *VMConfig) usesWinRM() bool {
	execMethod := cfg.ExecMethod
	if execMethod == "" {
		execMethod = "ssh"
	}
	fileMethod := cfg.FileMethod
	if fileMethod == "" {
		fileMethod = execMethod
	}
	return execMethod == "winrm" || fileMethod == "winrm"
}
