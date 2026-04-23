// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"context"
	"fmt"
)

// PoweroffAction forcefully powers off the VM process.
type PoweroffAction struct{}

func (a *PoweroffAction) Name() string { return "poweroff" }

func (a *PoweroffAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	if actx == nil || actx.Machine == nil {
		return fmt.Errorf("poweroff: machine is not available")
	}
	return actx.Machine.Kill()
}
