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
	for _, name := range []string{"efi-add-file", "efi-get-file"} {
		act, err := r.Get(name)
		if err != nil {
			t.Fatalf("expected %s to be registered: %v", name, err)
		}
		if act.Name() != name {
			t.Fatalf("action.Name() = %q, want %q", act.Name(), name)
		}
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

func TestEFIGetFileAction(t *testing.T) {
	dir := t.TempDir()
	overlay := filepath.Join(dir, "overlay.qcow2")
	dst := filepath.Join(dir, "output", "BOOTX64.EFI")
	want := []byte("hello-from-efi")

	if err := createTestESPImage(overlay); err != nil {
		t.Fatalf("failed to create test esp image: %v", err)
	}

	qcowFile, err := vfs.OpenQCOW2File(overlay)
	if err != nil {
		t.Fatalf("failed to open qcow image: %v", err)
	}
	fs, err := findEFIFAT32(qcowFile)
	if err != nil {
		t.Fatalf("failed to find EFI filesystem: %v", err)
	}
	file, err := fs.OpenFile("/EFI/Boot/BOOTX64.EFI", os.O_CREATE|os.O_RDWR|os.O_TRUNC)
	if err != nil {
		t.Fatalf("failed to create EFI source file: %v", err)
	}
	if _, err := file.Write(want); err != nil {
		t.Fatalf("failed to seed EFI source file: %v", err)
	}
	_ = file.Close()
	_ = qcowFile.Close()

	action := &EFIGetFileAction{}
	if err := action.Execute(context.Background(), &ActionContext{WorkDir: dir}, map[string]any{
		"src": "/efi/boot/bootx64.efi",
		"dst": dst,
	}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("failed to read extracted EFI file: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("extracted EFI file contents = %q, want %q", string(got), string(want))
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

func TestResolveParams_Env(t *testing.T) {
	t.Setenv("TEST_FOUNDRY_PREBOOT_ENV", "preboot-value")

	params, err := ResolveParams(map[string]any{
		"src": "${{ env.TEST_FOUNDRY_PREBOOT_ENV }}",
	}, &ActionContext{TestDir: "/tmp/example"})
	if err != nil {
		t.Fatalf("ResolveParams failed: %v", err)
	}
	if params["src"] != "preboot-value" {
		t.Fatalf("src = %v, want %q", params["src"], "preboot-value")
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
