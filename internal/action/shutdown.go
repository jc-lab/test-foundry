// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import "context"

// ShutdownAction gracefully shuts down the guest OS.
type ShutdownAction struct{}

func (a *ShutdownAction) Name() string { return "shutdown" }

func (a *ShutdownAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	var p ShutdownParams
	_ = DecodeParams(params, &p)

	return actx.Guest.Shutdown(ctx)
}
