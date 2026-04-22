// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package ipc

// ActionRequest represents an IPC request from the CLI to the daemon.
type ActionRequest struct {
	Action  string         `json:"action"`            // Action 이름 (예: "file-upload", "screenshot")
	Params  map[string]any `json:"params,omitempty"`  // Action 파라미터
	Timeout int            `json:"timeout,omitempty"` // Timeout in seconds (0 = unlimited)
}

// ActionResponse represents an IPC response from the daemon to the CLI.
type ActionResponse struct {
	Success bool   `json:"success"`         // 성공 여부
	Error   string `json:"error,omitempty"` // 에러 메시지
	Data    any    `json:"data,omitempty"`  // 응답 데이터 (action에 따라 다름)
}

// ExecResponseData holds the response data for an exec action.
type ExecResponseData struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// StatusResponse holds the response data for the /status endpoint.
type StatusResponse struct {
	VMName    string `json:"vm_name"`
	Status    string `json:"status"` // "running", "stopped"
	SSHPort   int    `json:"ssh_port"`
	VNCPort   int    `json:"vnc_port"`
	QMPSocket string `json:"qmp_socket"`
	TPM       bool   `json:"tpm"`
}
