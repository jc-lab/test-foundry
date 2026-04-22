// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"context"
	"fmt"

	"github.com/jc-lab/test-foundry/internal/qemu"
	"github.com/jc-lab/test-foundry/internal/vnc"
)

// ScreenshotAction captures a screenshot from the VM display via VNC.
type ScreenshotAction struct{}

func (a *ScreenshotAction) Name() string { return "screenshot" }

func (a *ScreenshotAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	var p ScreenshotParams
	if err := DecodeParams(params, &p); err != nil {
		return fmt.Errorf("screenshot: %w", err)
	}

	if p.Output == "" {
		return fmt.Errorf("screenshot: 'output' param is required")
	}

	vncPort := qemu.VNCPort(actx.Machine.Config.VNCDisplay)
	return vnc.SaveScreenshotPNG(ctx, "127.0.0.1", vncPort, p.Output)
}
