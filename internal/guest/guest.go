// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package guest

import (
	"context"
	"time"

	"github.com/jc-lab/test-foundry/internal/guest/windows/transport"
)

// ExecResult holds the result of a command execution on the guest.
type ExecResult struct {
	ExitCode int    // 프로세스 종료 코드
	Stdout   string // 표준 출력
	Stderr   string // 표준 에러
}

// Guest defines the interface for interacting with a guest OS.
// 각 Guest OS (Windows, Linux 등)는 이 인터페이스를 구현해야 한다.
type Guest interface {
	// OSType returns the guest OS type identifier ("windows", "linux").
	OSType() string

	FileTransport() transport.FileTransport

	// WaitBoot waits until the guest OS is reachable via SSH.
	// SSH 연결이 성공할 때까지 retryInterval 간격으로 재시도한다.
	WaitBoot(ctx context.Context, timeout time.Duration) error

	// WaitReady waits until the guest OS setup is fully complete.
	// Windows의 경우 OOBE 완료, Linux의 경우 cloud-init 완료 등.
	WaitReady(ctx context.Context, timeout time.Duration) error

	// Exec runs a command on the guest and returns the result.
	Exec(ctx context.Context, cmd string, args ...string) (*ExecResult, error)

	// Shutdown gracefully shuts down the guest OS.
	Shutdown(ctx context.Context) error

	// Reboot reboots the guest OS.
	Reboot(ctx context.Context) error
}
