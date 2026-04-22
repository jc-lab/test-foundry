// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package qemu

import (
	"fmt"
	"net"
)

// DisplayConfig holds display-related settings.
type DisplayConfig struct {
	Headless   bool // true: -display none, false: -display gtk
	VNCDisplay int  // VNC display number (포트 = 5900 + VNCDisplay)
}

// FindFreeVNCDisplay finds an available VNC display number.
// TODO: 구현 필요 사항:
//   - 5900번부터 순차적으로 포트를 시도하여 사용 가능한 포트 찾기
//   - net.Listen("tcp", ":59XX")로 포트 사용 가능 여부 확인
//   - 사용 가능한 display number 반환 (예: 포트 5901이면 display 1)
//   - 최대 100번까지 시도 (5900~5999)
func FindFreeVNCDisplay() (int, error) {
	for display := 0; display < 100; display++ {
		addr := net.JoinHostPort("", portFromDisplay(display))
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			continue
		}
		listener.Close()
		return display, nil
	}
	return 0, ErrNoFreeVNCDisplay
}

// portFromDisplay converts a VNC display number to a port string.
func portFromDisplay(display int) string {
	return fmt.Sprintf("%d", 5900+display)
}

// VNCPort returns the TCP port for the given VNC display number.
func VNCPort(display int) int {
	return 5900 + display
}
