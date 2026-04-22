// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"errors"
	"fmt"
)

var (
	// ErrNoFreeVNCDisplay is returned when no free VNC display port is available.
	ErrNoFreeVNCDisplay = errors.New("no free VNC display available (tried ports 5900-5999)")

	// ErrMachineStopped is returned when the QEMU machine has already stopped.
	ErrMachineStopped = errors.New("QEMU machine has stopped")

	// ErrQMPTimeout is returned when a QMP command times out.
	ErrQMPTimeout = errors.New("QMP command timed out")

	// ErrSnapshotFailed is returned when a snapshot operation fails.
	ErrSnapshotFailed = errors.New("snapshot operation failed")
)

// QMPCommandError wraps a QMP error response.
type QMPCommandError struct {
	Class string
	Desc  string
}

func (e *QMPCommandError) Error() string {
	return fmt.Sprintf("QMP error [%s]: %s", e.Class, e.Desc)
}
