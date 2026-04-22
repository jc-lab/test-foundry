// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"context"
)

// WaitForReset monitors QMP events for RESET and returns when detected.
func WaitForReset(ctx context.Context, machine *Machine) error {
	events, unsubscribe := machine.SubscribeEvents()
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-events:
			if !ok {
				return ErrMachineStopped
			}
			if event.Event == "RESET" {
				return nil
			}
		case <-machine.Done():
			return ErrMachineStopped
		}
	}
}
