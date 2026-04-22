// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"context"
	"fmt"
)

// WaitForDumpCompletion monitors QMP events until dump completion is reported.
func WaitForDumpCompletion(ctx context.Context, machine *Machine) error {
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
			switch event.Event {
			case "DUMP_COMPLETED":
				return nil
			case "DUMP_FAILED":
				return fmt.Errorf("guest dump failed")
			}
		case <-machine.Done():
			return ErrMachineStopped
		}
	}
}
