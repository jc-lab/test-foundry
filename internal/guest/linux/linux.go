// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package linux

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jc-lab/test-foundry/internal/guest"
	"github.com/jc-lab/test-foundry/internal/guest/windows/transport"
	"github.com/jc-lab/test-foundry/internal/logging"
)

// Guest implements the guest.Guest interface for Linux OS.
// 향후 구현 예정. 현재는 인터페이스 충족을 위한 stub만 제공.
type Guest struct {
	command         transport.CommandTransport
	files           transport.FileTransport
	transportShared bool
}

// Compile-time check that Guest implements guest.Guest.
var _ guest.Guest = (*Guest)(nil)

// NewLinuxGuest creates a new Guest with the given transport.
func NewLinuxGuest(command transport.CommandTransport, files transport.FileTransport) *Guest {
	return &Guest{
		command:         command,
		files:           files,
		transportShared: any(command) == any(files),
	}
}

// OSType returns "linux".
func (g *Guest) OSType() string {
	return "linux"
}

func (g *Guest) FileTransport() transport.FileTransport {
	return g.files
}

// WaitBoot waits until the Linux guest is reachable via SSH.
func (g *Guest) WaitBoot(ctx context.Context, timeout time.Duration) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Try immediately first.
	if err := g.command.Connect(timeoutCtx); err == nil {
		return nil
	} else {
		logging.Debug("WaitBoot connect attempt failed", "error", err)
	}

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("WaitBoot timed out: %w", timeoutCtx.Err())
		case <-ticker.C:
			if err := g.command.Connect(timeoutCtx); err == nil {
				return nil
			} else {
				logging.Debug("WaitBoot connect attempt failed", "error", err)
			}
		}
	}
}

// WaitReady waits until the Linux guest is fully ready.
// TODO: 향후 구현 - cloud-init 완료 감지 (cloud-init status --wait)
func (g *Guest) WaitReady(ctx context.Context, timeout time.Duration) error {
	return errors.New("not implemented")
}

// Exec runs a command on the Linux guest via SSH.
func (g *Guest) Exec(ctx context.Context, cmd string, args ...string) (*guest.ExecResult, error) {
	fullCmd := cmd
	if len(args) > 0 {
		fullCmd = cmd + " " + strings.Join(args, " ")
	}

	stdout, stderr, exitCode, err := g.command.RunCommand(ctx, fullCmd)
	if err != nil {
		return nil, err
	}

	return &guest.ExecResult{
		ExitCode: exitCode,
		Stdout:   stdout,
		Stderr:   stderr,
	}, nil
}

// Shutdown gracefully shuts down the Linux guest.
func (g *Guest) Shutdown(ctx context.Context) error {
	_, _, _, err := g.command.RunCommand(ctx, "shutdown -h now")
	if err != nil {
		if g.command.IsConnected() {
			return fmt.Errorf("shutdown command failed: %w", err)
		}
	}
	g.closeTransports()
	return nil
}

// Reboot reboots the Linux guest.
func (g *Guest) Reboot(ctx context.Context) error {
	_, _, _, err := g.command.RunCommand(ctx, "reboot")
	if err != nil {
		if g.command.IsConnected() {
			return fmt.Errorf("reboot command failed: %w", err)
		}
	}

	g.closeTransports()

	return nil
}

func (g *Guest) closeTransports() {
	if g.transportShared {
		_ = g.command.Close()
		return
	}
	_ = g.command.Close()
	_ = g.files.Close()
}
