// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package transport

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/NextronSystems/universalpath"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SSHTransport implements Transport using SSH + SFTP.
type SSHTransport struct {
	config    Config
	pathStyle universalpath.Style
	client    *ssh.Client
	sftp      *sftp.Client
	mu        sync.Mutex
}

var _ CommandTransport = (*SSHTransport)(nil)
var _ FileTransport = (*SSHTransport)(nil)

// NewSSHTransport creates a new SSHTransport.
func NewSSHTransport(config Config) *SSHTransport {
	return &SSHTransport{
		config:    config,
		pathStyle: guestPathStyle(config.OS),
	}
}

func (t *SSHTransport) Name() string { return "ssh" }

func (t *SSHTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	var authMethods []ssh.AuthMethod

	if t.config.KeyFile != "" {
		keyData, err := os.ReadFile(t.config.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to read key file %s: %w", t.config.KeyFile, err)
		}
		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			return fmt.Errorf("failed to parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else if t.config.Password != "" {
		authMethods = append(authMethods, ssh.Password(t.config.Password))
	}

	sshConfig := &ssh.ClientConfig{
		User:            t.config.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(t.config.Host, fmt.Sprintf("%d", t.config.Port))

	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to dial %s: %w", addr, err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, sshConfig)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to establish SSH connection: %w", err)
	}

	t.client = ssh.NewClient(sshConn, chans, reqs)
	return nil
}

func (t *SSHTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	var firstErr error

	if t.sftp != nil {
		if err := t.sftp.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		t.sftp = nil
	}

	if t.client != nil {
		if err := t.client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		t.client = nil
	}

	return firstErr
}

func (t *SSHTransport) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.client == nil {
		return false
	}

	_, _, err := t.client.SendRequest("keepalive@openssh.com", true, nil)
	return err == nil
}

func (t *SSHTransport) RunCommand(ctx context.Context, cmd string) (stdout, stderr string, exitCode int, err error) {
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

	session, err := client.NewSession()
	if err != nil {
		return "", "", -1, fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		return "", "", -1, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := session.StderrPipe()
	if err != nil {
		return "", "", -1, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Signal(ssh.SIGKILL)
			session.Close()
		case <-done:
		}
	}()

	if err := session.Start(cmd); err != nil {
		return "", "", -1, fmt.Errorf("failed to start command: %w", err)
	}

	stdoutBytes, _ := io.ReadAll(stdoutPipe)
	stderrBytes, _ := io.ReadAll(stderrPipe)

	exitCode = 0
	if err := session.Wait(); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			if ctx.Err() != nil {
				return string(stdoutBytes), string(stderrBytes), -1, ctx.Err()
			}
			return string(stdoutBytes), string(stderrBytes), -1, fmt.Errorf("command execution failed: %w", err)
		}
	}

	return string(stdoutBytes), string(stderrBytes), exitCode, nil
}

func (t *SSHTransport) ensureSFTP() error {
	if t.sftp != nil {
		return nil
	}
	if t.client == nil {
		return fmt.Errorf("SSH client is not connected")
	}
	sftpClient, err := sftp.NewClient(t.client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	t.sftp = sftpClient
	return nil
}

func (t *SSHTransport) Upload(ctx context.Context, localPath, remotePath string) error {
	t.mu.Lock()
	if t.client == nil {
		t.mu.Unlock()
		if err := t.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		t.mu.Lock()
	}
	if err := t.ensureSFTP(); err != nil {
		t.mu.Unlock()
		return err
	}
	sftpClient := t.sftp
	t.mu.Unlock()

	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file %s: %w", localPath, err)
	}
	defer localFile.Close()

	remoteDir := t.pathStyle.Dir(remotePath)
	if err := sftpClient.MkdirAll(remoteDir); err != nil {
		return fmt.Errorf("failed to create remote directory %s: %w", remoteDir, err)
	}

	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file %s: %w", remotePath, err)
	}
	defer remoteFile.Close()

	if _, err := io.Copy(remoteFile, localFile); err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	return nil
}

func (t *SSHTransport) Download(ctx context.Context, remotePath, localPath string) error {
	t.mu.Lock()
	if t.client == nil {
		t.mu.Unlock()
		if err := t.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		t.mu.Lock()
	}
	if err := t.ensureSFTP(); err != nil {
		t.mu.Unlock()
		return err
	}
	sftpClient := t.sftp
	t.mu.Unlock()

	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote file %s: %w", remotePath, err)
	}
	defer remoteFile.Close()

	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return fmt.Errorf("failed to create local directory %s: %w", localDir, err)
	}

	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file %s: %w", localPath, err)
	}
	defer localFile.Close()

	if _, err := io.Copy(localFile, remoteFile); err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}

	return nil
}
