// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"context"

	"github.com/jc-lab/test-foundry/internal/qemu"
)

// WaitPanicAction waits for a pvpanic event from the guest (BSOD/bugcheck).
type WaitPanicAction struct{}

func (a *WaitPanicAction) Name() string { return "wait-panic" }

func (a *WaitPanicAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	var p WaitPanicParams
	_ = DecodeParams(params, &p)

	_, err := qemu.WaitForPanic(ctx, actx.Machine)
	return err
}
