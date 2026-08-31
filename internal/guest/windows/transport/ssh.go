// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

const (
	// sshDialTimeout bounds the TCP dial and the SSH handshake.
	sshDialTimeout = 10 * time.Second
	// sshKeepaliveTimeout bounds the IsConnected probe.
	sshKeepaliveTimeout = 5 * time.Second
)

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

	return t.connectLocked(ctx)
}

// connectLocked establishes the SSH connection. The caller must hold t.mu.
// It is a no-op when a client already exists so that concurrent callers cannot
// race into creating (and leaking) a second connection.
func (t *SSHTransport) connectLocked(ctx context.Context) error {
	if t.client != nil {
		return nil
	}

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
		Timeout:         sshDialTimeout,
	}

	addr := net.JoinHostPort(t.config.Host, fmt.Sprintf("%d", t.config.Port))

	dialer := net.Dialer{Timeout: sshDialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to dial %s: %w", addr, err)
	}

	// ssh.NewClientConn takes no context, so the handshake and authentication
	// would otherwise block up to sshDialTimeout regardless of ctx. Closing the
	// raw connection aborts it.
	handshakeDone := make(chan struct{})
	defer close(handshakeDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-handshakeDone:
		}
	}()

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, sshConfig)
	if err != nil {
		_ = conn.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("failed to establish SSH connection: %w", err)
	}

	t.client = ssh.NewClient(sshConn, chans, reqs)
	return nil
}

// acquireClient returns a connected client, connecting on first use. The whole
// check-and-connect runs under t.mu so concurrent callers share one client.
func (t *SSHTransport) acquireClient(ctx context.Context) (*ssh.Client, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.connectLocked(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	return t.client, nil
}

// acquireSFTP returns a connected SFTP client, connecting on first use.
func (t *SSHTransport) acquireSFTP(ctx context.Context) (*sftp.Client, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.connectLocked(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	if err := t.ensureSFTPLocked(); err != nil {
		return nil, err
	}
	return t.sftp, nil
}

func (t *SSHTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	var firstErr error

	if t.sftp != nil {
		if err := t.sftp.Close(); err != nil {
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
	client := t.client
	t.mu.Unlock()

	if client == nil {
		return false
	}

	// SendRequest has no timeout of its own: on a hung guest it blocks forever.
	// The mutex is released above so a stuck probe cannot block Close().
	result := make(chan error, 1)
	go func() {
		_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
		result <- err
	}()

	timer := time.NewTimer(sshKeepaliveTimeout)
	defer timer.Stop()

	select {
	case err := <-result:
		return err == nil
	case <-timer.C:
		return false
	}
}

func (t *SSHTransport) RunCommand(ctx context.Context, stdout, stderr io.Writer, cmd string) (exitCode int, err error) {
	client, err := t.acquireClient(ctx)
	if err != nil {
		return -1, err
	}

	session, err := client.NewSession()
	if err != nil {
		return -1, fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		return -1, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := session.StderrPipe()
	if err != nil {
		return -1, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// killed records that we tore the session down because ctx was cancelled.
	// The server may still answer with an ordinary exit-status (even 0) for a
	// session we killed, so the exit status alone cannot be trusted afterwards.
	var killed atomic.Bool
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			killed.Store(true)
			_ = session.Signal(ssh.SIGKILL)
			_ = session.Close()
		case <-done:
		}
	}()

	if err := session.Start(cmd); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return -1, ctxErr
		}
		return -1, fmt.Errorf("failed to start command: %w", err)
	}

	var (
		wg      sync.WaitGroup
		copyMu  sync.Mutex
		copyErr error
	)
	copyPipe := func(dst io.Writer, src io.Reader) {
		defer wg.Done()
		if _, err := io.Copy(dst, src); err != nil {
			copyMu.Lock()
			if copyErr == nil {
				copyErr = err
			}
			copyMu.Unlock()
		}
	}
	wg.Add(2)
	go copyPipe(stdout, stdoutPipe)
	go copyPipe(stderr, stderrPipe)
	wg.Wait()

	waitErr := session.Wait()

	// A cancelled context always wins over whatever the session reported.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return -1, ctxErr
	}
	if killed.Load() {
		return -1, context.Canceled
	}

	if waitErr != nil {
		var exitErr *ssh.ExitError
		if errors.As(waitErr, &exitErr) {
			return exitErr.ExitStatus(), nil
		}
		return -1, fmt.Errorf("command execution failed: %w", waitErr)
	}

	if copyErr != nil {
		return -1, fmt.Errorf("failed to read command output: %w", copyErr)
	}

	return 0, nil
}

// ensureSFTPLocked creates the SFTP client on first use. Caller must hold t.mu.
func (t *SSHTransport) ensureSFTPLocked() error {
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
	sftpClient, err := t.acquireSFTP(ctx)
	if err != nil {
		return err
	}

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

	// Closing the remote handle unblocks a write that is stuck on a dead guest;
	// the wrapped reader stops the transfer between chunks.
	stopWatch := watchContext(ctx, remoteFile)
	defer stopWatch()

	if _, err := io.Copy(remoteFile, &ctxReader{ctx: ctx, r: localFile}); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("failed to upload file: %w", err)
	}

	return nil
}

func (t *SSHTransport) Download(ctx context.Context, remotePath, localPath string) error {
	sftpClient, err := t.acquireSFTP(ctx)
	if err != nil {
		return err
	}

	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote file %s: %w", remotePath, err)
	}
	defer remoteFile.Close()

	stopWatch := watchContext(ctx, remoteFile)
	defer stopWatch()

	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return fmt.Errorf("failed to create local directory %s: %w", localDir, err)
	}

	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file %s: %w", localPath, err)
	}
	defer localFile.Close()

	if _, err := io.Copy(localFile, &ctxReader{ctx: ctx, r: remoteFile}); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("failed to download file: %w", err)
	}

	return nil
}

// ctxReader aborts an io.Copy between chunks once ctx is done.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// watchContext closes c when ctx is done, so a transfer blocked inside a single
// read or write on an unresponsive guest is unblocked too. The returned func
// stops the watcher and must be called when the transfer finishes.
func watchContext(ctx context.Context, c io.Closer) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = c.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}
