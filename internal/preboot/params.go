// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package preboot

// EFIAddFileParams holds parameters for the efi-add-file action.
type EFIAddFileParams struct {
	Src string `yaml:"src"`
	Dst string `yaml:"dst"`
}

// EFIGetFileParams holds parameters for the efi-get-file action.
type EFIGetFileParams struct {
	Src string `yaml:"src"`
	Dst string `yaml:"dst"`
}
