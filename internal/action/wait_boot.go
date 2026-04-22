// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"context"
	"fmt"
	"time"
)

// WaitBootAction waits until the guest OS is reachable via SSH.
type WaitBootAction struct{}

func (a *WaitBootAction) Name() string { return "wait-boot" }

func (a *WaitBootAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	var p WaitBootParams
	if err := DecodeParams(params, &p); err != nil {
		return fmt.Errorf("wait-boot: %w", err)
	}

	if p.RetryInterval == "" {
		p.RetryInterval = "5s"
	}

	if _, err := time.ParseDuration(p.RetryInterval); err != nil {
		return fmt.Errorf("wait-boot: invalid retry_interval %q: %w", p.RetryInterval, err)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Minute)
	}
	timeout := time.Until(deadline)

	return actx.Guest.WaitBoot(ctx, timeout)
}
