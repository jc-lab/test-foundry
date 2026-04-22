// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"

	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/jc-lab/test-foundry/internal/logging"
)

// Machine represents a running QEMU instance with QMP communication.
type Machine struct {
	Config  *MachineConfig
	process *exec.Cmd
	qmpConn net.Conn

	mu             sync.Mutex
	reader         *bufio.Reader
	respCh         chan qmpRawMessage // command responses
	done           chan struct{}
	exitErr        error // stores the process exit error (non-nil means abnormal)
	nextListenerID int
	listeners      map[int]chan QMPEvent
}

// qmpRawMessage is used internally to classify incoming QMP JSON messages.
type qmpRawMessage struct {
	Return json.RawMessage `json:"return"`
	Error  *QMPError       `json:"error"`
	Event  string          `json:"event"`
	Data   json.RawMessage `json:"data"`
}

// QMPEvent represents a QMP asynchronous event.
type QMPEvent struct {
	Event     string          `json:"event"`
	Data      json.RawMessage `json:"data"`
	Timestamp struct {
		Seconds      int64 `json:"seconds"`
		Microseconds int64 `json:"microseconds"`
	} `json:"timestamp"`
}

// QMPResponse represents a QMP command response.
type QMPResponse struct {
	Return json.RawMessage `json:"return,omitempty"`
	Error  *QMPError       `json:"error,omitempty"`
}

// QMPError represents a QMP error.
type QMPError struct {
	Class string `json:"class"`
	Desc  string `json:"desc"`
}

// StartMachine creates and starts a new QEMU process based on the given config.
func StartMachine(ctx context.Context, config *MachineConfig) (*Machine, error) {
	args := config.BuildArgs()

	logging.Info("Starting QEMU", "path", config.QemuPath, "args", args)

	cmd := exec.CommandContext(ctx, config.QemuPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start QEMU: %w", err)
	}

	m := &Machine{
		Config:    config,
		process:   cmd,
		respCh:    make(chan qmpRawMessage, 16),
		done:      make(chan struct{}),
		listeners: make(map[int]chan QMPEvent),
	}

	// Wait for QMP socket to become available
	if err := m.connectQMP(ctx); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("failed to connect to QMP: %w", err)
	}

	// Perform QMP handshake
	if err := m.handshake(ctx); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("QMP handshake failed: %w", err)
	}

	// Start event loop
	go m.eventLoop()

	// Start process monitor
	go func() {
		m.exitErr = cmd.Wait()
		close(m.done)
	}()

	return m, nil
}

// connectQMP waits for the QMP socket to become available and connects.
func (m *Machine) connectQMP(ctx context.Context) error {
	socketPath := m.Config.QMPSocketPath

	for i := 0; i < 60; i++ { // max 30 seconds
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if _, err := os.Stat(socketPath); err == nil {
			conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
			if err == nil {
				m.qmpConn = conn
				m.reader = bufio.NewReader(conn)
				return nil
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("QMP socket did not become available: %s", socketPath)
}

// handshake performs the QMP capabilities negotiation.
func (m *Machine) handshake(ctx context.Context) error {
	// Read greeting message
	line, err := m.reader.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("failed to read QMP greeting: %w", err)
	}
	logging.Debug("QMP greeting received", "message", string(line))

	// Send qmp_capabilities
	capCmd := map[string]string{"execute": "qmp_capabilities"}
	data, err := json.Marshal(capCmd)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if _, err := m.qmpConn.Write(data); err != nil {
		return fmt.Errorf("failed to send qmp_capabilities: %w", err)
	}

	// Read response
	respLine, err := m.reader.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("failed to read qmp_capabilities response: %w", err)
	}

	var resp qmpRawMessage
	if err := json.Unmarshal(respLine, &resp); err != nil {
		return fmt.Errorf("failed to parse qmp_capabilities response: %w", err)
	}

	if resp.Error != nil {
		return &QMPCommandError{Class: resp.Error.Class, Desc: resp.Error.Desc}
	}

	logging.Debug("QMP capabilities negotiated")
	return nil
}

// eventLoop listens for QMP events and dispatches them.
func (m *Machine) eventLoop() {
	defer m.closeListeners()

	for {
		line, err := m.reader.ReadBytes('\n')
		if err != nil {
			return // connection closed
		}

		var raw qmpRawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			logging.Debug("Failed to parse QMP message", "raw", string(line))
			continue
		}

		if raw.Event != "" {
			// This is an event
			event := QMPEvent{
				Event: raw.Event,
				Data:  raw.Data,
			}
			logging.Debug("QMP event received", "event", raw.Event, "data", string(raw.Data))
			m.dispatchEvent(event)
		} else {
			// This is a command response
			select {
			case m.respCh <- raw:
			default:
				logging.Debug("QMP response channel full, dropping response")
			}
		}
	}
}

