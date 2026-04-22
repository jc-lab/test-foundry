// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"context"
	"time"
)

// WaitOOBEAction waits until Windows OOBE is completed.
type WaitOOBEAction struct{}

func (a *WaitOOBEAction) Name() string { return "wait-oobe" }

func (a *WaitOOBEAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	var p WaitOOBEParams
	_ = DecodeParams(params, &p)

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Minute)
	}
	timeout := time.Until(deadline)

	return actx.Guest.WaitReady(ctx, timeout)
}
