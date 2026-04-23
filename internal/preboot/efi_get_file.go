// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package preboot

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jc-lab/test-foundry/internal/action"
	"github.com/jc-lab/test-foundry/internal/vfs"
)

type EFIGetFileAction struct{}

func (a *EFIGetFileAction) Name() string {
	return "efi-get-file"
}

func (a *EFIGetFileAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	var p EFIGetFileParams
	if err := action.DecodeParams(params, &p); err != nil {
		return err
	}
	if p.Src == "" {
		return fmt.Errorf("missing required param: src")
	}
	if p.Dst == "" {
		return fmt.Errorf("missing required param: dst")
	}
	if actx == nil || actx.WorkDir == "" {
		return fmt.Errorf("workdir is not available in this context")
	}

	overlayPath := filepath.Join(actx.WorkDir, "overlay.qcow2")
	qcowFile, err := vfs.OpenQCOW2File(overlayPath)
	if err != nil {
		return fmt.Errorf("failed to open overlay image: %w", err)
	}
	defer qcowFile.Close()

	fs, err := findEFIFAT32(qcowFile)
	if err != nil {
		return err
	}

	srcPath := normalizeEFIPath(p.Src)
	file, err := fs.OpenFile(srcPath, os.O_RDONLY)
	if err != nil {
		return fmt.Errorf("failed to open source file in EFI partition: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read source file in EFI partition: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(p.Dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination directories: %w", err)
	}
	if err := os.WriteFile(p.Dst, data, 0644); err != nil {
		return fmt.Errorf("failed to write destination file: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
