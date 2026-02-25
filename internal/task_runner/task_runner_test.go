// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package task_runner_test

import (
	"bytes"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
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
				for _, node := range nodeLookupMock.Directory {
					nodeNames += node.Name + "\n"
				}

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
			fakeDeploymentDir,
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

	type testArgs struct {
		parallel bool
	}

	testCases := []testArgs{
		{
			parallel: true,
		},
		{
			parallel: false,
		},
	}

	for _, testCase := range testCases {
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

		tasks := []config.Task{
			{
				RemoteExec: &presets.RemoteExecTask{
					Description:       "test task",
					Filename:          scriptFilePath,
					ExecuteInParallel: testCase.parallel,
					Node:              "*",
				},
			},
		}

		slog.Debug("pre availableNodes", "nodes", nodeLookupMock.Directory)
		err := taskRunner.RunTasks(t.Context(), tasks, "", nil, nil)

		require.NoError(t, err, "RunTasks should succeed")

		require.Len(
			t,
			remoteScriptRunner.RunScriptCalls, len(nodeLookupMock.Directory),
			"expected script to run on all nodes",
		)

		startStopMap := map[int64]string{}

		availableNodes := make([]task_runner.RunScriptNode, len(nodeLookupMock.Directory))

		for idx, node := range nodeLookupMock.Directory {
			connectionOptions := *node.ConnectionOptions
			availableNodes[idx] = task_runner.RunScriptNode{
				Name:              node.Name,
				ConnectionOptions: &connectionOptions,
			}
		}

		slog.Debug("post availableNodes", "nodes", nodeLookupMock.Directory)

		slog.Debug("availableNodes", "nodes", availableNodes)
		for _, call := range remoteScriptRunner.RunScriptCalls {
			for idx, node := range availableNodes {
				if node.ConnectionOptions.Host == call.ConnectionOptions.Host {
					availableNodes = append(availableNodes[0:idx], availableNodes[idx+1:]...)
					startStopMap[call.Start.UnixMicro()] = "start"
					startStopMap[call.Stop.UnixMicro()] = "stop"
				}
			}
		}

		// All nodes should have been matched above and filtered out.
		require.Empty(t, availableNodes, "found unmatched nodes")

		startStopMapKeys := []int64{}
		for key := range startStopMap {
			startStopMapKeys = append(startStopMapKeys, key)
		}

		slices.Sort(startStopMapKeys)

		errInterleavedTasks := errors.New("tasks were interleaved")
		tasksNotInterleaved := func() error {
			for idx, key := range startStopMapKeys {
				if idx%2 == 0 {
					if startStopMap[key] != "start" {
						return errInterleavedTasks
					}
				} else {
					if startStopMap[key] != "stop" {
						return errInterleavedTasks
					}
				}
			}

			return nil
		}

		if testCase.parallel {
			require.ErrorIs(
				t,
				tasksNotInterleaved(),
				errInterleavedTasks,
			)
		} else {
			require.NoError(
				t,
				tasksNotInterleaved(),
			)
		}
	}
}
