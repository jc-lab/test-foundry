// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package transport

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/NextronSystems/universalpath"
	"github.com/jc-lab/test-foundry/internal/logging"
	"github.com/masterzen/winrm"
)

// WinRMTransport implements Transport using WinRM.
type WinRMTransport struct {
	config    Config
	pathStyle universalpath.Style
	client    *winrm.Client
	mu        sync.Mutex
}

// winrmProbeTimeout bounds the IsConnected probe.
const winrmProbeTimeout = 5 * time.Second

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

	return t.connectLocked(ctx)
}

// connectLocked establishes the WinRM client. The caller must hold t.mu.
// It is a no-op when a client already exists so that concurrent callers cannot
// race into creating a second one.
func (t *WinRMTransport) connectLocked(ctx context.Context) error {
	if t.client != nil {
		return nil
	}

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

// acquireClient returns a connected client, connecting on first use. The whole
// check-and-connect runs under t.mu so concurrent callers share one client.
func (t *WinRMTransport) acquireClient(ctx context.Context) (*winrm.Client, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.connectLocked(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	return t.client, nil
}

func (t *WinRMTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.client = nil
	return nil
}

func (t *WinRMTransport) IsConnected() bool {
	// The mutex is released before probing so a slow guest cannot block Close()
	// or a concurrent command for the duration of the check.
	t.mu.Lock()
	client := t.client
	t.mu.Unlock()

	if client == nil {
		return false
	}

	// Quick connectivity check
	ctx, cancel := context.WithTimeout(context.Background(), winrmProbeTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	_, err := client.RunWithContext(ctx, "echo ok", &stdout, &stderr)
	return err == nil
}

func (t *WinRMTransport) RunCommand(ctx context.Context, stdout, stderr io.Writer, cmd string) (exitCode int, err error) {
	client, err := t.acquireClient(ctx)
	if err != nil {
		return -1, err
	}

	exitCode, err = client.RunWithContext(ctx, cmd, stdout, stderr)
	if err != nil {
		if winrmErr, ok := errors.AsType[*winrm.ExecuteCommandError](err); ok {
			logging.Debug("winrm error", "body", winrmErr.Body)
		}

		return exitCode, fmt.Errorf("WinRM command failed: %w", err)
	}

	return exitCode, nil
}

// Upload copies a local file to the guest via WinRM (PowerShell Base64 transfer).
// For large files this is slow; SSH/SFTP is preferred for bulk transfers.
func (t *WinRMTransport) Upload(ctx context.Context, localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read local file %s: %w", localPath, err)
	}

	client, err := t.acquireClient(ctx)
	if err != nil {
		return err
	}

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
	client, err := t.acquireClient(ctx)
	if err != nil {
		return err
	}

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
