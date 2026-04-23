// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jc-lab/test-foundry/internal/guest"
	"github.com/jc-lab/test-foundry/internal/guest/windows/transport"
)

// --- mockGuest implements guest.Guest for testing ---

type mockFileTransport struct {
	uploadFn   func(ctx context.Context, localPath, remotePath string) error
	downloadFn func(ctx context.Context, remotePath, localPath string) error
}

type mockGuest struct {
	mockFileTransport
	waitBootFn func(ctx context.Context, timeout time.Duration) error
	rebootFn   func(ctx context.Context) error
	execFn     func(ctx context.Context, cmd string, args ...string) (*guest.ExecResult, error)
}

func (m *mockFileTransport) Upload(ctx context.Context, localPath, remotePath string) error {
	if m.uploadFn != nil {
		return m.uploadFn(ctx, localPath, remotePath)
	}
	return nil
}

func (m *mockFileTransport) Download(ctx context.Context, remotePath, localPath string) error {
	if m.downloadFn != nil {
		return m.downloadFn(ctx, remotePath, localPath)
	}
	return nil
}

func (m *mockGuest) FileTransport() transport.FileTransport {
	return nil
}

func (m *mockGuest) WaitBoot(ctx context.Context, timeout time.Duration) error {
	if m.waitBootFn != nil {
		return m.waitBootFn(ctx, timeout)
	}
	return nil
}

func (m *mockGuest) WaitReady(ctx context.Context, timeout time.Duration) error {
	return nil
}

func (m *mockGuest) Exec(ctx context.Context, cmd string, args ...string) (*guest.ExecResult, error) {
	if m.execFn != nil {
		return m.execFn(ctx, cmd, args...)
	}
	return &guest.ExecResult{ExitCode: 0}, nil
}

func (m *mockGuest) Shutdown(ctx context.Context) error { return nil }
func (m *mockGuest) Reboot(ctx context.Context) error {
	if m.rebootFn != nil {
		return m.rebootFn(ctx)
	}
	return nil
}
func (m *mockGuest) OSType() string { return "linux" }

// --- TestNewRegistry ---

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()

	expectedActions := []string{
		"wait-boot",
		"wait-oobe",
		"file-upload",
		"file-download",
		"exec",
		"screenshot",
		"shutdown",
		"poweroff",
		"reboot",
		"wait-reset",
		"dump",
		"sleep",
		"wait-panic",
	}

	for _, name := range expectedActions {
		act, err := r.Get(name)
		if err != nil {
			t.Errorf("expected action %q to be registered, got error: %v", name, err)
			continue
		}
		if act.Name() != name {
			t.Errorf("action.Name() = %q, want %q", act.Name(), name)
		}
	}
}

func TestWaitResetAction_WithMockGuest(t *testing.T) {
	action := &WaitResetAction{}
	err := action.Execute(context.Background(), &ActionContext{}, map[string]any{
		"retry_interval": "1ms",
	})
	if err == nil {
		t.Fatal("expected error when machine is missing")
	}
}

// --- TestRegistryGet_Unknown ---

func TestRegistryGet_Unknown(t *testing.T) {
	r := NewRegistry()

	_, err := r.Get("nonexistent-action")
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}

// --- TestSleepAction ---

func TestSleepAction(t *testing.T) {
	t.Run("valid_duration", func(t *testing.T) {
		action := &SleepAction{}
		ctx := context.Background()

		start := time.Now()
		err := action.Execute(ctx, &ActionContext{}, map[string]any{
			"duration": "100ms",
		})
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if elapsed < 80*time.Millisecond {
			t.Errorf("sleep was too short: %v", elapsed)
		}
		if elapsed > 500*time.Millisecond {
			t.Errorf("sleep was too long: %v", elapsed)
		}
	})

	t.Run("invalid_duration", func(t *testing.T) {
		action := &SleepAction{}
		ctx := context.Background()

		err := action.Execute(ctx, &ActionContext{}, map[string]any{
			"duration": "notaduration",
		})
		if err == nil {
			t.Fatal("expected error for invalid duration")
		}
	})

	t.Run("missing_duration", func(t *testing.T) {
		action := &SleepAction{}
		ctx := context.Background()

		err := action.Execute(ctx, &ActionContext{}, map[string]any{})
		if err == nil {
			t.Fatal("expected error for missing duration param")
		}
	})

	t.Run("context_cancellation", func(t *testing.T) {
		action := &SleepAction{}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := action.Execute(ctx, &ActionContext{}, map[string]any{
			"duration": "10s",
		})
		if err == nil {
			t.Fatal("expected error due to context cancellation")
		}
	})
}

func TestPoweroffAction(t *testing.T) {
	action := &PoweroffAction{}

	err := action.Execute(context.Background(), &ActionContext{}, nil)
	if err == nil {
		t.Fatal("expected error when machine is missing")
	}
}

