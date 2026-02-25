// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package remote

import (
	"context"
	"errors"
	"io"
)

type Remote interface {
	// Shell starts a shell session using stdio as input.
	// Returns ErrFailedToConnect on failure to connect.
	Shell(ctx context.Context, out io.Writer, errOut io.Writer) error

	// RunScript runs the script on the remote server.
	// Returns ErrFailedToConnect on failure to connect.
	RunScript(ctx context.Context, script io.Reader, out io.Writer, errOut io.Writer) error
}

var ErrFailedToConnect = errors.New("failed to establish shell connection with node")
