// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package task_runner

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/tofu"
	"github.com/exasol/exasol-personal/internal/util"
)

func (*TaskRunnerImpl) runLocalCommandForNode(
	ctx context.Context,
	templatedCommand []string,
	nodeName string,
	deploymentDir string,
	out, outErr io.Writer,
) error {
	command := make([]string, len(templatedCommand))

	tofuConfig := tofu.NewTofuConfigFromDeployment(deploymentDir, presets.InfrastructureTofu{})

	for idx, part := range templatedCommand {
		slog.Debug("parsing command part", "part", part)

		newPart, err := commandSubstitutions(part, map[string]string{
			"Node": nodeName,
			"Tofu": tofuConfig.TofuBinaryPath(),
		})
		if err != nil {
			return err
		}

		slog.Debug("final command part", "part", newPart)

		command[idx] = newPart
	}

	slog.Debug("running command", "command", command)

	return (&LocalCommandRunnerImpl{}).Run(ctx, command, deploymentDir, out, outErr)
}

var ErrNoMatchingNodesFound = errors.New("no matching nodes found")

func (s *TaskRunnerImpl) runLocalCommand(
	ctx context.Context,
	localCommand *presets.LocalCommandTask,
	deploymentDir string,
	out, outErr io.Writer,
) error {
	if localCommand.Description != "" {
		slog.Info(localCommand.Description)
	}

	slog.Debug("running local command", "command", localCommand, "nodeGlob", localCommand.Node)

	regexLogger := NewRegexLogger(localCommand.RegexLog)
	taskOut := util.CombineWriters(regexLogger, out)
	taskOutErr := util.CombineWriters(regexLogger, outErr)

	if localCommand.Node == "" {
		// If no node glob is specified, run the command once with .Node unset
		return s.runLocalCommandForNode(
			ctx,
			localCommand.Command,
			"",
			deploymentDir,
			taskOut,
			taskOutErr,
		)
	}

	// If a node glob is provided, run the command for every matching node, which .Node set
	// the node's name.
	nodes, err := s.nodeLookup.Find(localCommand.Node)
	if err != nil {
		return util.LoggedError(err, "failed to lookup nodes for local command",
			"nodeGlob", localCommand.Node)
	}

	if len(nodes) == 0 {
		return util.LoggedError(ErrNoMatchingNodesFound, "no matching nodes for local command",
			"nodeGlob", localCommand.Node,
		)
	}

	for _, node := range nodes {
		err = s.runLocalCommandForNode(
			ctx,
			localCommand.Command,
			node.Name,
			deploymentDir,
			taskOut,
			taskOutErr,
		)
		if err != nil {
			return util.LoggedError(err, "failed to execute local command",
				"node", node.Name,
				"command", localCommand.Command,
			)
		}
	}

	return nil
}
