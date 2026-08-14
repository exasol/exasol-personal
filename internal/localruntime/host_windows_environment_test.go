// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestWindowsHostRuntimeUsesWindowsPreparationAndSharedHostPolicies(t *testing.T) {
	t.Parallel()
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		DeploymentId: "windows-runtime",
		Connection:   &config.DeploymentConnection{Host: "127.0.0.1", DBPort: 28563},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}
	localRuntime := NewHostWindowsRuntime(deployment, nil)
	if _, ok := localRuntime.preparer.(windowsHostEnvironmentPreparer); !ok {
		t.Fatalf("expected Windows environment preparation, got %T", localRuntime.preparer)
	}
	connection, peer := net.Pipe()
	t.Cleanup(func() {
		_ = connection.Close()
		_ = peer.Close()
	})
	localRuntime.dialContext = func(
		context.Context, string, string,
	) (net.Conn, error) {
		return connection, nil
	}

	health, err := localRuntime.HealthCheck(context.Background())
	if err != nil || health.Ports["db"].State != PortStateReachable {
		t.Fatalf("expected shared host health policy, health=%#v err=%v", health, err)
	}
	if err := localRuntime.OpenHostShell(
		context.Background(), nil, nil, nil,
	); !errors.Is(err, ErrHostShellUnsupported) {
		t.Fatalf("expected unsupported Windows host shell, got %v", err)
	}
	if err := localRuntime.OpenContainerShell(
		context.Background(), nil, nil, nil,
	); !errors.Is(err, ErrContainerShellUnsupported) {
		t.Fatalf("expected unsupported Windows container shell, got %v", err)
	}
}

type windowsCommandOutput struct {
	value string
	err   error
}

type fakeWindowsHostCommandRunner struct {
	outputs map[string][]windowsCommandOutput
	runErrs map[string]error
	runs    []HostCommand
	env     map[string]string
}

func (runner *fakeWindowsHostCommandRunner) Output(
	_ context.Context,
	command HostCommand,
) (string, error) {
	key := formatHostCommand(command)
	outputs := runner.outputs[key]
	if len(outputs) == 0 {
		return "", fmt.Errorf("unexpected output command %q", key)
	}
	output := outputs[0]
	runner.outputs[key] = outputs[1:]

	return output.value, output.err
}

func (runner *fakeWindowsHostCommandRunner) Run(
	_ context.Context,
	_ io.Writer,
	command HostCommand,
) error {
	runner.runs = append(runner.runs, command)

	return runner.runErrs[formatHostCommand(command)]
}

func (runner *fakeWindowsHostCommandRunner) Getenv(name string) string {
	return runner.env[name]
}

func (runner *fakeWindowsHostCommandRunner) Setenv(name, value string) error {
	runner.env[name] = value

	return nil
}

func newReadyWindowsRunner() *fakeWindowsHostCommandRunner {
	return &fakeWindowsHostCommandRunner{
		outputs: map[string][]windowsCommandOutput{
			"podman --version": {{value: "podman version 5"}},
			"podman machine list --format {{.Name}}": {{
				value: windowsDefaultMachineName,
			}},
			"podman machine inspect --format {{.Rootful}} " + windowsDefaultMachineName: {{
				value: "true",
			}},
			"podman machine inspect --format {{.State}} " + windowsDefaultMachineName: {{
				value: "Running",
			}},
		},
		runErrs: map[string]error{},
		env:     map[string]string{"PATH": `C:\Windows`},
	}
}

