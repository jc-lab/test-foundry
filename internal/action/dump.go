// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"context"
	"fmt"

	"github.com/jc-lab/test-foundry/internal/qemu"
)

// DumpAction captures a guest memory dump via QMP.
type DumpAction struct{}

func (a *DumpAction) Name() string { return "dump" }

func (a *DumpAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	var p DumpParams
	if err := DecodeParams(params, &p); err != nil {
		return fmt.Errorf("dump: %w", err)
	}

	if p.Output == "" {
		return fmt.Errorf("dump: 'output' param is required")
	}

	args := map[string]interface{}{
		"paging":   false,
		"protocol": "file:" + p.Output,
	}
	if p.Format != "" {
		args["format"] = p.Format
	}

	resp, err := actx.Machine.Execute(ctx, "dump-guest-memory", args)
	if err != nil {
		return fmt.Errorf("dump: QMP dump-guest-memory failed: %w", err)
	}
	_ = resp

	if err := qemu.WaitForDumpCompletion(ctx, actx.Machine); err != nil {
		return fmt.Errorf("dump: waiting for DUMP_COMPLETED failed: %w", err)
	}

	return nil
}
