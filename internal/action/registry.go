// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/jc-lab/test-foundry/internal/expr"
	"github.com/jc-lab/test-foundry/internal/guest"
	"github.com/jc-lab/test-foundry/internal/qemu"
)

// ActionContext holds the shared resources available to all actions during execution.
type ActionContext struct {
	Machine *qemu.Machine // QEMU 머신 인스턴스 (QMP 통신)
	Guest   guest.Guest   // Guest OS 인스턴스 (SSH 통신)
	WorkDir string        // VM context directory 경로
	TestDir string        // test YAML directory path
	OutDir  string        // test output directory

	vmConfigOnce sync.Once
	vmConfig     map[string]any
	vmConfigErr  error
}

// Action is the interface that all step actions must implement.
type Action interface {
	// Name returns the action identifier (예: "wait-boot", "file-upload").
	Name() string

	// Execute performs the action with the given parameters.
	// params는 YAML의 step.params에서 전달된 map.
	Execute(ctx context.Context, actx *ActionContext, params map[string]any) error
}

func (a *ActionContext) VMConfig() (map[string]any, error) {
	a.vmConfigOnce.Do(func() {
		if a.Machine == nil || a.Machine.Config == nil {
			a.vmConfigErr = fmt.Errorf("vm config is not available in this context")
			return
		}

		data, err := json.Marshal(a.Machine.Config)
		if err != nil {
			a.vmConfigErr = fmt.Errorf("failed to marshal machine config: %w", err)
			return
		}

		if err := json.Unmarshal(data, &a.vmConfig); err != nil {
			a.vmConfigErr = fmt.Errorf("failed to decode machine config: %w", err)
			return
		}
	})

	return a.vmConfig, a.vmConfigErr
}

// ExprContext creates an expression context for resolving test expressions.
func (a *ActionContext) ExprContext() *expr.Context {
	if a == nil {
		return &expr.Context{}
	}

	return &expr.Context{
		TestDir: a.TestDir,
		OutDir:  a.OutDir,
		VMConfig: func() (map[string]any, error) {
			return a.VMConfig()
		},
	}
}

// Registry maintains a map of action name → Action implementation.
type Registry struct {
	actions map[string]Action
}

// NewRegistry creates a new Registry with all built-in actions registered.
func NewRegistry() *Registry {
	r := &Registry{
		actions: make(map[string]Action),
	}

	r.Register(&WaitBootAction{})
	r.Register(&WaitOOBEAction{})
	r.Register(&FileUploadAction{})
	r.Register(&FileDownloadAction{})
	r.Register(&ExecAction{})
	r.Register(&ScreenshotAction{})
	r.Register(&ShutdownAction{})
	r.Register(&PoweroffAction{})
	r.Register(&RebootAction{})
	r.Register(&WaitResetAction{})
	r.Register(&DumpAction{})
	r.Register(&SleepAction{})
	r.Register(&WaitPanicAction{})

	return r
}

// Register adds an action to the registry.
func (r *Registry) Register(action Action) {
	r.actions[action.Name()] = action
}

// Get returns the action for the given name, or an error if not found.
func (r *Registry) Get(name string) (Action, error) {
	action, ok := r.actions[name]
	if !ok {
		return nil, fmt.Errorf("unknown action: %s", name)
	}
	return action, nil
}
