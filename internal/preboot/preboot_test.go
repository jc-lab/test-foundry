// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package preboot

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/diskfs/go-diskfs/filesystem/fat32"
	"github.com/diskfs/go-diskfs/partition/gpt"
	"github.com/dypflying/go-qcow2lib/qcow2"
	"github.com/jc-lab/test-foundry/internal/vfs"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	act, err := r.Get("efi-add-file")
	if err != nil {
		t.Fatalf("expected efi-add-file to be registered: %v", err)
	}
	if act.Name() != "efi-add-file" {
		t.Fatalf("action.Name() = %q", act.Name())
	}
}

func TestEFIAddFileAction(t *testing.T) {
	dir := t.TempDir()
	overlay := filepath.Join(dir, "overlay.qcow2")
	src := filepath.Join(dir, "BOOTX64.EFI")
	dst := "/EFI/Boot/bootx64.efi"
	want := []byte("hello-efi")

	if err := os.WriteFile(src, want, 0644); err != nil {
		t.Fatalf("failed to write src file: %v", err)
	}

	if err := createTestESPImage(overlay); err != nil {
		t.Fatalf("failed to create test esp image: %v", err)
	}

	action := &EFIAddFileAction{}
	if err := action.Execute(context.Background(), &ActionContext{WorkDir: dir}, map[string]any{
		"src": src,
		"dst": dst,
	}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	qcowFile, err := vfs.OpenQCOW2File(overlay)
	if err != nil {
		t.Fatalf("failed to reopen qcow image: %v", err)
	}
	defer qcowFile.Close()

	fs, err := findEFIFAT32(qcowFile)
	if err != nil {
		t.Fatalf("failed to find EFI filesystem: %v", err)
	}
	file, err := fs.OpenFile(dst, os.O_RDONLY)
	if err != nil {
		t.Fatalf("failed to read EFI file: %v", err)
	}
	defer file.Close()

	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("failed to read EFI file contents: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("EFI file contents = %q, want %q", string(got), string(want))
	}
}

func TestResolveParams_TestDir(t *testing.T) {
	params, err := ResolveParams(map[string]any{
		"src": "${{ test.dir }}/BOOTX64.EFI",
	}, &ActionContext{TestDir: "/tmp/example"})
	if err != nil {
		t.Fatalf("ResolveParams failed: %v", err)
	}
	if params["src"] != "/tmp/example/BOOTX64.EFI" {
		t.Fatalf("src = %v, want %q", params["src"], "/tmp/example/BOOTX64.EFI")
	}
}

func createTestESPImage(filename string) error {
	const (
		diskSize       = int64(64 * 1024 * 1024)
		logicalBlock   = int64(512)
		partitionStart = uint64(2048)
		partitionBytes = uint64(32 * 1024 * 1024)
	)

	opts := map[string]any{
		qcow2.OPT_FMT:  "qcow2",
		qcow2.OPT_SIZE: uint64(diskSize),
	}
	if err := qcow2.Blk_Create(filename, opts); err != nil {
		return err
	}

	qcowFile, err := vfs.OpenQCOW2File(filename)
	if err != nil {
		return err
	}
	defer qcowFile.Close()

	partitionSectors := partitionBytes / uint64(logicalBlock)
	table := &gpt.Table{
		Partitions: []*gpt.Partition{
			{
				Index: 1,
				Start: partitionStart,
				End:   partitionStart + partitionSectors - 1,
				Size:  partitionBytes,
				Type:  gpt.EFISystemPartition,
				Name:  "EFI System",
			},
		},
		LogicalSectorSize:  int(logicalBlock),
		PhysicalSectorSize: int(logicalBlock),
		ProtectiveMBR:      true,
	}
	if err := table.Write(qcowFile, diskSize); err != nil {
		return err
	}

	espStart := int64(partitionStart) * logicalBlock
	fs, err := fat32.Create(qcowFile, int64(partitionBytes), espStart, logicalBlock, "TEST-ESP", true)
	if err != nil {
		return err
	}
	if err := fs.Mkdir("/EFI"); err != nil {
		return err
	}
	return fs.Mkdir("/EFI/Boot")
}
