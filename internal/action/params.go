// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package action

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// DecodeParams converts a map[string]any (from YAML step.params) into a typed struct.
// The target must be a pointer to a struct with `yaml` tags.
// This works by marshalling the map back to YAML and then unmarshalling into the struct.
func DecodeParams(raw map[string]any, target any) error {
	if raw == nil {
		return nil
	}

	data, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("failed to encode params: %w", err)
	}

	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to decode params: %w", err)
	}

	return nil
}

// EncodeParams converts a typed param struct into a map[string]any for IPC transmission.
// The source must be a struct with `yaml` tags.
func EncodeParams(source any) (map[string]any, error) {
	if source == nil {
		return nil, nil
	}

	data, err := yaml.Marshal(source)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}

	var result map[string]any
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal params to map: %w", err)
	}

	return result, nil
}

// --- Typed parameter structs for each action ---

// WaitBootParams holds parameters for the wait-boot action.
type WaitBootParams struct {
	RetryInterval string `yaml:"retry_interval"` // SSH 재시도 간격 (예: "5s"), default "5s"
}

// WaitOOBEParams holds parameters for the wait-oobe action.
// Currently empty but defined for future extensibility.
type WaitOOBEParams struct{}

// FileUploadParams holds parameters for the file-upload action.
type FileUploadParams struct {
	Src string `yaml:"src"` // 로컬 파일 경로
	Dst string `yaml:"dst"` // Guest 내 대상 경로
}

// FileDownloadParams holds parameters for the file-download action.
type FileDownloadParams struct {
	Src string `yaml:"src"` // Guest 내 파일 경로
	Dst string `yaml:"dst"` // 로컬 저장 경로
}

// ExecParams holds parameters for the exec action.
type ExecParams struct {
	Cmd            string   `yaml:"cmd"`              // 실행할 명령어
	Args           []string `yaml:"args"`             // 명령 인자
	ExpectExitCode *int     `yaml:"expect_exit_code"` // 기대 종료 코드 (nil이면 검사 안함)
}

// ScreenshotParams holds parameters for the screenshot action.
type ScreenshotParams struct {
	Output string `yaml:"output"` // 스크린샷 PNG 저장 경로
}

// ShutdownParams holds parameters for the shutdown action.
// Currently empty but defined for future extensibility.
type ShutdownParams struct{}

// RebootParams holds parameters for the reboot action.
// Currently empty but defined for future extensibility.
type RebootParams struct{}

// DumpParams holds parameters for the dump action.
type DumpParams struct {
	// Format
	// See https://qemu-project.gitlab.io/qemu/interop/qemu-qmp-ref.html#enum-QMP-dump.DumpGuestMemoryFormat
	// available: elf/kdump-zlib/kdump-lzo/kdump-snappy/kdump-raw-zlib/kdump-raw-lzo/kdump-raw-snappy/win-dmp
	Format string `yaml:"format"`
	Output string `yaml:"output"` // 메모리 덤프 저장 경로
}

// SleepParams holds parameters for the sleep action.
type SleepParams struct {
	Duration string `yaml:"duration"` // 대기 시간 (예: "10s", "1m30s")
}

// WaitPanicParams holds parameters for the wait-panic action.
// Currently empty but defined for future extensibility.
type WaitPanicParams struct{}
