// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package windows

import "testing"

func TestIsOOBEComplete(t *testing.T) {
	tests := []struct {
		name    string
		stdout  string
		want    bool
		wantErr bool
	}{
		{
			name: "complete",
			stdout: `
SystemSetupInProgress=0
OOBEInProgress=0
ImageState=IMAGE_STATE_COMPLETE
`,
			want: true,
		},
		{
			name: "system_setup_in_progress",
			stdout: `
SystemSetupInProgress=1
OOBEInProgress=0
ImageState=IMAGE_STATE_COMPLETE
`,
			want: false,
		},
		{
			name: "oobe_in_progress",
			stdout: `
SystemSetupInProgress=0
OOBEInProgress=1
ImageState=IMAGE_STATE_COMPLETE
`,
			want: false,
		},
		{
			name: "image_state_not_complete",
			stdout: `
SystemSetupInProgress=0
OOBEInProgress=0
ImageState=IMAGE_STATE_UNDEPLOYABLE
`,
			want: false,
		},
		{
			name: "missing_key",
			stdout: `
SystemSetupInProgress=0
ImageState=IMAGE_STATE_COMPLETE
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isOOBEComplete(tt.stdout)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("isOOBEComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}
