// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

//go:build windows

package qemu

import (
	"fmt"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// setupChildProcess is a no-op on Windows; job assignment happens after Start.
func setupChildProcess(_ *exec.Cmd) {}

// attachToJobObject creates a Windows Job Object with KillOnJobClose set,
// assigns the given process to it, and returns a cleanup function that closes
// the job handle. When test-foundry.exe is force-killed the OS closes all its
// handles, which in turn kills every process in the job.
func attachToJobObject(pid int) (func(), error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateJobObject: %w", err)
	}

	// Enable KillOnJobClose so the OS kills all job members when the last
	// handle to the job is closed (including on unexpected parent termination).
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		windows.CloseHandle(job)
		return nil, fmt.Errorf("SetInformationJobObject: %w", err)
	}

	procHandle, err := windows.OpenProcess(windows.PROCESS_ALL_ACCESS, false, uint32(pid))
	if err != nil {
		windows.CloseHandle(job)
		return nil, fmt.Errorf("OpenProcess: %w", err)
	}
	defer windows.CloseHandle(procHandle)

	if err := windows.AssignProcessToJobObject(job, procHandle); err != nil {
		windows.CloseHandle(job)
		return nil, fmt.Errorf("AssignProcessToJobObject: %w", err)
	}

	return func() { windows.CloseHandle(job) }, nil
}
