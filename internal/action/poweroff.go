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

func (a *PoweroffAction) Execute(ctx context.Context, sctx *StepContext, params map[string]any) error {
	if sctx == nil || sctx.Machine == nil {
		return fmt.Errorf("poweroff: machine is not available")
	}
	return sctx.Machine.Kill()
}
