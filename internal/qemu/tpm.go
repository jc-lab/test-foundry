// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"context"
	"fmt"
	"os"

	"os/exec"
	"time"

	"github.com/jc-lab/test-foundry/internal/logging"
)

// TPMConfig holds swtpm-related configuration.
type TPMConfig struct {
	StateDir   string
	SocketPath string
	LogPath    string
}

// TPMProcess represents a running swtpm process.
type TPMProcess struct {
	config  *TPMConfig
	process *exec.Cmd
}

// StartTPM starts the swtpm process for TPM 2.0 emulation.
func StartTPM(ctx context.Context, config *TPMConfig) (*TPMProcess, error) {
	// Ensure state directory exists
	if err := os.MkdirAll(config.StateDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create TPM state directory: %w", err)
	}

	args := []string{
		"socket",
		"--tpmstate", fmt.Sprintf("dir=%s", config.StateDir),
		"--ctrl", fmt.Sprintf("type=unixio,path=%s", config.SocketPath),
		"--tpm2",
		"--log", fmt.Sprintf("level=20,file=%s", config.LogPath),
	}

	logging.Info("Starting swtpm", "args", args)

	cmd := exec.CommandContext(ctx, "swtpm", args...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start swtpm: %w", err)
	}

	tp := &TPMProcess{
		config:  config,
		process: cmd,
	}

	// Wait for socket to appear
	for i := 0; i < 30; i++ {
		if _, err := os.Stat(config.SocketPath); err == nil {
			logging.Info("swtpm socket ready", "path", config.SocketPath)
			return tp, nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Socket didn't appear - kill and report error
	_ = cmd.Process.Kill()
	return nil, fmt.Errorf("swtpm socket did not appear: %s", config.SocketPath)
}

// Stop terminates the swtpm process gracefully.
func (t *TPMProcess) Stop() error {
	if t.process == nil || t.process.Process == nil {
		return nil
	}

	// Try SIGTERM first
	if err := t.process.Process.Signal(os.Interrupt); err != nil {
		// If signal fails, go straight to kill
		return t.process.Process.Kill()
	}

	// Wait up to 5 seconds for graceful exit
	done := make(chan error, 1)
	go func() {
		done <- t.process.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		return t.process.Process.Kill()
	}
}

// BuildQEMUArgs returns the QEMU command-line arguments for TPM passthrough.
func (t *TPMProcess) BuildQEMUArgs() []string {
	return []string{
		"-chardev", "socket,id=chrtpm,path=" + t.config.SocketPath,
		"-tpmdev", "emulator,id=tpm0,chardev=chrtpm",
		"-device", "tpm-tis,tpmdev=tpm0",
	}
}
