// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import "sync"

// StepContext holds per-step execution context, wrapping the shared ActionContext.
// Actions call SetOutput to record values that later steps can read via
// ${{ steps.<id>.outputs.<name> }}.
type StepContext struct {
	*ActionContext
	mu      sync.Mutex
	outputs map[string]string
}

// NewStepContext creates a StepContext for a single step execution.
func NewStepContext(actx *ActionContext) *StepContext {
	return &StepContext{ActionContext: actx}
}

// SetOutput records an output value for this step.
func (s *StepContext) SetOutput(name, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outputs == nil {
		s.outputs = make(map[string]string)
	}
	s.outputs[name] = value
}

// Outputs returns a snapshot of all recorded outputs for this step.
func (s *StepContext) Outputs() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]string, len(s.outputs))
	for k, v := range s.outputs {
		result[k] = v
	}
	return result
}
