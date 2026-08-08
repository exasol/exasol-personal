// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestTaskUnmarshalYAML_AcceptsExactlyOneTaskType(t *testing.T) {
	t.Parallel()

	var remoteExecTask Task
	require.NoError(t, yaml.Unmarshal([]byte(`
remoteExec:
  description: run something
  filename: script.sh
`), &remoteExecTask))
	require.NotNil(t, remoteExecTask.RemoteExec)
	require.Nil(t, remoteExecTask.LocalCommand)

	var localCommandTask Task
	require.NoError(t, yaml.Unmarshal([]byte(`
localCommand:
  description: run something locally
  command: ["echo", "hi"]
`), &localCommandTask))
	require.NotNil(t, localCommandTask.LocalCommand)
	require.Nil(t, localCommandTask.RemoteExec)
}

func TestTaskUnmarshalYAML_RejectsNoTaskType(t *testing.T) {
	t.Parallel()

	var task Task

	err := yaml.Unmarshal([]byte(`description: nothing set`), &task)

	require.ErrorIs(t, err, ErrNoTaskTypeSet)
}

func TestTaskUnmarshalYAML_RejectsMultipleTaskTypes(t *testing.T) {
	t.Parallel()

	var task Task

	err := yaml.Unmarshal([]byte(`
remoteExec:
  description: run something
  filename: script.sh
localCommand:
  description: run something locally
  command: ["echo", "hi"]
`), &task)

	require.ErrorIs(t, err, ErrMultipleTaskTypesSet)
}
