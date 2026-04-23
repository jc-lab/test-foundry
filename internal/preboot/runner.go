// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package preboot

import (
	"context"
	"fmt"
	"time"

	"github.com/jc-lab/test-foundry/internal/config"
	"github.com/jc-lab/test-foundry/internal/logging"
)

type StepResult struct {
	Action     string         `json:"action"`
	Status     string         `json:"status"`
	DurationMs int64          `json:"duration_ms"`
	Error      string         `json:"error,omitempty"`
	Params     map[string]any `json:"params,omitempty"`
}

type RunResult struct {
	Steps []StepResult `json:"steps"`
}

type Runner struct {
	registry *Registry
	actx     *ActionContext
}

func NewRunner(registry *Registry, actx *ActionContext) *Runner {
	return &Runner{registry: registry, actx: actx}
}

func (r *Runner) RunSteps(ctx context.Context, steps []config.Step) (*RunResult, error) {
	result := &RunResult{}

	for i, step := range steps {
		stepResult := StepResult{Action: step.Action}

		resolvedParams, err := ResolveParams(step.Params, r.actx)
		if err != nil {
			stepResult.Status = "failed"
			stepResult.Error = fmt.Sprintf("failed to resolve params: %v", err)
			result.Steps = append(result.Steps, stepResult)
			return result, nil
		}
		stepResult.Params = resolvedParams

		act, err := r.registry.Get(step.Action)
		if err != nil {
			stepResult.Status = "failed"
			stepResult.Error = err.Error()
			result.Steps = append(result.Steps, stepResult)
			return result, nil
		}

		stepCtx, stepCancel := context.WithTimeout(ctx, step.Timeout.Duration)
		logging.Info("Executing preboot step", "index", i, "action", step.Action, "timeout", step.Timeout.Duration)

		startTime := time.Now()
		err = act.Execute(stepCtx, r.actx, resolvedParams)
		duration := time.Since(startTime)
		stepCancel()

		stepResult.DurationMs = duration.Milliseconds()
		if err != nil {
			stepResult.Status = "failed"
			stepResult.Error = err.Error()
			result.Steps = append(result.Steps, stepResult)
			logging.Error("Preboot step failed", "index", i, "action", step.Action, "duration", duration, "error", err)
			return result, nil
		}

		stepResult.Status = "passed"
		result.Steps = append(result.Steps, stepResult)
		logging.Info("Preboot step passed", "index", i, "action", step.Action, "duration", duration)
	}

	return result, nil
}
