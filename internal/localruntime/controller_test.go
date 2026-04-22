// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"bufio"
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestControllerEnsure_CreatesDeploymentScopedControlDir(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())
	controller := runtime.Controller()

	// When
	err := controller.Ensure()

	// Then
	if err != nil {
		t.Fatalf("expected ensure to succeed, got %v", err)
	}

	info, statErr := os.Stat(controller.Paths().HostDir)
	if statErr != nil {
		t.Fatalf("expected control dir to exist, got %v", statErr)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", controller.Paths().HostDir)
	}
}

func TestControllerSharedDir_DescribesMountedControlBridge(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())
	controller := runtime.Controller()

	// When
	sharedDir := controller.SharedDir()

	// Then
	if sharedDir.Tag != controlShareTag {
		t.Fatalf("expected control share tag %q, got %q", controlShareTag, sharedDir.Tag)
	}
	if sharedDir.Source != controller.Paths().HostDir {
		t.Fatalf("expected control share source %q, got %q", controller.Paths().HostDir, sharedDir.Source)
	}
	if sharedDir.Destination != controller.Paths().GuestDir {
		t.Fatalf("expected guest control dir %q, got %q", controller.Paths().GuestDir, sharedDir.Destination)
	}
}

func TestControllerRequestGracefulStop_FallsBackToStopMarker(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())
	controller := runtime.Controller()

	// When
	err := controller.RequestGracefulStop(context.Background())

	// Then
	if err != nil {
		t.Fatalf("expected graceful stop request to succeed, got %v", err)
	}

	data, readErr := os.ReadFile(controller.Paths().HostStopRequestPath)
	if readErr != nil {
		t.Fatalf("expected stop marker to be written, got %v", readErr)
	}
	if strings.TrimSpace(string(data)) == "" {
		t.Fatal("expected stop marker to contain a timestamp")
	}
}

func TestControllerSendCommand_UsesUnixControlSocket(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())
	controller := runtime.Controller()
	serverDone := make(chan error, 1)
	controller.dial = func(context.Context, string, string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()

			command, err := bufio.NewReader(server).ReadString('\n')
			if err != nil {
				serverDone <- err
				return
			}
			if strings.TrimSpace(command) != "PING" {
				serverDone <- errUnexpectedCommand(command)
				return
			}
			_, err = server.Write([]byte("OK pong\n"))
			serverDone <- err
		}()

		return client, nil
	}

	// When
	response, err := controller.SendCommand(context.Background(), "PING")

	// Then
	if err != nil {
		t.Fatalf("expected send command to succeed, got %v", err)
	}
	if response != "OK pong" {
		t.Fatalf("expected trimmed socket response %q, got %q", "OK pong", response)
	}
	if serverErr := <-serverDone; serverErr != nil {
		t.Fatalf("expected socket server to succeed, got %v", serverErr)
	}
}

func TestControllerWaitForRuntimeState_ReturnsWhenStateAppears(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())
	controller := runtime.Controller()
	if err := controller.Ensure(); err != nil {
		t.Fatalf("expected ensure to succeed, got %v", err)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = os.WriteFile(
			controller.Paths().HostRuntimeStatePath,
			[]byte("sql_port=8563\nui_port=8443\njupyter_enabled=0\njupyter_port=8888\nvoila_port=8866\n"),
			0o600,
		)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// When
	state, err := controller.WaitForRuntimeState(ctx)

	// Then
	if err != nil {
		t.Fatalf("expected runtime state wait to succeed, got %v", err)
	}
	if state.SQLPort != 8563 || state.UIPort != 8443 {
		t.Fatalf("expected sql/ui ports to be parsed, got %#v", state)
	}
}

func TestControllerReadRuntimeState_ParsesRuntimeMetadata(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())
	controller := runtime.Controller()
	if err := controller.Ensure(); err != nil {
		t.Fatalf("expected ensure to succeed, got %v", err)
	}
	runtimeState := strings.Join([]string{
		"sql_port=8563",
		"ui_port=8443",
		"stack_enabled=marimo",
		"stack_port=marimo,marimo,2718,2718",
		"jupyter_enabled=1",
		"jupyter_port=8888",
		"voila_port=8866",
		"",
	}, "\n")
	if err := os.WriteFile(controller.Paths().HostRuntimeStatePath, []byte(runtimeState), 0o600); err != nil {
		t.Fatalf("expected runtime state fixture to be written, got %v", err)
	}

	// When
	state, err := controller.ReadRuntimeState()

	// Then
	if err != nil {
		t.Fatalf("expected runtime state read to succeed, got %v", err)
	}
	if state.SQLPort != 8563 {
		t.Fatalf("expected sql port 8563, got %d", state.SQLPort)
	}
	if state.UIPort != 8443 {
		t.Fatalf("expected ui port 8443, got %d", state.UIPort)
	}
	if !state.JupyterEnabled {
		t.Fatal("expected jupyter to be enabled")
	}
	if len(state.EnabledStacks) != 1 || state.EnabledStacks[0] != "marimo" {
		t.Fatalf("expected marimo to be enabled, got %#v", state.EnabledStacks)
	}
	if len(state.StackPorts) != 1 {
		t.Fatalf("expected one stack port entry, got %#v", state.StackPorts)
	}
	if state.StackPorts[0].HostPort != 2718 || state.StackPorts[0].GuestPort != 2718 {
		t.Fatalf("expected stack ports to round-trip, got %#v", state.StackPorts[0])
	}
}

func errUnexpectedCommand(command string) error {
	return &unexpectedCommandError{command: strings.TrimSpace(command)}
}

type unexpectedCommandError struct {
	command string
}

func (e *unexpectedCommandError) Error() string {
	return "unexpected command: " + e.command
}
