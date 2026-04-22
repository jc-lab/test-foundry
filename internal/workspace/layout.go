// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package workspace

import "path/filepath"

// Layout defines the directory structure and file paths within a VM context.
type Layout struct {
	Root string // VM context root directory (예: .testfoundry/win11/)
}

// NewLayout creates a Layout for the given workdir and VM name.
func NewLayout(workdir, vmName string) *Layout {
	return &Layout{
		Root: filepath.Join(workdir, vmName),
	}
}

// TestContext creates a child layout under the VM root for an individual test run.
func (l *Layout) TestContext(name string) *Layout {
	return &Layout{
		Root: filepath.Join(l.Root, "test."+name),
	}
}

// ConfigFile returns the path to config.json.
func (l *Layout) ConfigFile() string {
	return filepath.Join(l.Root, "config.json")
}

// OverlayImage returns the path to overlay.qcow2.
func (l *Layout) OverlayImage() string {
	return filepath.Join(l.Root, "overlay.qcow2")
}

// EFIVars returns the path to efivars.fd.
func (l *Layout) EFIVars() string {
	return filepath.Join(l.Root, "efivars.fd")
}

// DaemonPID returns the path to daemon.pid.
func (l *Layout) DaemonPID() string {
	return filepath.Join(l.Root, "daemon.pid")
}

// DaemonAddr returns the path to daemon.addr.
func (l *Layout) DaemonAddr() string {
	return filepath.Join(l.Root, "daemon.addr")
}

// SSHPort returns the path to ssh.port.
func (l *Layout) SSHPort() string {
	return filepath.Join(l.Root, "ssh.port")
}

// VNCPort returns the path to vnc.port.
func (l *Layout) VNCPort() string {
	return filepath.Join(l.Root, "vnc.port")
}

// QMPSocket returns the path to qmp.sock.
func (l *Layout) QMPSocket() string {
	return filepath.Join(l.Root, "qmp.sock")
}

// SerialLog returns the path to serial.log.
func (l *Layout) SerialLog() string {
	return filepath.Join(l.Root, "serial.log")
}

// TPMDir returns the path to the tpm/ directory.
func (l *Layout) TPMDir() string {
	return filepath.Join(l.Root, "tpm")
}

// TPMSocket returns the path to tpm/swtpm.sock.
func (l *Layout) TPMSocket() string {
	return filepath.Join(l.Root, "tpm", "swtpm.sock")
}

// TPMLog returns the path to tpm/swtpm.log.
func (l *Layout) TPMLog() string {
	return filepath.Join(l.Root, "tpm", "swtpm.log")
}
