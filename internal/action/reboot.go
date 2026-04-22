// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import "context"

// RebootAction reboots the guest OS and waits for SSH reconnection.
type RebootAction struct{}

func (a *RebootAction) Name() string { return "reboot" }

func (a *RebootAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	var p RebootParams
	_ = DecodeParams(params, &p)

	return actx.Guest.Reboot(ctx)
}
