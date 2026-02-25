// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package task_runner_mocks

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/exasol/exasol-personal/internal/remote"
	"github.com/exasol/exasol-personal/internal/task_runner"
	"github.com/gobwas/glob"
	"github.com/google/uuid"
)

type LocalCommandRunnerMock struct{}

// Run implements task_runner.LocalCommandRunner.
func (*LocalCommandRunnerMock) Run(
	_ context.Context,
	_ []string,
	_ string,
	_ io.Writer,
	_ io.Writer,
) error {
	panic("unimplemented")
}

func NewLocalCommandRunnerMock() task_runner.LocalCommandRunner {
	return &LocalCommandRunnerMock{}
}

type RemoteScriptRunnerMock struct {
	mutex                 sync.Mutex
	RunScriptCalls        []RemoteScriptRunnerMockRunScriptCall
	RunScriptCallDuration time.Duration
}

const remoteScriptExecDuration = time.Millisecond + 150

func NewRemoteScriptRunnerMock() *RemoteScriptRunnerMock {
	return &RemoteScriptRunnerMock{
		RunScriptCallDuration: remoteScriptExecDuration,
	}
}

var _ task_runner.RemoteScriptRunner = &RemoteScriptRunnerMock{}

type RemoteScriptRunnerMockRunScriptCall struct {
	ConnectionOptions remote.SSHConnectionOptions
	Script            string
	Out               io.Writer
	OutErr            io.Writer
	Start             time.Time
	Stop              time.Time
}

// RunScript implements task_runner.RemoteScriptRunner.
func (r *RemoteScriptRunnerMock) RunScript(
	_ context.Context,
	connectionOptions *remote.SSHConnectionOptions,
	script io.Reader,
	out io.Writer,
	errOut io.Writer,
) error {
	slog.Debug("running mock RunScript")

	scriptData, err := io.ReadAll(script)
	if err != nil {
		panic("error reading script")
	}

	startTime := time.Now()
	time.Sleep(r.RunScriptCallDuration)
	stopTime := time.Now()

	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.RunScriptCalls = append(r.RunScriptCalls,
		RemoteScriptRunnerMockRunScriptCall{
			ConnectionOptions: *connectionOptions,
			Script:            string(scriptData),
			Out:               out,
			OutErr:            errOut,
			Start:             startTime,
			Stop:              stopTime,
		},
	)

	return nil
}

type NodeLookupMock struct {
	Directory []task_runner.RunScriptNode
}

var _ task_runner.NodeLookup = &NodeLookupMock{}

func NewNodeLookupMock(directory []task_runner.RunScriptNode) *NodeLookupMock {
	return &NodeLookupMock{
		Directory: directory,
	}
}

func NewNodeLookupDirectory(n int) []task_runner.RunScriptNode {
	directory := make([]task_runner.RunScriptNode, n)

	const baseNodeNumber = 11

	for i := range n {
		directory[i] = task_runner.RunScriptNode{
			Name:              fmt.Sprintf("n%d", baseNodeNumber+i),
			ConnectionOptions: NewUniqueSSHConnectionOptions(),
		}
	}

	return directory
}

// Find implements task_runner.NodeLookup.
func (n *NodeLookupMock) Find(nodeNameGlob string) ([]task_runner.RunScriptNode, error) {
	slog.Debug("Looking up node glob", "glob", nodeNameGlob, "directorySize", len(n.Directory))
	results := []task_runner.RunScriptNode{}

	pattern := glob.MustCompile(nodeNameGlob)
	for _, node := range n.Directory {
		slog.Debug("checking node against glob", "glob", nodeNameGlob, "node", node.Name)

		if pattern.Match(node.Name) {
			slog.Debug("node matches against glob", "glob", nodeNameGlob, "node", node.Name)
			results = append(results, node)
		}
	}

	return results, nil
}

func NewUniqueSSHConnectionOptions() *remote.SSHConnectionOptions {
	return &remote.SSHConnectionOptions{
		Host: uuid.New().String(),
		Port: uuid.New().String(),
		User: uuid.New().String(),
		Key:  []byte(uuid.New().String()),
	}
}

func NewUniqueFile(path string) {
	//nolint:mnd // ignore magic number 0600
	if os.WriteFile(path, []byte(uuid.New().String()), 0o600) != nil {
		panic("failed to write unique file")
	}
}
