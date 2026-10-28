// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const nftablesStderr = "internal:0:0-0: Error: Could not process rule: " +
	"No such file or directory\n\n" +
	"Error: netavark: nftables error: \"nft\" did not return successfully while applying ruleset\n"

type stderrEnvironment struct {
	stderr string
	err    error
}

func (environment *stderrEnvironment) Run(
	_ context.Context, _ io.Reader, _, stderr io.Writer, _ ...string,
) error {
	_, _ = io.WriteString(stderr, environment.stderr)

	return environment.err
}

func TestPodmanCommandFailureReportsRuntimeErrorLine(t *testing.T) {
	t.Parallel()
	// Given
	exitErr := errors.New("exit status 126")
	runtime := podmanRuntime{
		environment: &stderrEnvironment{stderr: nftablesStderr, err: exitErr},
		binary:      podmanBinary,
	}
	// When
	err := runtime.run(t.Context(), nil, nil, "run")
	// Then
	require.ErrorIs(t, err, exitErr)
	require.Equal(t, "exit status 126: Error: netavark: nftables error: "+
		"\"nft\" did not return successfully while applying ruleset", err.Error())
	require.Contains(t, CommandStderr(err), "Could not process rule")
	require.Contains(t, CommandStderr(fmt.Errorf("start: %w", err)), "Could not process rule")
	require.Empty(t, CommandStderr(exitErr))
}

func TestDockerCommandFailureReportsDaemonErrorLine(t *testing.T) {
	t.Parallel()
	// Given
	runtime := podmanRuntime{
		environment: &stderrEnvironment{
			stderr: "docker: Error response from daemon: Bind for 127.0.0.1:8563 failed: " +
				"port is already allocated\n\nRun 'docker run --help' for more information\n",
			err: errors.New("exit status 125"),
		},
		binary: "docker",
	}
	// When
	err := runtime.run(t.Context(), nil, nil, "run")
	// Then
	require.Equal(t, "exit status 125: docker: Error response from daemon: "+
		"Bind for 127.0.0.1:8563 failed: port is already allocated", err.Error())
}

func TestPodmanCommandFailureStillStreamsStderr(t *testing.T) {
	t.Parallel()
	// Given
	var streamed bytes.Buffer
	runtime := podmanRuntime{
		environment: &stderrEnvironment{stderr: nftablesStderr, err: errors.New("exit status 126")},
		binary:      podmanBinary,
	}
	// When
	_, err := runtime.output(t.Context(), nil, &streamed, "network", "create")
	// Then
	require.Error(t, err)
	require.Equal(t, nftablesStderr, streamed.String())
}

func TestPodmanCommandFailureWithoutStderrKeepsOriginalError(t *testing.T) {
	t.Parallel()
	// Given
	exitErr := errors.New("exit status 125")
	runtime := podmanRuntime{
		environment: &stderrEnvironment{stderr: "\n", err: exitErr},
		binary:      podmanBinary,
	}
	// When
	err := runtime.run(t.Context(), nil, nil, "run")
	// Then
	require.Same(t, exitErr, err)
}

func TestPodmanSucceededTreatsExitOneAsNegativeAnswer(t *testing.T) {
	t.Parallel()
	if goruntime.GOOS == "windows" {
		t.Skip("uses a POSIX shell to produce a real exit status")
	}
	// Given
	exitOne := exec.CommandContext(t.Context(), "sh", "-c", "exit 1").Run()
	runtime := podmanRuntime{
		environment: &stderrEnvironment{stderr: "Error: no such network\n", err: exitOne},
		binary:      podmanBinary,
	}
	// When
	succeeded, err := runtime.succeeded(t.Context(), nil, nil, "network", "exists", "x")
	// Then
	require.NoError(t, err)
	require.False(t, succeeded)
}

func TestStderrTailKeepsMostRecentOutput(t *testing.T) {
	t.Parallel()
	// Given
	tail := &stderrTail{}
	// When
	_, _ = tail.Write([]byte(strings.Repeat("x", stderrTailLimit)))
	_, _ = tail.Write([]byte("Error: last"))
	// Then
	require.Len(t, tail.data, stderrTailLimit)
	require.True(t, strings.HasSuffix(string(tail.data), "Error: last"))
}
