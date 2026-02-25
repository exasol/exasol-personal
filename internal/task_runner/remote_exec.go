// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package task_runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/remote"
	"github.com/exasol/exasol-personal/internal/util"
)

func (s *TaskRunnerImpl) runTaskRemoteExec(
	ctx context.Context,
	remoteExec *presets.RemoteExecTask,
	deploymentDir string,
	out, outErr io.Writer,
) error {
	if remoteExec.Description != "" {
		slog.Info(remoteExec.Description)
	}

	slog.Debug(
		"running script on remote",
		"script",
		remoteExec.Filename,
		"nodeGlob",
		remoteExec.Node,
	)

	nodes, err := s.nodeLookup.Find(remoteExec.Node)
	if err != nil {
		return util.LoggedError(err, "failed to lookup nodes", "nodeGlob", remoteExec.Node)
	}

	slog.Debug("running script against remote nodes", "nodes", nodes, "script", remoteExec.Filename)

	scriptFilePath := filepath.Join(deploymentDir, remoteExec.Filename)

	scriptFile, err := os.ReadFile(scriptFilePath)
	if err != nil {
		slog.Debug("failed to read script file", "file", scriptFilePath)
		return fmt.Errorf("%w: could not read script file %s", err, scriptFilePath)
	}

	var scriptResults map[string]error

	if remoteExec.ExecuteInParallel {
		slog.Debug("running script in parallel")
		scriptResults = s.RunScriptParallel(ctx, nodes, scriptFile)
	} else {
		slog.Debug("running script in serial")

		scriptResults = make(map[string]error)
		for _, node := range nodes {
			regexLogger := NewRegexLoggerWithNode(remoteExec.RegexLog, node.Name)
			taskOut := util.CombineWriters(regexLogger, out)
			taskOutErr := util.CombineWriters(regexLogger, outErr)

			scriptResults[node.Name] = s.RunScript(
				ctx,
				scriptFile,
				node,
				taskOut,
				taskOutErr,
			)
		}
	}

	scriptErrors := []error{}
	for node, result := range scriptResults {
		if result != nil {
			scriptErrors = append(scriptErrors, util.LoggedError(
				result, "script failed on node",
				"node", node))
		}
	}

	// Node: if a script fails on any node, no further scripts will be run.
	if len(scriptErrors) > 0 {
		return errors.Join(scriptErrors...)
	}

	return nil
}

func (s *TaskRunnerImpl) RunScriptParallel(
	ctx context.Context,
	nodes []RunScriptNode,
	script []byte,
	// nolint: revive
) map[string]error {
	results := make(map[string]error)

	waitGroup := sync.WaitGroup{}
	mtx := sync.Mutex{}

	for _, node := range nodes {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			err := s.RunScript(ctx, script, node, nil, nil)

			mtx.Lock()
			defer mtx.Unlock()

			results[node.Name] = err
		}()
	}

	waitGroup.Wait()

	return results
}

func (s *TaskRunnerImpl) RunScript(
	ctx context.Context,
	script []byte,
	node RunScriptNode,
	out, outErr io.Writer,
) error {
	retryDelay := scriptRetryDelay
	for {
		select {
		case <-ctx.Done():
			slog.Info("script cancelled", "node", node.Name)
			return context.Canceled
		default:
		}

		slog.Debug("attempting to connect to node", "node", node.Name)

		err := s.remoteScriptRunner.RunScript(
			ctx,
			node.ConnectionOptions,
			bytes.NewReader(script),
			out,
			outErr,
		)

		if !errors.Is(err, remote.ErrFailedToConnect) {
			return err
		}

		slog.Info("retrying connection to node after delay",
			"node", node.Name,
			"retryDelay", fmt.Sprintf("%.2f", retryDelay.Seconds()))

		time.Sleep(retryDelay)

		retryDelay = nextRetryDelay(retryDelay)
	}
}
