// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"reflect"

	"github.com/exasol/exasol-personal/internal/presets"
	"gopkg.in/yaml.v3"
)

// Task model used by installation manifest steps and the task runner.

type Task struct {
	RemoteExec   *presets.RemoteExecTask   `yaml:"remoteExec"`
	LocalCommand *presets.LocalCommandTask `yaml:"localCommand"`
}

var (
	ErrNoTaskTypeSet        = errors.New("no task types are set")
	ErrMultipleTaskTypesSet = errors.New("multiple task types are set on a single task")
)

func (s *Task) UnmarshalYAML(value *yaml.Node) error {
	// This type alias does not have the UnmarshalYAML method.
	// If we do not do this, the `value.Decode(...)` call below would cause an infinite loop.
	type alias Task
	result := (*alias)(s)

	if err := value.Decode(&result); err != nil {
		return err
	}

	// Ensure that only one field is set
	totalSetTasks := 0

	val := reflect.ValueOf(s).Elem()
	for i := range val.NumField() {
		if !val.Field(i).IsNil() {
			totalSetTasks++
		}
	}

	if totalSetTasks > 1 {
		return ErrMultipleTaskTypesSet
	}

	if totalSetTasks == 0 {
		return ErrNoTaskTypeSet
	}

	return nil
}
