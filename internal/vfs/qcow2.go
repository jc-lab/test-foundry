// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package vfs

import (
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"os"

	"github.com/diskfs/go-diskfs/backend"
	"github.com/dypflying/go-qcow2lib/qcow2"
)

const qcow2LogicalSectorSize = 512

type QCOW2File struct {
	root   *qcow2.BdrvChild
	offset int64
}

func (f *QCOW2File) Stat() (iofs.FileInfo, error) {
	return nil, errors.New("no implemented")
}

func (f *QCOW2File) Read(bytes []byte) (int, error) {
	return 0, errors.New("no implemented")
}

func (f *QCOW2File) Sys() (*os.File, error) {
	return nil, errors.New("no implemented")
}

func (f *QCOW2File) Writable() (backend.WritableFile, error) {
	return f, nil
}

func OpenQCOW2File(filename string) (*QCOW2File, error) {
	root, err := qcow2.Blk_Open(filename, map[string]any{
		qcow2.OPT_FMT: "qcow2",
	}, qcow2.BDRV_O_RDWR)
	if err != nil {
		return nil, err
	}
	return &QCOW2File{root: root}, nil
}

func (f *QCOW2File) Close() error {
	qcow2.Blk_Close(f.root)
	return nil
}

func (f *QCOW2File) Length() (int64, error) {
	length, err := qcow2.Blk_Getlength(f.root)
	if err != nil {
		return 0, err
	}
	return int64(length), nil
}

func (f *QCOW2File) ReadAt(p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	alignedStart := alignDown(off, qcow2LogicalSectorSize)
	alignedEnd := alignUp(off+int64(len(p)), qcow2LogicalSectorSize)
	buf := make([]byte, alignedEnd-alignedStart)
	n, err := qcow2.Blk_Pread(f.root, uint64(alignedStart), buf, uint64(len(buf)))
	if err != nil {
		return int(n), err
	}
	copy(p, buf[off-alignedStart:off-alignedStart+int64(len(p))])
	return len(p), nil
}

func (f *QCOW2File) WriteAt(p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	alignedStart := alignDown(off, qcow2LogicalSectorSize)
	alignedEnd := alignUp(off+int64(len(p)), qcow2LogicalSectorSize)
	buf := make([]byte, alignedEnd-alignedStart)
	if _, err := qcow2.Blk_Pread(f.root, uint64(alignedStart), buf, uint64(len(buf))); err != nil {
		return 0, err
	}
	copy(buf[off-alignedStart:off-alignedStart+int64(len(p))], p)
	n, err := qcow2.Blk_Pwrite(f.root, uint64(alignedStart), buf, uint64(len(buf)), 0)
	if err != nil {
		return int(n), err
	}
	return len(p), nil
}

func (f *QCOW2File) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		f.offset = offset
	case io.SeekCurrent:
		f.offset += offset
	case io.SeekEnd:
		length, err := f.Length()
		if err != nil {
			return 0, err
		}
		f.offset = length + offset
	default:
		return 0, fmt.Errorf("invalid seek whence %d", whence)
	}

	if f.offset < 0 {
		return 0, fmt.Errorf("negative seek offset")
	}
	return f.offset, nil
}

func (f *QCOW2File) Path() string {
	return ""
}

func alignDown(value int64, alignment int64) int64 {
	return value / alignment * alignment
}

func alignUp(value int64, alignment int64) int64 {
	if value%alignment == 0 {
		return value
	}
	return alignDown(value, alignment) + alignment
}
