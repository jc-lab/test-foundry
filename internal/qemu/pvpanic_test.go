// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"bufio"
	"context"
	"net"
	"testing"
)

func TestHandlePanicEvent(t *testing.T) {
	t.Run("nil_event", func(t *testing.T) {
		err := HandlePanicEvent(context.Background(), &Machine{}, nil)
		if err == nil {
			t.Fatal("expected error for nil event")
		}
	})

	t.Run("unsupported_action", func(t *testing.T) {
		err := HandlePanicEvent(context.Background(), &Machine{}, &PanicEvent{Action: "unknown"})
		if err == nil {
			t.Fatal("expected error for unsupported action")
		}
	})

	t.Run("none_action", func(t *testing.T) {
		err := HandlePanicEvent(context.Background(), &Machine{}, &PanicEvent{Action: "none"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("pause_action", func(t *testing.T) {
		machine, cleanup := newTestQMPMachine(t)
		defer cleanup()

		err := HandlePanicEvent(context.Background(), machine, &PanicEvent{Action: "pause"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("resume_action", func(t *testing.T) {
		machine, cleanup := newTestQMPMachine(t)
		defer cleanup()

		err := HandlePanicEvent(context.Background(), machine, &PanicEvent{Action: "resume"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func newTestQMPMachine(t *testing.T) (*Machine, func()) {
	t.Helper()

	client, server := net.Pipe()
	done := make(chan struct{})

	machine := &Machine{
		qmpConn: client,
		reader:  bufio.NewReader(client),
		respCh:  make(chan qmpRawMessage, 1),
		done:    done,
	}

	go func() {
		buf := make([]byte, 512)
		_, _ = server.Read(buf)
		machine.respCh <- qmpRawMessage{}
	}()

	cleanup := func() {
		_ = client.Close()
		_ = server.Close()
		close(done)
	}

	return machine, cleanup
}
