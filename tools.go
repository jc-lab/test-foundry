// Copyright 2025 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

//go:build tools
// +build tools

package tools

import (
	_ "github.com/google/addlicense"
)

//go:generate go run github.com/google/addlicense -c "JC-Lab" -l "GPL-2.0-only" -s .
