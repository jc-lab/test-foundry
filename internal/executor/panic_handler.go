// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package executor

import (
	"context"
	"time"

	"github.com/jc-lab/test-foundry/internal/logging"
	"github.com/jc-lab/test-foundry/internal/qemu"
)

// PanicHandler monitors for guest panic events and coordinates the response.
type PanicHandler struct {
	machine *qemu.Machine
	panicCh chan struct{} // test step runner에게 panic 발생을 알리는 채널
}

// NewPanicHandler creates a new PanicHandler.
func NewPanicHandler(machine *qemu.Machine) *PanicHandler {
	return &PanicHandler{
		machine: machine,
		panicCh: make(chan struct{}, 1),
	}
}

// PanicCh returns the channel that signals panic occurrence to the step runner.
func (h *PanicHandler) PanicCh() <-chan struct{} {
	return h.panicCh
}

// Start begins monitoring for GUEST_PANICKED events in a goroutine.
// The goroutine will exit when the context is cancelled or the machine stops.
func (h *PanicHandler) Start(ctx context.Context) {
	go func() {
		for {
			panicEvent, err := qemu.WaitForPanic(ctx, h.machine)
			if err != nil {
				// Context cancelled or machine stopped — exit the loop
				return
			}

			logging.Warn("Panic event received", "event", panicEvent)

			actionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := qemu.HandlePanicEvent(actionCtx, h.machine, panicEvent); err != nil {
				logging.Warn("Failed to apply panic event action", "action", panicEvent.Action, "error", err)
			}
			cancel()

			// Signal panic to the step runner
			close(h.panicCh)

			return
		}
	}()
}
