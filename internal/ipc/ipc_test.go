// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package ipc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- TestProtocol_Serialization ---

func TestProtocol_ActionRequest_Serialization(t *testing.T) {
	req := ActionRequest{
		Action: "exec",
		Params: map[string]any{
			"cmd":  "echo",
			"args": []interface{}{"hello", "world"},
		},
		Timeout: 30,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal ActionRequest: %v", err)
	}

	var decoded ActionRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ActionRequest: %v", err)
	}

	if decoded.Action != req.Action {
		t.Errorf("Action = %q, want %q", decoded.Action, req.Action)
	}
	if decoded.Timeout != req.Timeout {
		t.Errorf("Timeout = %d, want %d", decoded.Timeout, req.Timeout)
	}
	cmd, ok := decoded.Params["cmd"].(string)
	if !ok || cmd != "echo" {
		t.Errorf("Params[cmd] = %v, want %q", decoded.Params["cmd"], "echo")
	}
}

func TestProtocol_ActionResponse_Serialization(t *testing.T) {
	t.Run("success_response", func(t *testing.T) {
		resp := ActionResponse{
			Success: true,
			Data:    map[string]any{"result": "ok"},
		}

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("failed to marshal ActionResponse: %v", err)
		}

		var decoded ActionResponse
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal ActionResponse: %v", err)
		}

		if !decoded.Success {
			t.Error("expected Success to be true")
		}
		if decoded.Error != "" {
			t.Errorf("Error = %q, want empty", decoded.Error)
		}
	})

	t.Run("error_response", func(t *testing.T) {
		resp := ActionResponse{
			Success: false,
			Error:   "command failed: exit code 1",
		}

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("failed to marshal ActionResponse: %v", err)
		}

		var decoded ActionResponse
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal ActionResponse: %v", err)
		}

		if decoded.Success {
			t.Error("expected Success to be false")
		}
		if decoded.Error != "command failed: exit code 1" {
			t.Errorf("Error = %q, want %q", decoded.Error, "command failed: exit code 1")
		}
	})
}

func TestProtocol_ExecResponseData_Serialization(t *testing.T) {
	resp := ExecResponseData{
		ExitCode: 0,
		Stdout:   "hello world\n",
		Stderr:   "",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal ExecResponseData: %v", err)
	}

	var decoded ExecResponseData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ExecResponseData: %v", err)
	}

	if decoded.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", decoded.ExitCode)
	}
	if decoded.Stdout != "hello world\n" {
		t.Errorf("Stdout = %q, want %q", decoded.Stdout, "hello world\n")
	}
	if decoded.Stderr != "" {
		t.Errorf("Stderr = %q, want empty", decoded.Stderr)
	}
}

func TestProtocol_StatusResponse_Serialization(t *testing.T) {
	resp := StatusResponse{
		VMName:    "test-vm",
		Status:    "running",
		SSHPort:   2222,
		VNCPort:   5901,
		QMPSocket: "/work/qmp.sock",
		TPM:       true,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal StatusResponse: %v", err)
	}

	var decoded StatusResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal StatusResponse: %v", err)
	}

	if decoded.VMName != "test-vm" {
		t.Errorf("VMName = %q, want %q", decoded.VMName, "test-vm")
	}
	if decoded.Status != "running" {
		t.Errorf("Status = %q, want %q", decoded.Status, "running")
	}
	if decoded.SSHPort != 2222 {
		t.Errorf("SSHPort = %d, want %d", decoded.SSHPort, 2222)
	}
	if decoded.VNCPort != 5901 {
		t.Errorf("VNCPort = %d, want %d", decoded.VNCPort, 5901)
	}
	if decoded.QMPSocket != "/work/qmp.sock" {
		t.Errorf("QMPSocket = %q, want %q", decoded.QMPSocket, "/work/qmp.sock")
	}
	if !decoded.TPM {
		t.Error("expected TPM to be true")
	}
}

// --- TestNewClient ---

func TestNewClient(t *testing.T) {
	client := NewClient("127.0.0.1:18230")

	expectedBaseURL := "http://127.0.0.1:18230"
	if client.baseURL != expectedBaseURL {
		t.Errorf("baseURL = %q, want %q", client.baseURL, expectedBaseURL)
	}
	if client.httpClient == nil {
		t.Error("httpClient should not be nil")
	}
}

func TestNewClient_DifferentAddresses(t *testing.T) {
	tests := []struct {
		addr        string
		wantBaseURL string
	}{
		{"localhost:8080", "http://localhost:8080"},
		{"0.0.0.0:9999", "http://0.0.0.0:9999"},
		{"192.168.1.100:3000", "http://192.168.1.100:3000"},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			client := NewClient(tt.addr)
			if client.baseURL != tt.wantBaseURL {
				t.Errorf("baseURL = %q, want %q", client.baseURL, tt.wantBaseURL)
			}
		})
	}
}

// --- TestNewClientFromWorkspace ---

func TestNewClientFromWorkspace(t *testing.T) {
	dir := t.TempDir()
	vmName := "test-vm"

	// Create the VM directory and daemon.addr file
	vmDir := filepath.Join(dir, vmName)
	if err := os.MkdirAll(vmDir, 0755); err != nil {
		t.Fatal(err)
	}

	addrFile := filepath.Join(vmDir, "daemon.addr")
	if err := os.WriteFile(addrFile, []byte("127.0.0.1:18230\n"), 0644); err != nil {
		t.Fatal(err)
	}

	client, err := NewClientFromWorkspace(dir, vmName)
	if err != nil {
		t.Fatalf("NewClientFromWorkspace failed: %v", err)
	}

	expectedBaseURL := "http://127.0.0.1:18230"
	if client.baseURL != expectedBaseURL {
		t.Errorf("baseURL = %q, want %q", client.baseURL, expectedBaseURL)
	}
}

func TestNewClientFromWorkspace_TrimWhitespace(t *testing.T) {
	dir := t.TempDir()
	vmName := "test-vm"

	vmDir := filepath.Join(dir, vmName)
	os.MkdirAll(vmDir, 0755)

	addrFile := filepath.Join(vmDir, "daemon.addr")
	os.WriteFile(addrFile, []byte("  127.0.0.1:12345  \n"), 0644)

	client, err := NewClientFromWorkspace(dir, vmName)
	if err != nil {
		t.Fatalf("NewClientFromWorkspace failed: %v", err)
	}

	if client.baseURL != "http://127.0.0.1:12345" {
		t.Errorf("baseURL = %q, want %q (whitespace should be trimmed)", client.baseURL, "http://127.0.0.1:12345")
	}
}

// --- TestNewClientFromWorkspace_Missing ---

func TestNewClientFromWorkspace_Missing(t *testing.T) {
	dir := t.TempDir()

	_, err := NewClientFromWorkspace(dir, "nonexistent-vm")
	if err == nil {
		t.Fatal("expected error when daemon.addr does not exist")
	}
}

func TestNewClientFromWorkspace_EmptyAddr(t *testing.T) {
	dir := t.TempDir()
	vmName := "test-vm"

	vmDir := filepath.Join(dir, vmName)
	os.MkdirAll(vmDir, 0755)

	addrFile := filepath.Join(vmDir, "daemon.addr")
	os.WriteFile(addrFile, []byte("  \n"), 0644)

	_, err := NewClientFromWorkspace(dir, vmName)
	if err == nil {
		t.Fatal("expected error when daemon.addr is empty")
	}
}
