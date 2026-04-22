// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"os/exec"
	"path/filepath"
)

// Tools holds resolved QEMU binary paths that can be reused across the process.
type Tools struct {
	QemuPath    string
	QemuImgPath string
}

// ResolveTools resolves the companion qemu-img path once for a qemu-system binary.
func ResolveTools(qemuPath string) *Tools {
	return &Tools{
		QemuPath:    qemuPath,
		QemuImgPath: ResolveQemuImg(qemuPath),
	}
}

// ResolveQemuImg finds the qemu-img binary based on the qemu system binary path.
func ResolveQemuImg(qemuPath string) string {
	qemuImg := filepath.Join(filepath.Dir(qemuPath), "qemu-img")
	if _, err := exec.LookPath(qemuImg); err != nil {
		return "qemu-img" // fallback to PATH
	}
	return qemuImg
}
