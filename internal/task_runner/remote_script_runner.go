// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package task_runner

import (
	"context"
	"io"
	"time"

	"github.com/exasol/exasol-personal/internal/remote"
)

type RemoteScriptRunner interface {
	RunScript(
		ctx context.Context,
		connectionOptions *remote.SSHConnectionOptions,
		script io.Reader,
		out io.Writer,
		errOut io.Writer,
	) error
}

const (
	scriptRetryDelay            = time.Millisecond * 500
	scriptRetryDelayScaleFactor = 2.0
	scriptRetryDelayMaximum     = time.Second * 10
)

type RemoteScriptRunnerImpl struct{}

func (*RemoteScriptRunnerImpl) RunScript(
	ctx context.Context,
	connectionOptions *remote.SSHConnectionOptions,
	script io.Reader,
	out io.Writer,
	outErr io.Writer,
) error {
	return remote.NewSshRemote(connectionOptions).RunScript(ctx, script, out, outErr)
}

func nextRetryDelay(current time.Duration) time.Duration {
	return min(scriptRetryDelayMaximum, time.Duration(float64(current)*scriptRetryDelayScaleFactor))
}
