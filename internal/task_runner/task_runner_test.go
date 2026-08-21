// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package task_runner_test

import (
	"bytes"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/task_runner"
	mocks "github.com/exasol/exasol-personal/internal/task_runner/task_runner_mocks"
	"github.com/stretchr/testify/require"
)

func TestRunLocalCommand(t *testing.T) {
	t.Parallel()
	slog.SetLogLoggerLevel(slog.LevelDebug)

	fakeDeploymentDir := t.TempDir()
	deployment := config.NewDeploymentDir(fakeDeploymentDir)

	nodeLookupMock := mocks.NewNodeLookupMock(mocks.NewNodeLookupDirectory(5))

	type testArgs struct {
		tasks          []config.Task
		expectedStdOut string
		expectedStdErr string
	}

	testCases := []testArgs{
		{
			tasks: []config.Task{
				{
					LocalCommand: &presets.LocalCommandTask{
						Description: "test task echo",
						Command:     []string{"echo", "{{ .Node }}"},
						Node:        "*",
					},
				},
			},
			expectedStdOut: func() string { // output contains all node names
				nodeNames := ""
				var nodeNamesSb49 strings.Builder
				for _, node := range nodeLookupMock.Directory {
					_, _ = nodeNamesSb49.WriteString(node.Name + "\n")
				}
				nodeNames += nodeNamesSb49.String()

				return nodeNames
			}(),
			expectedStdErr: "",
		},
		{
			tasks: []config.Task{
				{
					LocalCommand: &presets.LocalCommandTask{
						Description: "test task pwd",
						Command:     []string{"pwd"},
					},
				},
			},
			expectedStdOut: fakeDeploymentDir + "\n",
			expectedStdErr: "",
		},
	}

	for _, testCase := range testCases {
		localCommandRunner := &task_runner.LocalCommandRunnerImpl{}
		remoteScriptRunner := mocks.NewRemoteScriptRunnerMock()

		taskRunner := task_runner.NewTaskRunner(
			localCommandRunner,
			remoteScriptRunner,
			nodeLookupMock,
		)

		slog.Debug("pre availableNodes", "nodes", nodeLookupMock.Directory)

		var stdOutBuff bytes.Buffer
		var stdErrBuff bytes.Buffer

		err := taskRunner.RunTasks(
			t.Context(),
			testCase.tasks,
			deployment,
			&stdOutBuff,
			&stdErrBuff,
		)

		require.NoError(t, err, "RunTasks should succeed")

		require.Equal(t, testCase.expectedStdOut, stdOutBuff.String())
		require.Equal(t, testCase.expectedStdErr, stdErrBuff.String())
	}
}

func TestRunRemoteScript(t *testing.T) {
	t.Parallel()
	slog.SetLogLoggerLevel(slog.LevelInfo)

	testCases := []struct {
		name     string
		parallel bool
	}{
		{name: "parallel", parallel: true},
		{name: "sequential", parallel: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runRemoteScriptAndVerifyOrdering(t, testCase.parallel)
		})
	}
}

//nolint:revive // parallel selects which table-driven test case runs, not internal control coupling.
func runRemoteScriptAndVerifyOrdering(t *testing.T, parallel bool) {
	t.Helper()

	localCommandRunner := mocks.NewLocalCommandRunnerMock()
	remoteScriptRunner := mocks.NewRemoteScriptRunnerMock()

	nodeLookupMock := mocks.NewNodeLookupMock(mocks.NewNodeLookupDirectory(5))

	taskRunner := task_runner.NewTaskRunner(
		localCommandRunner,
		remoteScriptRunner,
		nodeLookupMock,
	)

	fakeDeploymentDir := t.TempDir()
	scriptFilePath := filepath.Join(fakeDeploymentDir, "script.sh")
	mocks.NewUniqueFile(scriptFilePath)
	deployment := config.NewDeploymentDir(fakeDeploymentDir)

	tasks := []config.Task{
		{
			RemoteExec: &presets.RemoteExecTask{
				Description:       "test task",
				Filename:          scriptFilePath,
				ExecuteInParallel: parallel,
				Node:              "*",
			},
		},
	}

	slog.Debug("pre availableNodes", "nodes", nodeLookupMock.Directory)
	err := taskRunner.RunTasks(t.Context(), tasks, deployment, nil, nil)

	require.NoError(t, err, "RunTasks should succeed")

	require.Len(
		t,
		remoteScriptRunner.RunScriptCalls, len(nodeLookupMock.Directory),
		"expected script to run on all nodes",
	)

	startStopMap := matchCallsToNodes(
		t, nodeLookupMock.Directory, remoteScriptRunner.RunScriptCalls,
	)

	err = verifyStartStopOrdering(startStopMap)
	if parallel {
		require.ErrorIs(t, err, errInterleavedTasks)
	} else {
		require.NoError(t, err)
	}
}

// matchCallsToNodes pairs each recorded RunScript call with the node it targeted
// (by connection host) and returns a map of each call's start/stop timestamp to
// whether it marks a "start" or "stop". It fails the test if any node was not
// matched to exactly one call.
func matchCallsToNodes(
	t *testing.T,
	nodes []task_runner.RunScriptNode,
	calls []mocks.RemoteScriptRunnerMockRunScriptCall,
) map[int64]string {
	t.Helper()

	availableNodes := make([]task_runner.RunScriptNode, len(nodes))

	for idx, node := range nodes {
		connectionOptions := *node.ConnectionOptions
		availableNodes[idx] = task_runner.RunScriptNode{
			Name:              node.Name,
			ConnectionOptions: &connectionOptions,
		}
	}

	startStopMap := map[int64]string{}

	for _, call := range calls {
		for idx, node := range availableNodes {
			if node.ConnectionOptions.Host != call.ConnectionOptions.Host {
				continue
			}

			availableNodes = append(availableNodes[0:idx], availableNodes[idx+1:]...)
			startStopMap[call.Start.UnixMicro()] = "start"
			startStopMap[call.Stop.UnixMicro()] = "stop"
		}
	}

	require.Empty(t, availableNodes, "found unmatched nodes")

	return startStopMap
}

var errInterleavedTasks = errors.New("tasks were interleaved")

// verifyStartStopOrdering checks that sorted timestamps alternate strictly
// between "start" and "stop", which only holds when tasks ran sequentially.
func verifyStartStopOrdering(startStopMap map[int64]string) error {
	startStopMapKeys := make([]int64, 0, len(startStopMap))
	for key := range startStopMap {
		startStopMapKeys = append(startStopMapKeys, key)
	}

	slices.Sort(startStopMapKeys)

	for idx, key := range startStopMapKeys {
		want := "start"
		if idx%2 != 0 {
			want = "stop"
		}

		if startStopMap[key] != want {
			return errInterleavedTasks
		}
	}

	return nil
}
