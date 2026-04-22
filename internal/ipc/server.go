// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"

	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jc-lab/test-foundry/internal/logging"

	"github.com/jc-lab/test-foundry/internal/action"
)

// Server is the IPC HTTP server that runs alongside a QEMU instance.
type Server struct {
	listener   net.Listener
	httpServer *http.Server
	registry   *action.Registry
	actx       *action.ActionContext
	addr       string // 바인딩된 주소 (예: "127.0.0.1:18230")
}

// StartServer creates and starts the IPC HTTP server.
func StartServer(ctx context.Context, registry *action.Registry, actx *action.ActionContext) (*Server, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to bind IPC server: %w", err)
	}

	s := &Server{
		listener: listener,
		registry: registry,
		actx:     actx,
		addr:     listener.Addr().String(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/action/", s.handleAction)
	mux.HandleFunc("/qmp", s.handleQMP)

	s.httpServer = &http.Server{
		Handler: mux,
	}

	go func() {
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			logging.Error("IPC server error", "error", err)
		}
	}()

	logging.Info("IPC server listening", "addr", s.addr)
	return s, nil
}

// Addr returns the HTTP server address.
func (s *Server) Addr() string {
	return s.addr
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// handleStatus handles GET /status requests.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := StatusResponse{
		VMName:    s.actx.Machine.Config.MachineName,
		Status:    "running",
		SSHPort:   s.actx.Machine.Config.SSHHostPort,
		VNCPort:   5900 + s.actx.Machine.Config.VNCDisplay,
		QMPSocket: s.actx.Machine.Config.QMPSocketPath,
		TPM:       s.actx.Machine.Config.TPMEnabled,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleAction handles POST /action/{action} requests.
func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	actionName := strings.TrimPrefix(r.URL.Path, "/action/")
	if actionName == "" {
		http.Error(w, "action name is required", http.StatusBadRequest)
		return
	}

	act, err := s.registry.Get(actionName)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Parse request body
	var req ActionRequest
	if r.Header.Get("Content-Type") == "application/json" || (r.ContentLength > 0 && !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/")) {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ActionResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to decode request: %v", err),
			})
			return
		}
	}

	// For file-upload: handle multipart form
	if actionName == "file-upload" && strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(256 << 20); err != nil { // 256MB max
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ActionResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to parse multipart form: %v", err),
			})
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ActionResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to get uploaded file: %v", err),
			})
			return
		}
		defer file.Close()

		dst := r.FormValue("dst")
		if dst == "" {
			dst = header.Filename
		}

		// Save uploaded file to a temporary location, then pass to action
		tmpData, err := io.ReadAll(file)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ActionResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to read uploaded file: %v", err),
			})
			return
		}

		// Write temporary file to workdir
		tmpPath := fmt.Sprintf("%s/.upload-%s", s.actx.WorkDir, header.Filename)
		if err := os.WriteFile(tmpPath, tmpData, 0644); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ActionResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to save temp file: %v", err),
			})
			return
		}

		req.Params = map[string]any{
			"src": tmpPath,
			"dst": dst,
		}
	}

	if req.Params == nil {
		req.Params = make(map[string]any)
	}

	// Set up context with timeout
	ctx := r.Context()
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.Timeout)*time.Second)
		defer cancel()
	}

	// Execute the action
	execErr := act.Execute(ctx, s.actx, req.Params)

	// For screenshot: return PNG binary
	if actionName == "screenshot" && execErr == nil {
		output, _ := req.Params["output"].(string)
		if output != "" {
			w.Header().Set("Content-Type", "image/png")
			http.ServeFile(w, r, output)
			return
		}
	}

	// For file-download: return file as binary response
	if actionName == "file-download" && execErr == nil {
		dst, _ := req.Params["dst"].(string)
		if dst != "" {
			w.Header().Set("Content-Type", "application/octet-stream")
			http.ServeFile(w, r, dst)
			return
		}
	}

	// Standard JSON response
	resp := ActionResponse{
		Success: execErr == nil,
	}
	if execErr != nil {
		resp.Error = execErr.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	if !resp.Success {
		w.WriteHeader(http.StatusInternalServerError)
	}
	json.NewEncoder(w).Encode(resp)
}

// handleQMP handles POST /qmp requests for raw QMP commands.
func (s *Server) handleQMP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Command   string                 `json:"command"`
		Arguments map[string]interface{} `json:"arguments,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to decode request: %v", err)})
		return
	}

	if req.Command == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "command is required"})
		return
	}

	resp, err := s.actx.Machine.Execute(r.Context(), req.Command, req.Arguments)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
