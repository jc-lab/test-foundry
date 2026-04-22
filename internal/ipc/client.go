// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Client communicates with the IPC daemon via HTTP.
type Client struct {
	baseURL    string // 예: "http://127.0.0.1:18230"
	httpClient *http.Client
}

// NewClient creates a new IPC client from the daemon address.
func NewClient(addr string) *Client {
	return &Client{
		baseURL:    "http://" + addr,
		httpClient: &http.Client{},
	}
}

// NewClientFromWorkspace creates a new IPC client by reading daemon.addr from the workspace.
func NewClientFromWorkspace(workdir, vmName string) (*Client, error) {
	addrFile := filepath.Join(workdir, vmName, "daemon.addr")
	data, err := os.ReadFile(addrFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read daemon address (is the daemon running?): %w", err)
	}

	addr := strings.TrimSpace(string(data))
	if addr == "" {
		return nil, fmt.Errorf("daemon address file is empty: %s", addrFile)
	}

	return NewClient(addr), nil
}

// Status calls GET /status and returns the VM status.
func (c *Client) Status(ctx context.Context) (*StatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/status", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create status request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call /status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status request failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var status StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode status response: %w", err)
	}

	return &status, nil
}

// ExecuteAction calls POST /action/{name} with the given parameters.
func (c *Client) ExecuteAction(ctx context.Context, name string, params map[string]any, timeout int) (*ActionResponse, error) {
	reqBody := ActionRequest{
		Action:  name,
		Params:  params,
		Timeout: timeout,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal action request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/action/"+name, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create action request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call /action/%s: %w", name, err)
	}
	defer resp.Body.Close()

	var actionResp ActionResponse
	if err := json.NewDecoder(resp.Body).Decode(&actionResp); err != nil {
		return nil, fmt.Errorf("failed to decode action response: %w", err)
	}

	if !actionResp.Success {
		return &actionResp, fmt.Errorf("action %s failed: %s", name, actionResp.Error)
	}

	return &actionResp, nil
}

// UploadFile calls POST /action/file-upload with multipart form data.
func (c *Client) UploadFile(ctx context.Context, localPath, remotePath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add the destination path field
	if err := writer.WriteField("dst", remotePath); err != nil {
		return fmt.Errorf("failed to write dst field: %w", err)
	}

	// Add the file
	part, err := writer.CreateFormFile("file", filepath.Base(localPath))
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("failed to copy file data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/action/file-upload", &buf)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call /action/file-upload: %w", err)
	}
	defer resp.Body.Close()

	var actionResp ActionResponse
	if err := json.NewDecoder(resp.Body).Decode(&actionResp); err != nil {
		return fmt.Errorf("failed to decode upload response: %w", err)
	}

	if !actionResp.Success {
		return fmt.Errorf("file-upload failed: %s", actionResp.Error)
	}

	return nil
}

// DownloadFile calls POST /action/file-download and saves the response body to a local file.
func (c *Client) DownloadFile(ctx context.Context, remotePath, localPath string) error {
	reqBody := ActionRequest{
		Action: "file-download",
		Params: map[string]any{
			"src": remotePath,
			"dst": remotePath, // server-side temp destination
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal download request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/action/file-download", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call /action/file-download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("file-download failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	// Create parent directories if needed
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	outFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		return fmt.Errorf("failed to save downloaded file: %w", err)
	}

	return nil
}

// Screenshot calls POST /action/screenshot and saves the PNG to the given path.
func (c *Client) Screenshot(ctx context.Context, outputPath string) error {
	reqBody := ActionRequest{
		Action: "screenshot",
		Params: map[string]any{
			"output": outputPath,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal screenshot request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/action/screenshot", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create screenshot request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call /action/screenshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("screenshot failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	// Create parent directories if needed
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		return fmt.Errorf("failed to save screenshot: %w", err)
	}

	return nil
}

// SendQMP sends a raw QMP command and returns the response.
func (c *Client) SendQMP(ctx context.Context, command string, args map[string]any) ([]byte, error) {
	reqBody := map[string]any{
		"command": command,
	}
	if args != nil {
		reqBody["arguments"] = args
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal QMP request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/qmp", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create QMP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call /qmp: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read QMP response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("QMP command failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
