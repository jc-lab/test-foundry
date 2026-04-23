// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package preboot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/fat32"
	"github.com/diskfs/go-diskfs/partition"
	"github.com/diskfs/go-diskfs/partition/gpt"
	"github.com/jc-lab/test-foundry/internal/action"
	"github.com/jc-lab/test-foundry/internal/vfs"
)

type EFIAddFileAction struct{}

func (a *EFIAddFileAction) Name() string {
	return "efi-add-file"
}

func (a *EFIAddFileAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	var p EFIAddFileParams
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

	srcData, err := os.ReadFile(p.Src)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
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

	dstPath := normalizeEFIPath(p.Dst)
	if err := mkdirAll(fs, filepath.ToSlash(filepath.Dir(dstPath))); err != nil {
		return fmt.Errorf("failed to create destination directories: %w", err)
	}

	file, err := fs.OpenFile(dstPath, os.O_RDWR|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("failed to open destination file in EFI partition: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(srcData); err != nil {
		return fmt.Errorf("failed to write destination file in EFI partition: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func normalizeEFIPath(p string) string {
	cleaned := filepath.ToSlash(filepath.Clean("/" + strings.TrimPrefix(filepath.ToSlash(p), "/")))
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func mkdirAll(fs filesystem.FileSystem, dir string) error {
	dir = normalizeEFIPath(dir)
	if dir == "/" {
		return nil
	}

	var current string
	for _, part := range strings.Split(strings.TrimPrefix(dir, "/"), "/") {
		current = normalizeEFIPath(filepath.ToSlash(filepath.Join(current, part)))
		if err := fs.Mkdir(current); err != nil && !strings.Contains(strings.ToLower(err.Error()), "file exists") {
			return err
		}
	}
	return nil
}

func findEFIFAT32(file *vfs.QCOW2File) (filesystem.FileSystem, error) {
	table, err := partition.Read(file, 512, 512)
	if err != nil {
		return nil, fmt.Errorf("failed to read partition table: %w", err)
	}

	partitions := table.GetPartitions()
	for _, part := range partitions {
		gptPart, ok := part.(*gpt.Partition)
		if !ok || gptPart.Type != gpt.EFISystemPartition {
			continue
		}
		fs, err := fat32.Read(file, gptPart.GetSize(), gptPart.GetStart(), 512)
		if err == nil {
			return fs, nil
		}
	}

	for _, part := range partitions {
		fs, err := fat32.Read(file, part.GetSize(), part.GetStart(), 512)
		if err == nil {
			return fs, nil
		}
	}

	length, err := file.Length()
	if err == nil {
		if fs, fatErr := fat32.Read(file, length, 0, 512); fatErr == nil {
			return fs, nil
		}
	}

	return nil, fmt.Errorf("EFI FAT32 partition not found")
}
