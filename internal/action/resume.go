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

func (a *ResumeAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	var p ResumeParams
	_ = DecodeParams(params, &p)

	if actx == nil || actx.Machine == nil {
		return fmt.Errorf("resume: machine is not available")
	}
	return actx.Machine.Resume(ctx)
}
