// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"context"
	"fmt"
)

// FileUploadAction uploads a file to the guest via SFTP.
type FileUploadAction struct{}

func (a *FileUploadAction) Name() string { return "file-upload" }

func (a *FileUploadAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	var p FileUploadParams
	if err := DecodeParams(params, &p); err != nil {
		return fmt.Errorf("file-upload: %w", err)
	}

	if p.Src == "" || p.Dst == "" {
		return fmt.Errorf("file-upload: 'src' and 'dst' params are required")
	}

	return actx.Guest.FileTransport().Upload(ctx, p.Src, p.Dst)
}
