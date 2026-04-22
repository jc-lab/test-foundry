// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/jc-lab/test-foundry/internal/logging"
)

const defaultSnapshotName = "test-foundry-ready"

// SnapshotPaths holds the file paths involved in a snapshot operation.
type SnapshotPaths struct {
	QemuImgPath  string // qemu-img binary path
	OverlayImage string // overlay.qcow2 path
	EFIVars      string // live efivars.fd path (empty if no UEFI)
	TPMStateDir  string // live tpm/ directory (empty if no TPM)
	SnapshotDir  string // snapshot/ directory for saved state
	SnapshotName string // internal snapshot tag name
}

// SaveSnapshot creates a snapshot while QEMU is **stopped**.
//
// Steps:
//  1. Create snapshot directory
//  2. Create qcow2 internal snapshot via `qemu-img snapshot -c`
//  3. Copy efivars.fd to snapshot/ (if exists)
//  4. Copy TPM state directory to snapshot/ (if exists)
func SaveSnapshot(paths *SnapshotPaths) error {
	name := paths.SnapshotName
	if name == "" {
		name = defaultSnapshotName
	}

	// 1. Create snapshot directory
	if err := os.MkdirAll(paths.SnapshotDir, 0755); err != nil {
		return fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	// 2. qemu-img snapshot -c <name> <overlay.qcow2>
	logging.Info("Creating qcow2 snapshot", "name", name, "image", paths.OverlayImage)
	cmd := exec.Command(paths.QemuImgPath, "snapshot", "-c", name, paths.OverlayImage)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("qemu-img snapshot -c failed: %w\n%s", err, string(output))
	}

	// 3. Copy efivars.fd
	if paths.EFIVars != "" {
		if _, err := os.Stat(paths.EFIVars); err == nil {
			dst := filepath.Join(paths.SnapshotDir, "efivars.fd")
			logging.Debug("Saving EFI vars snapshot", "src", paths.EFIVars, "dst", dst)
			if err := copyFileSync(paths.EFIVars, dst); err != nil {
				return fmt.Errorf("failed to snapshot efivars: %w", err)
			}
		}
	}

	// 4. Copy TPM state directory
	if paths.TPMStateDir != "" {
		if info, err := os.Stat(paths.TPMStateDir); err == nil && info.IsDir() {
			dst := filepath.Join(paths.SnapshotDir, "tpm")
			logging.Debug("Saving TPM state snapshot", "src", paths.TPMStateDir, "dst", dst)
			if err := copyDir(paths.TPMStateDir, dst); err != nil {
				return fmt.Errorf("failed to snapshot TPM state: %w", err)
			}
		}
	}

	logging.Info("Snapshot saved", "name", name)
	return nil
}

// RestoreSnapshot restores a previously saved snapshot while QEMU is **stopped**.
//
// Steps:
//  1. Apply qcow2 internal snapshot via `qemu-img snapshot -a`
//  2. Restore efivars.fd from snapshot/ (if exists)
//  3. Restore TPM state directory from snapshot/ (if exists)
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

	// 2. Restore efivars.fd
	snapEFI := filepath.Join(paths.SnapshotDir, "efivars.fd")
	if paths.EFIVars != "" {
		if _, err := os.Stat(snapEFI); err == nil {
			logging.Debug("Restoring EFI vars from snapshot", "src", snapEFI, "dst", paths.EFIVars)
			if err := copyFileSync(snapEFI, paths.EFIVars); err != nil {
				return fmt.Errorf("failed to restore efivars: %w", err)
			}
		}
	}

	// 3. Restore TPM state directory
	snapTPM := filepath.Join(paths.SnapshotDir, "tpm")
	if paths.TPMStateDir != "" {
		if info, err := os.Stat(snapTPM); err == nil && info.IsDir() {
			logging.Debug("Restoring TPM state from snapshot", "src", snapTPM, "dst", paths.TPMStateDir)
			// Remove current TPM state and replace
			_ = os.RemoveAll(paths.TPMStateDir)
			if err := copyDir(snapTPM, paths.TPMStateDir); err != nil {
				return fmt.Errorf("failed to restore TPM state: %w", err)
			}
		}
	}

	logging.Info("Snapshot restored", "name", name)
	return nil
}

// ResolveQemuImg finds the qemu-img binary based on the qemu system binary path.
func ResolveQemuImg(qemuPath string) string {
	qemuImg := filepath.Join(filepath.Dir(qemuPath), "qemu-img")
	if _, err := exec.LookPath(qemuImg); err != nil {
		return "qemu-img" // fallback to PATH
	}
	return qemuImg
}

// copyFileSync copies a file from src to dst, ensuring data is fsynced.
func copyFileSync(src, dst string) error {
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

	return out.Sync()
}

// copyDir recursively copies a directory tree from src to dst.
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFileSync(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}
