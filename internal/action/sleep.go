// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"context"
	"fmt"
	"time"
)

// SleepAction waits for a specified duration.
type SleepAction struct{}

func (a *SleepAction) Name() string { return "sleep" }

func (a *SleepAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	var p SleepParams
	if err := DecodeParams(params, &p); err != nil {
		return fmt.Errorf("sleep: %w", err)
	}

	if p.Duration == "" {
		return fmt.Errorf("sleep: 'duration' param is required")
	}

	dur, err := time.ParseDuration(p.Duration)
	if err != nil {
		return fmt.Errorf("sleep: invalid duration %q: %w", p.Duration, err)
	}

	select {
	case <-time.After(dur):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