func TestWindowsHostEnvironmentPrepare_ReadyEnvironmentIsUnchanged(t *testing.T) {
	t.Parallel()
	runner := newReadyWindowsRunner()
	preparer := windowsHostEnvironmentPreparer{runner: runner}

	if err := preparer.EnsureReady(context.Background(), PrepareOptions{}); err != nil {
		t.Fatalf("expected ready environment, got %v", err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("expected readiness checks only, ran %#v", runner.runs)
	}
}

func TestWindowsHostEnvironmentPrepare_RefreshesPathBeforeInstalling(t *testing.T) {
	t.Parallel()
	runner := newReadyWindowsRunner()
	runner.outputs["podman --version"] = []windowsCommandOutput{
		{err: errors.New("not found")},
		{value: "podman version 5"},
	}
	runner.outputs[registeredPathCommand()] = []windowsCommandOutput{{
		value: `C:\Program Files\RedHat\Podman;C:\Windows`,
	}}
	preparer := windowsHostEnvironmentPreparer{runner: runner}

	if err := preparer.EnsureReady(context.Background(), PrepareOptions{}); err != nil {
		t.Fatalf("expected refreshed Podman discovery, got %v", err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("expected no installation command, ran %#v", runner.runs)
	}
	if runner.env["PATH"] != `C:\Program Files\RedHat\Podman;C:\Windows` {
		t.Fatalf("unexpected refreshed PATH %q", runner.env["PATH"])
	}
}

func TestWindowsHostEnvironmentPrepare_ApprovedInstallUsesExactCommand(t *testing.T) {
	t.Parallel()
	runner := newReadyWindowsRunner()
	runner.outputs["podman --version"] = []windowsCommandOutput{
		{err: errors.New("not found")},
		{err: errors.New("still not found")},
		{value: "podman version 5"},
	}
	runner.outputs[registeredPathCommand()] = []windowsCommandOutput{
		{value: `C:\Windows`},
		{value: `C:\Program Files\RedHat\Podman;C:\Windows`},
	}
	var request HostChangeRequest
	options := PrepareOptions{ApproveHostChange: func(
		_ context.Context,
		actual HostChangeRequest,
	) (bool, error) {
		request = actual

		return true, nil
	}}
	preparer := windowsHostEnvironmentPreparer{runner: runner}

	if err := preparer.EnsureReady(context.Background(), options); err != nil {
		t.Fatalf("expected approved installation, got %v", err)
	}
	if request.Kind != HostChangeInstallContainerRuntime ||
		!reflect.DeepEqual(request.Commands, []HostCommand{windowsPodmanInstallCommand}) {
		t.Fatalf("unexpected approval request %#v", request)
	}
	if !reflect.DeepEqual(runner.runs, []HostCommand{windowsPodmanInstallCommand}) {
		t.Fatalf("expected exact winget command, ran %#v", runner.runs)
	}
}

func TestWindowsHostEnvironmentPrepare_DeclinedInstallMakesNoHostChange(t *testing.T) {
	t.Parallel()
	runner := newReadyWindowsRunner()
	runner.outputs["podman --version"] = []windowsCommandOutput{
		{err: errors.New("not found")}, {err: errors.New("still not found")},
	}
	runner.outputs[registeredPathCommand()] = []windowsCommandOutput{{value: `C:\Windows`}}
	preparer := windowsHostEnvironmentPreparer{runner: runner}
	options := PrepareOptions{ApproveHostChange: func(
		context.Context, HostChangeRequest,
	) (bool, error) {
		return false, nil
	}}

	err := preparer.EnsureReady(context.Background(), options)

	if err == nil || !strings.Contains(err.Error(), "was not approved") {
		t.Fatalf("expected declined approval error, got %v", err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("expected no host change, ran %#v", runner.runs)
	}
}

func TestWindowsHostEnvironmentPrepare_InstallFailureCanBeRetried(t *testing.T) {
	t.Parallel()
	runner := newReadyWindowsRunner()
	runner.outputs["podman --version"] = []windowsCommandOutput{
		{err: errors.New("not found")},
		{err: errors.New("still not found")},
		{err: errors.New("not found on retry")},
		{err: errors.New("still not found on retry")},
		{value: "podman version 5"},
	}
	runner.outputs[registeredPathCommand()] = []windowsCommandOutput{
		{value: `C:\Windows`},
		{value: `C:\Windows`},
		{value: `C:\Program Files\RedHat\Podman;C:\Windows`},
	}
	installErr := errors.New("winget unavailable")
	runner.runErrs[formatHostCommand(windowsPodmanInstallCommand)] = installErr
	options := PrepareOptions{ApproveHostChange: func(
		context.Context, HostChangeRequest,
	) (bool, error) {
		return true, nil
	}}
	preparer := windowsHostEnvironmentPreparer{runner: runner}

	firstErr := preparer.EnsureReady(context.Background(), options)
	if !errors.Is(firstErr, installErr) {
		t.Fatalf("expected causal installation failure, got %v", firstErr)
	}
	delete(runner.runErrs, formatHostCommand(windowsPodmanInstallCommand))
	if err := preparer.EnsureReady(context.Background(), options); err != nil {
		t.Fatalf("expected preparation retry to succeed, got %v", err)
	}
	if len(runner.runs) != 2 {
		t.Fatalf("expected one install attempt per preparation call, got %#v", runner.runs)
	}
}

func TestWindowsHostEnvironmentPrepare_CreatesMissingRootfulMachine(t *testing.T) {
	t.Parallel()
	runner := newReadyWindowsRunner()
	runner.outputs["podman machine list --format {{.Name}}"] = []windowsCommandOutput{{}}
	preparer := windowsHostEnvironmentPreparer{runner: runner}

	if err := preparer.EnsureReady(context.Background(), PrepareOptions{}); err != nil {
		t.Fatalf("expected machine creation, got %v", err)
	}
	want := []HostCommand{
		hostCommand("podman", "machine", "init", "--disk-size", "40", "--rootful"),
		hostCommand("podman", "machine", "start", windowsDefaultMachineName),
	}
	if !reflect.DeepEqual(runner.runs, want) {
		t.Fatalf("unexpected machine commands %#v", runner.runs)
	}
}

func TestWindowsHostEnvironmentPrepare_StartsStoppedRootfulMachine(t *testing.T) {
	t.Parallel()
	runner := newReadyWindowsRunner()
	stateCommand := "podman machine inspect --format {{.State}} " + windowsDefaultMachineName
	runner.outputs[stateCommand] = []windowsCommandOutput{{value: "Stopped"}}
	preparer := windowsHostEnvironmentPreparer{runner: runner}

	if err := preparer.EnsureReady(context.Background(), PrepareOptions{}); err != nil {
		t.Fatalf("expected stopped machine start, got %v", err)
	}
	want := []HostCommand{
		hostCommand("podman", "machine", "start", windowsDefaultMachineName),
	}
	if !reflect.DeepEqual(runner.runs, want) {
		t.Fatalf("unexpected machine commands %#v", runner.runs)
	}
}

func TestWindowsHostEnvironmentPrepare_ApprovedRootlessConversion(t *testing.T) {
	t.Parallel()
	runner := newReadyWindowsRunner()
	rootfulCommand := "podman machine inspect --format {{.Rootful}} " +
		windowsDefaultMachineName
	runner.outputs[rootfulCommand] = []windowsCommandOutput{{value: "false"}}
	var request HostChangeRequest
	options := PrepareOptions{ApproveHostChange: func(
		_ context.Context,
		actual HostChangeRequest,
	) (bool, error) {
		request = actual

		return true, nil
	}}
	preparer := windowsHostEnvironmentPreparer{runner: runner}

	if err := preparer.EnsureReady(context.Background(), options); err != nil {
		t.Fatalf("expected rootless conversion, got %v", err)
	}
	want := []HostCommand{
		hostCommand("podman", "machine", "stop", windowsDefaultMachineName),
		hostCommand("podman", "machine", "set", "--rootful", windowsDefaultMachineName),
		hostCommand("podman", "machine", "start", windowsDefaultMachineName),
	}
	if request.Kind != HostChangeEnablePrivilegedRuntime ||
		!reflect.DeepEqual(request.Commands, want) || !reflect.DeepEqual(runner.runs, want) {
		t.Fatalf("unexpected conversion request=%#v runs=%#v", request, runner.runs)
	}
}

func TestWindowsHostEnvironmentPrepare_DeclinedRootlessConversionMakesNoChange(t *testing.T) {
	t.Parallel()
	runner := newReadyWindowsRunner()
	rootfulCommand := "podman machine inspect --format {{.Rootful}} " +
		windowsDefaultMachineName
	runner.outputs[rootfulCommand] = []windowsCommandOutput{{value: "false"}}
	options := PrepareOptions{ApproveHostChange: func(
		context.Context, HostChangeRequest,
	) (bool, error) {
		return false, nil
	}}
	preparer := windowsHostEnvironmentPreparer{runner: runner}

	err := preparer.EnsureReady(context.Background(), options)

	if err == nil || !strings.Contains(err.Error(), "was not approved") {
		t.Fatalf("expected declined conversion error, got %v", err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("expected no conversion commands, ran %#v", runner.runs)
	}
}

func TestMergeWindowsPath_PrependsUniqueRegisteredEntriesCaseInsensitively(t *testing.T) {
	t.Parallel()

	actual := mergeWindowsPath(
		`C:\Windows;C:\Tools`,
		`c:\windows; C:\Program Files\RedHat\Podman ;; C:\TOOLS`,
	)

	if actual != `C:\Program Files\RedHat\Podman;C:\Windows;C:\Tools` {
		t.Fatalf("unexpected merged PATH %q", actual)
	}
}

func registeredPathCommand() string {
	return formatHostCommand(hostCommand(
		"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass",
		"-Command", windowsRegisteredPathScript,
	))
}
