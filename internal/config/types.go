// Copyright 2026 JC-Lab
// SPDX-License-Identifier: GPL-2.0-only

package config

import "time"

// Step represents a single step in a setup or test sequence.
type Step struct {
	Action  string         `yaml:"action"`  // Action 이름 (예: "wait-boot", "file-upload", "exec")
	Timeout Duration       `yaml:"timeout"` // Step 실행 제한 시간
	Params  map[string]any `yaml:"params"`  // Action별 파라미터 (action마다 다른 구조)
}

// Duration is a wrapper around time.Duration that supports YAML unmarshalling
// from human-readable strings like "120s", "5m", "1h30m".
type Duration struct {
	time.Duration
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Duration.
// TODO: 구현 필요 사항:
//   - "120s", "5m", "10m30s" 등 time.ParseDuration 형식 지원
//   - 파싱 실패 시 에러 반환
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = dur
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface for Duration.
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.Duration.String(), nil
}
