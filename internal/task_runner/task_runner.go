// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package task_runner

import (
	"context"
	"io"
	"log/slog"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/remote"
	"github.com/exasol/exasol-personal/internal/util"
)

const CommandOutputBufferSize = 128 * 1024

type TaskRunner interface {
	RunTasks(
		ctx context.Context,
		tasks []config.Task,
		deploymentDir string,
		out, outErr io.Writer,
	) error
}

type NodeLookup interface {
	Find(
		nodeNameGlob string,
	) ([]RunScriptNode, error)
}

type RunScriptNode struct {
	Name              string
	ConnectionOptions *remote.SSHConnectionOptions
}

func NewTaskRunner(
	localCommandRunner LocalCommandRunner,
	remoteScriptRunner RemoteScriptRunner,
	nodeLookup NodeLookup,
) TaskRunner {
	return &TaskRunnerImpl{
		localCommandRunner: localCommandRunner,
		remoteScriptRunner: remoteScriptRunner,
		nodeLookup:         nodeLookup,
	}
}

type TaskRunnerImpl struct {
	localCommandRunner LocalCommandRunner
	remoteScriptRunner RemoteScriptRunner
	nodeLookup         NodeLookup
}

func (s *TaskRunnerImpl) RunTasks(
	ctx context.Context,
	tasks []config.Task,
	deploymentDir string,
	out, outErr io.Writer,
) error {
	slog.Debug("running post-deploy scripts", "scriptCount", len(tasks))

	logBuffer := NewLogBuffer()

	taskOut := util.CombineWriters(logBuffer, out)
	taskOutErr := util.CombineWriters(logBuffer, outErr)

	var err error
	for _, script := range tasks {
		var taskDesc string

		logBuffer.Clear()

		if script.LocalCommand != nil {
			err = s.runLocalCommand(ctx, script.LocalCommand, deploymentDir, taskOut, taskOutErr)
			taskDesc = script.LocalCommand.Description
		} else if script.RemoteExec != nil {
			err = s.runTaskRemoteExec(ctx, script.RemoteExec, deploymentDir, taskOut, taskOutErr)
			taskDesc = script.RemoteExec.Description
		}

		if err != nil {
			logBuffer.ReplayLogMessages(ctx)

			slog.Error("task failed", "task", taskDesc)
			slog.Error("see above output for failure details")

			return err
		}
	}

	return nil
}
