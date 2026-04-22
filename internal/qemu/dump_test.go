// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"context"
	"testing"
	"time"
)

func TestWaitForDumpCompletion(t *testing.T) {
	t.Run("dump_completed", func(t *testing.T) {
		machine := &Machine{
			done:      make(chan struct{}),
			listeners: make(map[int]chan QMPEvent),
		}

		go func() {
			time.Sleep(10 * time.Millisecond)
			machine.dispatchEvent(QMPEvent{Event: "DUMP_COMPLETED"})
		}()

		if err := WaitForDumpCompletion(context.Background(), machine); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("dump_failed", func(t *testing.T) {
		machine := &Machine{
			done:      make(chan struct{}),
			listeners: make(map[int]chan QMPEvent),
		}

		go func() {
			time.Sleep(10 * time.Millisecond)
			machine.dispatchEvent(QMPEvent{Event: "DUMP_FAILED"})
		}()

		if err := WaitForDumpCompletion(context.Background(), machine); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("context_canceled", func(t *testing.T) {
		machine := &Machine{
			done:      make(chan struct{}),
			listeners: make(map[int]chan QMPEvent),
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		if err := WaitForDumpCompletion(ctx, machine); err == nil {
			t.Fatal("expected error")
		}
	})
}