// Execute sends a QMP command and waits for the response.
func (m *Machine) Execute(ctx context.Context, command string, args map[string]interface{}) (*QMPResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build command
	cmd := map[string]interface{}{"execute": command}
	if args != nil {
		cmd["arguments"] = args
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal QMP command: %w", err)
	}
	data = append(data, '\n')

	if _, err := m.qmpConn.Write(data); err != nil {
		return nil, fmt.Errorf("failed to send QMP command: %w", err)
	}

	// Wait for response
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case raw, ok := <-m.respCh:
		if !ok {
			return nil, ErrMachineStopped
		}
		resp := &QMPResponse{
			Return: raw.Return,
			Error:  raw.Error,
		}
		if resp.Error != nil {
			return resp, &QMPCommandError{Class: resp.Error.Class, Desc: resp.Error.Desc}
		}
		return resp, nil
	case <-m.done:
		return nil, ErrMachineStopped
	}
}

// SubscribeEvents registers a realtime listener for QMP events.
// Events are delivered only while the listener is subscribed.
func (m *Machine) SubscribeEvents() (<-chan QMPEvent, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.nextListenerID
	m.nextListenerID++
	ch := make(chan QMPEvent, 1)
	m.listeners[id] = ch

	return ch, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if listener, ok := m.listeners[id]; ok {
			delete(m.listeners, id)
			close(listener)
		}
	}
}

// Done returns a channel that is closed when the QEMU process exits.
func (m *Machine) Done() <-chan struct{} {
	return m.done
}

// Wait blocks until the QEMU process exits and returns the exit error.
func (m *Machine) Wait() error {
	<-m.done
	return m.exitErr
}

// ExitError returns the process exit error after Done() is closed.
// Returns nil if the process exited normally (exit code 0) or hasn't exited yet.
func (m *Machine) ExitError() error {
	return m.exitErr
}

// IsRunning returns whether the QEMU process is still running.
func (m *Machine) IsRunning() bool {
	select {
	case <-m.done:
		return false
	default:
		return true
	}
}

// Kill forcefully terminates the QEMU process.
func (m *Machine) Kill() error {
	if m.qmpConn != nil {
		m.qmpConn.Close()
	}
	if m.process != nil && m.process.Process != nil {
		return m.process.Process.Kill()
	}
	return nil
}

func (m *Machine) dispatchEvent(event QMPEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, ch := range m.listeners {
		select {
		case ch <- event:
		default:
			logging.Debug("QMP listener channel full, dropping event", "event", event.Event, "listener_id", id)
		}
	}
}

func (m *Machine) closeListeners() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, ch := range m.listeners {
		close(ch)
		delete(m.listeners, id)
	}
}

// Shutdown sends a quit command via QMP for graceful QEMU termination.
func (m *Machine) Shutdown(ctx context.Context) error {
	_, err := m.Execute(ctx, "quit", nil)
	if err != nil {
		// If QMP fails, try to force kill
		return m.Kill()
	}

	// Wait for process to exit
	select {
	case <-m.done:
		return nil
	case <-ctx.Done():
		return m.Kill()
	}
}

// Pause sends the QMP stop command to pause the VM.
func (m *Machine) Pause(ctx context.Context) error {
	_, err := m.Execute(ctx, "stop", nil)
	return err
}
