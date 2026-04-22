// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"context"
	"fmt"

	"github.com/jc-lab/test-foundry/internal/logging"
)

// PanicEvent represents a pvpanic event received from QEMU.
type PanicEvent struct {
	Action string
}

// WaitForPanic monitors QMP events for GUEST_PANICKED and returns when detected.
func WaitForPanic(ctx context.Context, machine *Machine) (*PanicEvent, error) {
	events, unsubscribe := machine.SubscribeEvents()
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case event, ok := <-events:
			if !ok {
				return nil, ErrMachineStopped
			}
			if event.Event == "GUEST_PANICKED" || event.Event == "GUEST_CRASHLOADED" {
				logging.Warn("guest panic detected", "event", event.Event)
				return &PanicEvent{Action: "pause"}, nil
			}
			// Ignore non-panic events, continue listening
		case <-machine.Done():
			return nil, ErrMachineStopped
		}
	}
}

// HandlePanicEvent applies the action requested by a panic event.
func HandlePanicEvent(ctx context.Context, machine *Machine, event *PanicEvent) error {
	if event == nil {
		return fmt.Errorf("panic event is nil")
	}

	switch event.Action {
	case "", "none":
		return nil
	case "pause":
		return machine.Pause(ctx)
	default:
		return fmt.Errorf("unsupported panic action %q", event.Action)
	}
}
