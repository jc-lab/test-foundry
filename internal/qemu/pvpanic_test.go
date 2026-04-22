// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"context"
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
}
