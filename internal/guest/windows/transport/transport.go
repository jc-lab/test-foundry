// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

// Package transport defines the abstraction for guest OS communication transports (SSH, WinRM).
package transport

import "context"

// ExecResult holds the result of a command execution on the guest.
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// Connector defines the shared lifecycle operations for guest transports.
type Connector interface {
	// Connect establishes a connection to the guest.
	Connect(ctx context.Context) error

	// Close closes the connection.
	Close() error

	// IsConnected returns whether the connection is active.
	IsConnected() bool

	// Name returns the transport name ("ssh" or "winrm").
	Name() string
}

// CommandTransport defines the transport surface used for command execution.
type CommandTransport interface {
	Connector

	// RunCommand executes a command on the guest.
	RunCommand(ctx context.Context, cmd string) (stdout, stderr string, exitCode int, err error)
}

// FileTransport defines the transport surface used for file transfers.
type FileTransport interface {
	Connector

	// Upload copies a local file to the guest.
	Upload(ctx context.Context, localPath, remotePath string) error

	// Download copies a file from the guest to local.
	Download(ctx context.Context, remotePath, localPath string) error
}

// Config holds common transport connection parameters.
type Config struct {
	OS       string // OS (windows/linux)
	Host     string // 호스트 (보통 "127.0.0.1")
	Port     int    // 포트 (SSH: 22, WinRM: 5985)
	Username string // 사용자명
	Password string // 비밀번호
	KeyFile  string // SSH 키 파일 경로 (SSH only)
	UseTLS   bool   // WinRM HTTPS 사용 여부 (WinRM only)
}
