// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"fmt"
	"os/exec"

	"github.com/jc-lab/test-foundry/internal/logging"
)

const defaultSnapshotName = "test-foundry-ready"

// SnapshotPaths holds the file paths involved in a snapshot operation.
type SnapshotPaths struct {
	QemuImgPath  string // qemu-img binary path
	OverlayImage string // overlay.qcow2 path
	EFIVars      string // live efivars.fd path (empty if no UEFI)
	TPMStateDir  string // live tpm/ directory (empty if no TPM)
	SnapshotName string // internal snapshot tag name
}

// SaveSnapshot creates a snapshot while QEMU is **stopped**.
//
// Steps:
//  1. Create qcow2 internal snapshot via `qemu-img snapshot -c`
func SaveSnapshot(paths *SnapshotPaths) error {
	name := paths.SnapshotName
	if name == "" {
		name = defaultSnapshotName
	}

	// 1. qemu-img snapshot -c <name> <overlay.qcow2>
	logging.Info("Creating qcow2 snapshot", "name", name, "image", paths.OverlayImage)
	cmd := exec.Command(paths.QemuImgPath, "snapshot", "-c", name, paths.OverlayImage)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("qemu-img snapshot -c failed: %w\n%s", err, string(output))
	}

	logging.Info("Snapshot saved", "name", name)
	return nil
}

// RestoreSnapshot restores a previously saved snapshot while QEMU is **stopped**.
//
// Steps:
//  1. Apply qcow2 internal snapshot via `qemu-img snapshot -a`
func RestoreSnapshot(paths *SnapshotPaths) error {
	name := paths.SnapshotName
	if name == "" {
		name = defaultSnapshotName
	}

	// 1. qemu-img snapshot -a <name> <overlay.qcow2>
	logging.Info("Restoring qcow2 snapshot", "name", name, "image", paths.OverlayImage)
	cmd := exec.Command(paths.QemuImgPath, "snapshot", "-a", name, paths.OverlayImage)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("qemu-img snapshot -a failed: %w\n%s", err, string(output))
	}

	logging.Info("Snapshot restored", "name", name)
	return nil
}
