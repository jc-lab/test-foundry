// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"context"
	"fmt"
	"time"

	"github.com/jc-lab/test-foundry/internal/qemu"
)

// WaitResetAction waits for a QMP RESET event.
type WaitResetAction struct{}

func (a *WaitResetAction) Name() string { return "wait-reset" }

func (a *WaitResetAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	if actx.Machine == nil {
		return fmt.Errorf("wait-reset: machine is required")
	}
	if err := qemu.WaitForReset(ctx, actx.Machine); err != nil {
		return fmt.Errorf("wait-reset: %w", err)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Minute)
	}
	timeout := time.Until(deadline)
	if timeout <= 0 {
		return ctx.Err()
	}

	return actx.Guest.WaitBoot(ctx, timeout)
}
