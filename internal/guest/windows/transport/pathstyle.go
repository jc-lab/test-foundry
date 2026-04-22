// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package transport

import "github.com/NextronSystems/universalpath"

func guestPathStyle(os string) universalpath.Style {
	switch os {
	case "windows":
		return universalpath.Windows
	default:
		return universalpath.Unix
	}
}
