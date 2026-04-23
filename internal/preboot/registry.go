// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package preboot

import (
	"context"
	"fmt"
)

type ActionContext struct {
	WorkDir string
	TestDir string
}

type Action interface {
	Name() string
	Execute(ctx context.Context, actx *ActionContext, params map[string]any) error
}

type Registry struct {
	actions map[string]Action
}

func NewRegistry() *Registry {
	r := &Registry{actions: make(map[string]Action)}
	r.Register(&EFIAddFileAction{})
	return r
}

func (r *Registry) Register(action Action) {
	r.actions[action.Name()] = action
}

func (r *Registry) Get(name string) (Action, error) {
	action, ok := r.actions[name]
	if !ok {
		return nil, fmt.Errorf("unknown preboot action: %s", name)
	}
	return action, nil
}
