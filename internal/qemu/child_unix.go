// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

//go:build !windows

package qemu

import (
	"os/exec"
	"syscall"
)

// setupChildProcess configures the command so that the child receives SIGKILL
// when the parent process dies (Linux/macOS pdeathsig).
func setupChildProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
}

// attachToJobObject is a no-op on non-Windows platforms.
// Returns a cleanup function and nil error.
func attachToJobObject(_ int) (func(), error) {
	return func() {}, nil
}
