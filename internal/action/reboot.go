// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"context"
	"fmt"

	"github.com/jc-lab/test-foundry/internal/qemu"
)

// RebootAction reboots the guest OS and waits for SSH reconnection.
type RebootAction struct{}

func (a *RebootAction) Name() string { return "reboot" }

func (a *RebootAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	var p RebootParams
	_ = DecodeParams(params, &p)

	if err := actx.Guest.Reboot(ctx); err != nil {
		return fmt.Errorf("reboot command: %w", err)
	}

	if err := qemu.WaitForReset(ctx, actx.Machine); err != nil {
		return fmt.Errorf("wait-reset: %w", err)
	}

	return nil
}
