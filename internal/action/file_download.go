// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"context"
	"fmt"
)

// FileDownloadAction downloads a file from the guest via SFTP.
type FileDownloadAction struct{}

func (a *FileDownloadAction) Name() string { return "file-download" }

func (a *FileDownloadAction) Execute(ctx context.Context, actx *ActionContext, params map[string]any) error {
	var p FileDownloadParams
	if err := DecodeParams(params, &p); err != nil {
		return fmt.Errorf("file-download: %w", err)
	}

	if p.Src == "" || p.Dst == "" {
		return fmt.Errorf("file-download: 'src' and 'dst' params are required")
	}

	return actx.Guest.FileTransport().Download(ctx, p.Src, p.Dst)
}
