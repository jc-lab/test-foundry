// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"context"
	"testing"
	"time"
)

func TestWaitForReset(t *testing.T) {
	t.Run("reset_event_received", func(t *testing.T) {
		machine := &Machine{
			done:      make(chan struct{}),
			listeners: make(map[int]chan QMPEvent),
		}

		go func() {
			time.Sleep(10 * time.Millisecond)
			machine.dispatchEvent(QMPEvent{Event: "RESET"})
		}()

		if err := WaitForReset(context.Background(), machine); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("context_canceled", func(t *testing.T) {
		machine := &Machine{
			done:      make(chan struct{}),
			listeners: make(map[int]chan QMPEvent),
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		err := WaitForReset(ctx, machine)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("machine_stopped", func(t *testing.T) {
		machine := &Machine{
			done:      make(chan struct{}),
			listeners: make(map[int]chan QMPEvent),
		}
		close(machine.done)

		err := WaitForReset(context.Background(), machine)
		if err != ErrMachineStopped {
			t.Fatalf("WaitForReset() error = %v, want %v", err, ErrMachineStopped)
		}
	})
}

func TestSubscribeEventsRealtimeOnly(t *testing.T) {
	machine := &Machine{
		done:      make(chan struct{}),
		listeners: make(map[int]chan QMPEvent),
	}

	machine.dispatchEvent(QMPEvent{Event: "RESET"})

	events, unsubscribe := machine.SubscribeEvents()
	defer unsubscribe()

	select {
	case event := <-events:
		t.Fatalf("unexpected stale event received: %s", event.Event)
	case <-time.After(20 * time.Millisecond):
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		machine.dispatchEvent(QMPEvent{Event: "RESET"})
	}()

	select {
	case event := <-events:
		if event.Event != "RESET" {
			t.Fatalf("event = %q, want %q", event.Event, "RESET")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for realtime event")
	}
}
