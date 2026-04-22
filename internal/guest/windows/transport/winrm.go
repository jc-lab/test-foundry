// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package transport

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/NextronSystems/universalpath"
	"github.com/masterzen/winrm"
)

// WinRMTransport implements Transport using WinRM.
type WinRMTransport struct {
	config    Config
	pathStyle universalpath.Style
	client    *winrm.Client
	mu        sync.Mutex
}

var _ CommandTransport = (*WinRMTransport)(nil)
var _ FileTransport = (*WinRMTransport)(nil)

// NewWinRMTransport creates a new WinRMTransport.
func NewWinRMTransport(config Config) *WinRMTransport {
	return &WinRMTransport{
		config:    config,
		pathStyle: guestPathStyle(config.OS),
	}
}

func (t *WinRMTransport) Name() string { return "winrm" }

func (t *WinRMTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	port := t.config.Port
	if port == 0 {
		if t.config.UseTLS {
			port = 5986
		} else {
			port = 5985
		}
	}

	endpoint := winrm.NewEndpoint(
		t.config.Host,
		port,
		t.config.UseTLS,
		true, // insecure (skip TLS verify for test environments)
		nil,  // CA cert
		nil,  // client cert
		nil,  // client key
		time.Duration(0),
	)

	client, err := winrm.NewClient(endpoint, t.config.Username, t.config.Password)
	if err != nil {
		return fmt.Errorf("failed to create WinRM client: %w", err)
	}

	// Test the connection with a simple command
	var stdout, stderr bytes.Buffer
	_, err = client.RunWithContext(ctx, "echo ok", &stdout, &stderr)
	if err != nil {
		return fmt.Errorf("WinRM connection test failed: %w", err)
	}

	t.client = client
	return nil
}

func (t *WinRMTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.client = nil
	return nil
}

func (t *WinRMTransport) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.client == nil {
		return false
	}

	// Quick connectivity check
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	_, err := t.client.RunWithContext(ctx, "echo ok", &stdout, &stderr)
	return err == nil
}

func (t *WinRMTransport) RunCommand(ctx context.Context, cmd string) (stdout, stderr string, exitCode int, err error) {
	t.mu.Lock()
	if t.client == nil {
		t.mu.Unlock()
		if connErr := t.Connect(ctx); connErr != nil {
			return "", "", -1, fmt.Errorf("failed to connect: %w", connErr)
		}
		t.mu.Lock()
	}
	client := t.client
	t.mu.Unlock()

	var stdoutBuf, stderrBuf bytes.Buffer
	exitCode, err = client.RunWithContext(ctx, cmd, &stdoutBuf, &stderrBuf)
	if err != nil {
		return stdoutBuf.String(), stderrBuf.String(), exitCode, fmt.Errorf("WinRM command failed: %w", err)
	}

	return stdoutBuf.String(), stderrBuf.String(), exitCode, nil
}

// Upload copies a local file to the guest via WinRM (PowerShell Base64 transfer).
// For large files this is slow; SSH/SFTP is preferred for bulk transfers.
func (t *WinRMTransport) Upload(ctx context.Context, localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read local file %s: %w", localPath, err)
	}

	t.mu.Lock()
	if t.client == nil {
		t.mu.Unlock()
		if connErr := t.Connect(ctx); connErr != nil {
			return fmt.Errorf("failed to connect: %w", connErr)
		}
		t.mu.Lock()
	}
	client := t.client
	t.mu.Unlock()

	// Normalize to Windows path separators
	remotePath = strings.ReplaceAll(remotePath, "/", "\\")

	// Ensure parent directory exists
	remoteDir := t.pathStyle.Dir(remotePath)
	remotePath = t.pathStyle.Clean(remotePath)

	mkdirCmd := fmt.Sprintf(`powershell -Command "New-Item -ItemType Directory -Force -Path '%s' | Out-Null"`, remoteDir)
	var out, errOut bytes.Buffer
	if _, err := client.RunWithContext(ctx, mkdirCmd, &out, &errOut); err != nil {
		return fmt.Errorf("failed to create remote directory: %w (%s)", err, errOut.String())
	}

	// Transfer in chunks (WinRM has command size limits)
	const chunkSize = 48000 // ~64KB base64 → ~48KB raw, safe for WinRM
	encoded := base64.StdEncoding.EncodeToString(data)

	// Write first chunk (create/overwrite file)
	for i := 0; i < len(encoded); i += chunkSize {
		end := i + chunkSize
		if end > len(encoded) {
			end = len(encoded)
		}
		chunk := encoded[i:end]

		var psCmd string
		if i == 0 {
			psCmd = fmt.Sprintf(
				`powershell -Command "[IO.File]::WriteAllBytes('%s', [Convert]::FromBase64String('%s'))"`,
				remotePath, chunk,
			)
		} else {
			// Append subsequent chunks
			psCmd = fmt.Sprintf(
				`powershell -Command "$c=[Convert]::FromBase64String('%s'); $f=[IO.File]::Open('%s','Append'); $f.Write($c,0,$c.Length); $f.Close()"`,
				chunk, remotePath,
			)
		}

		out.Reset()
		errOut.Reset()
		if _, err := client.RunWithContext(ctx, psCmd, &out, &errOut); err != nil {
			return fmt.Errorf("failed to write file chunk: %w (%s)", err, errOut.String())
		}
	}

	return nil
}

// Download copies a file from the guest to local via WinRM (PowerShell Base64 transfer).
func (t *WinRMTransport) Download(ctx context.Context, remotePath, localPath string) error {
	t.mu.Lock()
	if t.client == nil {
		t.mu.Unlock()
		if connErr := t.Connect(ctx); connErr != nil {
			return fmt.Errorf("failed to connect: %w", connErr)
		}
		t.mu.Lock()
	}
	client := t.client
	t.mu.Unlock()

	remotePath = strings.ReplaceAll(remotePath, "/", "\\")

	psCmd := fmt.Sprintf(
		`powershell -Command "[Convert]::ToBase64String([IO.File]::ReadAllBytes('%s'))"`,
		remotePath,
	)

	var stdoutBuf, stderrBuf bytes.Buffer
	exitCode, err := client.RunWithContext(ctx, psCmd, &stdoutBuf, &stderrBuf)
	if err != nil {
		return fmt.Errorf("failed to read remote file: %w (%s)", err, stderrBuf.String())
	}
	if exitCode != 0 {
		return fmt.Errorf("failed to read remote file (exit %d): %s", exitCode, stderrBuf.String())
	}

	encoded := strings.TrimSpace(stdoutBuf.String())
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("failed to decode base64 file content: %w", err)
	}

	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return fmt.Errorf("failed to create local directory %s: %w", localDir, err)
	}

	if err := os.WriteFile(localPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write local file %s: %w", localPath, err)
	}

	return nil
}
