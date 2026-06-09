// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"context"
	"fmt"
)

// ResumeAction resumes a paused VM through QMP.
type ResumeAction struct{}

func (a *ResumeAction) Name() string { return "resume" }

func (a *ResumeAction) Execute(ctx context.Context, sctx *StepContext, params map[string]any) error {
	var p ResumeParams
	_ = DecodeParams(params, &p)

	if sctx == nil || sctx.Machine == nil {
		return fmt.Errorf("resume: machine is not available")
	}
	return sctx.Machine.Resume(ctx)
}
