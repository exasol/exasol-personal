// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type PayloadRef struct {
	Version      string          `json:"version,omitempty"`
	Architecture string          `json:"architecture,omitempty"`
	Checksum     string          `json:"checksum,omitempty"`
	CachePath    string          `json:"cachePath,omitempty"`
	Boot         *PayloadBootRef `json:"boot,omitempty"`
}

type PayloadBootRef struct {
	KernelPath string `json:"kernelPath,omitempty"`
	InitrdPath string `json:"initrdPath,omitempty"`
}

type State struct {
	Ports   map[string]int `json:"ports,omitempty"`
	Payload *PayloadRef    `json:"payload,omitempty"`
}

type Store struct {
	path string
}

const (
	stateDirMode  = 0o700
	stateFileMode = 0o600
)

func NewStore(path string) Store {
	return Store{path: path}
}

func (s Store) Path() string {
	return s.path
}

func (s Store) Read() (*State, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{}, nil
		}

		return nil, fmt.Errorf("failed to read local runtime state: %w", err)
	}

	var result State
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode local runtime state: %w", err)
	}

	return &result, nil
}

func (s Store) Write(state *State) error {
	if state == nil {
		state = &State{}
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode local runtime state: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.path), stateDirMode); err != nil {
		return fmt.Errorf("failed to create local runtime state dir: %w", err)
	}

	if err := os.WriteFile(s.path, append(data, '\n'), stateFileMode); err != nil {
		return fmt.Errorf("failed to write local runtime state: %w", err)
	}

	return nil
}