// --- TestFileUploadAction_MissingParams ---

func TestFileUploadAction_MissingParams(t *testing.T) {
	action := &FileUploadAction{}
	ctx := context.Background()

	tests := []struct {
		name   string
		params map[string]any
	}{
		{"empty_params", map[string]any{}},
		{"missing_dst", map[string]any{"src": "/local/file"}},
		{"missing_src", map[string]any{"dst": "/remote/file"}},
		{"nil_params", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := tt.params
			if params == nil {
				params = map[string]any{}
			}
			err := action.Execute(ctx, &ActionContext{}, params)
			if err == nil {
				t.Fatal("expected error for missing params")
			}
		})
	}
}

// --- TestFileDownloadAction_MissingParams ---

func TestFileDownloadAction_MissingParams(t *testing.T) {
	action := &FileDownloadAction{}
	ctx := context.Background()

	tests := []struct {
		name   string
		params map[string]any
	}{
		{"empty_params", map[string]any{}},
		{"missing_dst", map[string]any{"src": "/remote/file"}},
		{"missing_src", map[string]any{"dst": "/local/file"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Execute(ctx, &ActionContext{}, tt.params)
			if err == nil {
				t.Fatal("expected error for missing params")
			}
		})
	}
}

// --- TestExecAction_MissingCmd ---

func TestExecAction_MissingCmd(t *testing.T) {
	action := &ExecAction{}
	ctx := context.Background()

	err := action.Execute(ctx, &ActionContext{}, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing cmd param")
	}
}

func TestExecAction_WithMockGuest(t *testing.T) {
	t.Run("success_default_exit_code", func(t *testing.T) {
		mg := &mockGuest{
			execFn: func(ctx context.Context, cmd string, args ...string) (*guest.ExecResult, error) {
				return &guest.ExecResult{
					ExitCode: 0,
					Stdout:   "hello",
				}, nil
			},
		}

		action := &ExecAction{}
		ctx := context.Background()
		actx := &ActionContext{Guest: mg}

		err := action.Execute(ctx, actx, map[string]any{
			"cmd":  "echo",
			"args": []interface{}{"hello"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("expect_exit_code_match", func(t *testing.T) {
		mg := &mockGuest{
			execFn: func(ctx context.Context, cmd string, args ...string) (*guest.ExecResult, error) {
				return &guest.ExecResult{ExitCode: 0}, nil
			},
		}

		action := &ExecAction{}
		ctx := context.Background()
		actx := &ActionContext{Guest: mg}

		err := action.Execute(ctx, actx, map[string]any{
			"cmd":              "test",
			"expect_exit_code": 0,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("expect_exit_code_mismatch", func(t *testing.T) {
		mg := &mockGuest{
			execFn: func(ctx context.Context, cmd string, args ...string) (*guest.ExecResult, error) {
				return &guest.ExecResult{
					ExitCode: 1,
					Stderr:   "command failed",
				}, nil
			},
		}

		action := &ExecAction{}
		ctx := context.Background()
		actx := &ActionContext{Guest: mg}

		err := action.Execute(ctx, actx, map[string]any{
			"cmd":              "failing-cmd",
			"expect_exit_code": 0,
		})
		if err == nil {
			t.Fatal("expected error for exit code mismatch")
		}
	})

	t.Run("expect_exit_code_float64", func(t *testing.T) {
		// YAML/JSON numbers are often decoded as float64
		mg := &mockGuest{
			execFn: func(ctx context.Context, cmd string, args ...string) (*guest.ExecResult, error) {
				return &guest.ExecResult{ExitCode: 0}, nil
			},
		}

		action := &ExecAction{}
		ctx := context.Background()
		actx := &ActionContext{Guest: mg}

		err := action.Execute(ctx, actx, map[string]any{
			"cmd":              "test",
			"expect_exit_code": float64(0),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("exec_guest_error", func(t *testing.T) {
		mg := &mockGuest{
			execFn: func(ctx context.Context, cmd string, args ...string) (*guest.ExecResult, error) {
				return nil, fmt.Errorf("SSH connection failed")
			},
		}

		action := &ExecAction{}
		ctx := context.Background()
		actx := &ActionContext{Guest: mg}

		err := action.Execute(ctx, actx, map[string]any{
			"cmd": "echo",
		})
		if err == nil {
			t.Fatal("expected error when guest exec fails")
		}
	})
}

// --- TestDumpAction_MissingOutput ---

func TestDumpAction_MissingOutput(t *testing.T) {
	action := &DumpAction{}
	ctx := context.Background()

	err := action.Execute(ctx, &ActionContext{}, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing output param")
	}
}

// --- TestScreenshotAction_MissingOutput ---

func TestScreenshotAction_MissingOutput(t *testing.T) {
	action := &ScreenshotAction{}
	ctx := context.Background()

	err := action.Execute(ctx, &ActionContext{}, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing output param")
	}
}
