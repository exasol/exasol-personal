// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	podmanTestCommand   = "podman"
	windowsTestDataPath = `C:\deployments\personal\local\data\exa`
)

type recordedCommand struct {
	Name string
	Args []string
}

type fakeExitError int

func (err fakeExitError) Error() string { return fmt.Sprintf("exit %d", err) }
func (err fakeExitError) ExitCode() int { return int(err) }

type fakeCommandRunner struct {
	Commands        []recordedCommand
	Outputs         map[string][]byte
	OutputSequences map[string][][]byte
	OutputCalls     map[string]int
	OutputError     map[string]error
	RunError        map[string]error
	Missing         map[string]bool
	LastStdin       io.Reader
}

func (runner *fakeCommandRunner) Run(
	_ context.Context,
	stdin io.Reader,
	_, _ io.Writer,
	name string,
	args ...string,
) error {
	runner.LastStdin = stdin
	runner.Commands = append(runner.Commands, recordedCommand{Name: name, Args: args})

	return runner.RunError[commandKey(name, args...)]
}

func TestMacPrerequisitesRequirePinnedVersionAndSchemas(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		version   string
		config    int
		hook      int
		state     int
		wantError bool
	}{
		{name: "matching", version: "2.0.0", config: 1, hook: 1, state: 1},
		{name: "wrong version", version: "2.0.1", config: 1, hook: 1, state: 1, wantError: true},
		{
			name: "wrong config schema", version: "2.0.0",
			config: 2, hook: 1, state: 1, wantError: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runner := &fakeCommandRunner{
				Outputs:     map[string][]byte{},
				OutputError: map[string]error{},
				RunError:    map[string]error{},
				Missing:     map[string]bool{},
			}
			adapter := NewMacVMAdapter(t.TempDir(), "/test/local-vm", runner)
			adapter.ExpectedVersion = "2.0.0"
			versionJSON := `{"version":%q,"configSchemaVersion":%d,` +
				`"hookAPIVersion":%d,"stateSchemaVersion":%d}`
			runner.Outputs[commandKey(
				"/test/local-vm", "version", "--json",
			)] = []byte(fmt.Sprintf(
				versionJSON,
				test.version,
				test.config,
				test.hook,
				test.state,
			))

			err := adapter.Prerequisites(context.Background(), PrerequisiteOptions{})
			if (err != nil) != test.wantError {
				t.Fatalf("unexpected prerequisite result: %v", err)
			}
		})
	}
}

func (runner *fakeCommandRunner) Output(
	_ context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	runner.Commands = append(runner.Commands, recordedCommand{Name: name, Args: args})
	key := commandKey(name, args...)
	if sequence := runner.OutputSequences[key]; len(sequence) != 0 {
		index := runner.OutputCalls[key]
		if index >= len(sequence) {
			index = len(sequence) - 1
		}
		if runner.OutputCalls == nil {
			runner.OutputCalls = map[string]int{}
		}
		runner.OutputCalls[key]++

		return sequence[index], runner.OutputError[key]
	}

	return runner.Outputs[key], runner.OutputError[key]
}

func (runner *fakeCommandRunner) LookPath(name string) (string, error) {
	if runner.Missing[name] {
		return "", errors.New("not found")
	}

	return name, nil
}

func commandKey(name string, args ...string) string {
	return name + "\x00" + strings.Join(args, "\x00")
}

func TestWindowsPathToWSLConvertsCanonicalDrivePath(t *testing.T) {
	t.Parallel()

	// Given / When
	converted, err := WindowsPathToWSL(`D:\deployments\personal\local\data\exa`)
	// Then
	if err != nil {
		t.Fatalf("expected drive path conversion to succeed, got %v", err)
	}
	if converted != "/mnt/d/deployments/personal/local/data/exa" {
		t.Fatalf("unexpected converted path %q", converted)
	}
}

func TestWindowsPathToWSLRejectsUNCRelativeAndTraversal(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		`\\server\share\exa`,
		`local\data\exa`,
		`C:\deployments\..\target`,
		`C:\deployments\.\target`,
		`C:\deployments\\target`,
		`C:\deployments\target\`,
	} {
		// When
		_, err := WindowsPathToWSL(path)

		// Then
		if err == nil {
			t.Fatalf("expected path %q to be rejected", path)
		}
	}
}

func TestWindowsPrerequisitesRejectsWSL1(t *testing.T) {
	t.Parallel()

	runner := &fakeCommandRunner{
		Outputs: map[string][]byte{
			commandKey("wsl.exe", "--status"): []byte("Default Version: 1"),
		},
		OutputError: map[string]error{},
		RunError:    map[string]error{},
		Missing:     map[string]bool{},
	}
	adapter := NewWindowsPodmanAdapter(t.TempDir(), runner)

	err := adapter.Prerequisites(context.Background(), PrerequisiteOptions{})

	if err == nil || !strings.Contains(err.Error(), "WSL2 is required") {
		t.Fatalf("expected WSL1 to be rejected, got %v", err)
	}
	if len(runner.Commands) != 1 {
		t.Fatalf("Podman must not be inspected for a WSL1 environment: %#v", runner.Commands)
	}
}

func TestReportsWSL2AcceptsLocalizedUTF16StyleStatus(t *testing.T) {
	t.Parallel()

	status := make([]byte, 0, len("Standardversion: 2\r\n")*2)
	for _, value := range []byte("Standardversion: 2\r\n") {
		status = append(status, value, 0)
	}
	if !reportsWSL2(status) {
		t.Fatal("expected localized UTF-16-style WSL2 status to be recognized")
	}
}

func TestWindowsPrerequisitesReusesDefaultWSLMachineWithoutMutatingIt(t *testing.T) {
	t.Parallel()

	// Given
	runner := &fakeCommandRunner{
		Outputs: map[string][]byte{
			commandKey("wsl.exe", "--status"): []byte(
				"Default Version: 2",
			),
			commandKey("podman", "version", "--format", "{{.Client.Version}}"): []byte("5.8.2"),
			commandKey("podman", "version", "--format", "{{.Server.Version}}"): []byte("5.8.2"),
			commandKey("podman", "machine", "list", "--format", "json"): []byte(`[
				{"Name":"other","Default":false,"Running":false,"VMType":"wsl"},
				{"Name":"current","Default":true,"Running":true,"VMType":"wsl"}
			]`),
		},
		OutputError: map[string]error{},
		RunError:    map[string]error{},
		Missing:     map[string]bool{},
	}
	adapter := NewWindowsPodmanAdapter(t.TempDir(), runner)

	// When
	err := adapter.Prerequisites(context.Background(), PrerequisiteOptions{})
	// Then
	if err != nil {
		t.Fatalf("expected current WSL machine to be reused, got %v", err)
	}
	for _, command := range runner.Commands {
		joined := strings.Join(command.Args, " ")
		for _, forbidden := range []string{"machine rm", "machine stop", "machine set"} {
			if strings.Contains(joined, forbidden) {
				t.Fatalf("unexpected machine mutation: %#v", command)
			}
		}
	}
	if len(runner.Commands) != 4 {
		t.Fatalf("expected detection-only commands, got %#v", runner.Commands)
	}
}

func TestWindowsPrerequisitesOffersUserScopedInstallAndUpgrade(t *testing.T) {
	t.Parallel()

	// Given
	runner := &fakeCommandRunner{
		Outputs: map[string][]byte{
			commandKey("wsl.exe", "--status"): []byte(
				"Default Version: 2",
			),
			commandKey("podman", "machine", "list", "--format", "json"): []byte(`[]`),
		},
		OutputSequences: map[string][][]byte{
			commandKey("podman", "version", "--format", "{{.Client.Version}}"): {
				[]byte("5.7.1"), []byte("5.8.2"),
			},
			commandKey("podman", "version", "--format", "{{.Server.Version}}"): {
				[]byte("5.7.1"), []byte("5.8.2"),
			},
		},
		OutputCalls: map[string]int{},
		OutputError: map[string]error{},
		RunError:    map[string]error{},
		Missing:     map[string]bool{"podman": true},
	}
	adapter := NewWindowsPodmanAdapter(t.TempDir(), runner)
	var prompts []string

	// When
	err := adapter.Prerequisites(context.Background(), PrerequisiteOptions{
		Interactive: true,
		Confirm: func(prompt string) (bool, error) {
			prompts = append(prompts, prompt)
			return true, nil
		},
	})
	// Then
	if err != nil {
		t.Fatalf("expected approved prerequisite setup to succeed, got %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("expected install and upgrade prompts, got %#v", prompts)
	}
	assertRecordedCommand(t, runner.Commands, "winget", "install", "--id", "RedHat.Podman")
	assertRecordedCommand(t, runner.Commands, "winget", "upgrade", "--id", "RedHat.Podman")
	assertRecordedCommand(t, runner.Commands, "podman", "machine", "init")
	assertRecordedCommand(t, runner.Commands, "podman", "machine", "start")
}

func TestWindowsPrerequisitesNeverInstallsWithoutApproval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		interactive bool
		confirm     func(string) (bool, error)
	}{
		{name: "non-interactive"},
		{
			name:        "declined",
			interactive: true,
			confirm: func(string) (bool, error) {
				return false, nil
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runner := &fakeCommandRunner{
				Outputs: map[string][]byte{
					commandKey("wsl.exe", "--status"): []byte("Default Version: 2"),
				},
				OutputError: map[string]error{},
				RunError:    map[string]error{},
				Missing:     map[string]bool{"podman": true},
			}
			adapter := NewWindowsPodmanAdapter(t.TempDir(), runner)

			err := adapter.Prerequisites(context.Background(), PrerequisiteOptions{
				Interactive: test.interactive,
				Confirm:     test.confirm,
			})
			if err == nil {
				t.Fatal("expected missing Podman to require explicit approval")
			}
			for _, command := range runner.Commands {
				if command.Name == "winget" {
					t.Fatalf("Podman was installed without approval: %#v", runner.Commands)
				}
			}
		})
	}
}

func TestMacStageGeneratesDisposableProviderAndWorkloadArtifacts(t *testing.T) {
	t.Parallel()

	// Given
	deployment := t.TempDir()
	dataPath := filepath.Join(deployment, "local", "data", "exa")
	adapter := NewMacVMAdapter(deployment, "/test/local-vm", &fakeCommandRunner{})
	spec := testWorkloadSpec()
	spec.DataPath = dataPath
	spec.CPUs = 4
	spec.MemoryMiB = 12288

	// When
	err := adapter.stage(spec, adapter.paths(), true)
	// Then
	if err != nil {
		t.Fatalf("expected macOS staging to succeed, got %v", err)
	}
	configData, err := os.ReadFile(adapter.paths().Config)
	if err != nil {
		t.Fatal(err)
	}
	var config localVMConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		t.Fatal(err)
	}
	if config.Resources.CPUs != 4 || config.Resources.MemoryMiB != 12288 {
		t.Fatalf("unexpected VM resources: %#v", config.Resources)
	}
	if config.BootHook.Share != "control" || config.BootHook.Path != "hooks/start" {
		t.Fatalf("unexpected boot hook: %#v", config.BootHook)
	}
	if len(config.Shares) != 2 ||
		config.Shares[1].HostPath != filepath.Join(deployment, "local", "data") ||
		config.Shares[1].GuestPath != "/mnt/data" {
		t.Fatalf("expected Personal-owned data share, got %#v", config.Shares)
	}
	if bytes.Contains(configData, []byte("runtimeDisk")) {
		t.Fatalf("provider scratch disk must not be caller configuration:\n%s", configData)
	}
	manifest, err := os.ReadFile(adapter.paths().Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(manifest, []byte("path: /mnt/data/exa")) {
		t.Fatalf("expected guest data path in manifest:\n%s", manifest)
	}
	if bytes.Contains(configData, []byte(spec.ImageReference)) {
		t.Fatalf("local-vm config must not contain Nano concerns:\n%s", configData)
	}
}

func TestMacArtifactsForTwoDeploymentsRemainIsolated(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	firstRoot := filepath.Join(parent, "first")
	secondRoot := filepath.Join(parent, "second")
	first := NewMacVMAdapter(firstRoot, "/test/local-vm", &fakeCommandRunner{})
	second := NewMacVMAdapter(secondRoot, "/test/local-vm", &fakeCommandRunner{})
	firstSpec := testWorkloadSpec()
	firstSpec.DeploymentID = "11111111-1111-1111-1111-111111111111"
	firstSpec.DataPath = filepath.Join(firstRoot, "local", "data", "exa")
	firstSpec.DBHostPort = 28563
	firstSpec.LoadImageArchive = func() ([]byte, error) { return []byte("first image"), nil }
	secondSpec := testWorkloadSpec()
	secondSpec.DeploymentID = "22222222-2222-2222-2222-222222222222"
	secondSpec.DataPath = filepath.Join(secondRoot, "local", "data", "exa")
	secondSpec.DBHostPort = 28564
	secondSpec.LoadImageArchive = func() ([]byte, error) { return []byte("second image"), nil }

	if err := first.stage(firstSpec, first.paths(), true); err != nil {
		t.Fatal(err)
	}
	if err := second.stage(secondSpec, second.paths(), true); err != nil {
		t.Fatal(err)
	}

	firstManifest, err := os.ReadFile(first.paths().Manifest)
	if err != nil {
		t.Fatal(err)
	}
	secondManifest, err := os.ReadFile(second.paths().Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(firstManifest, secondManifest) {
		t.Fatal("deployment-scoped manifests unexpectedly match")
	}
	if !bytes.Contains(firstManifest, []byte(WorkloadName(firstSpec.DeploymentID))) ||
		bytes.Contains(firstManifest, []byte(WorkloadName(secondSpec.DeploymentID))) {
		t.Fatalf("first manifest contains another deployment's identity:\n%s", firstManifest)
	}
	if !bytes.Contains(secondManifest, []byte(WorkloadName(secondSpec.DeploymentID))) ||
		bytes.Contains(secondManifest, []byte(WorkloadName(firstSpec.DeploymentID))) {
		t.Fatalf("second manifest contains another deployment's identity:\n%s", secondManifest)
	}
	for path, expected := range map[string]string{
		first.paths().Image:  "first image",
		second.paths().Image: "second image",
	} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != expected {
			t.Fatalf("unexpected staged image at %s: data=%q err=%v", path, data, err)
		}
	}
}

func TestMacInteractiveShellForwardsStdinAndRequestsTTY(t *testing.T) {
	t.Parallel()

	providerState := []byte(`{
		"schemaVersion":1,
		"phase":"running",
		"ssh":{"address":"127.0.0.1","port":20022},
		"privateKeyPath":"/test/key",
		"hook":{"phase":"succeeded"}
	}`)
	runner := &fakeCommandRunner{
		Outputs:     map[string][]byte{},
		OutputError: map[string]error{},
		RunError:    map[string]error{},
		Missing:     map[string]bool{},
	}
	adapter := NewMacVMAdapter(t.TempDir(), "/test/local-vm", runner)
	runner.Outputs[commandKey(
		"/test/local-vm", "status", "--state-dir", adapter.paths().State, "--json",
	)] = providerState
	stdin := strings.NewReader("exit\n")

	err := adapter.Shell(
		context.Background(),
		testWorkloadSpec(),
		ShellVM,
		stdin,
		io.Discard,
		io.Discard,
	)
	if err != nil {
		t.Fatal(err)
	}
	if runner.LastStdin != stdin {
		t.Fatal("interactive shell did not forward stdin")
	}
	if len(runner.Commands) != 2 || !contains(runner.Commands[1].Args, "-tt") {
		t.Fatalf("interactive shell did not request a TTY: %#v", runner.Commands)
	}
}

func TestMacStartInitializesProviderBeforeStartingWorkload(t *testing.T) {
	t.Parallel()

	deployment := t.TempDir()
	runner := &fakeCommandRunner{
		Outputs:     map[string][]byte{},
		OutputError: map[string]error{},
		RunError:    map[string]error{},
		Missing:     map[string]bool{},
	}
	adapter := NewMacVMAdapter(deployment, "/test/local-vm", runner)
	paths := adapter.paths()
	providerState := []byte(`{
		"schemaVersion":1,
		"phase":"running",
		"ssh":{"address":"127.0.0.1","port":20022},
		"privateKeyPath":"/test/key",
		"forwards":[
			{"name":"database","hostAddress":"127.0.0.1","hostPort":8563}
		],
		"hook":{"phase":"succeeded"}
	}`)
	runner.Outputs[commandKey(
		"/test/local-vm", "status", "--state-dir", paths.State, "--json",
	)] = providerState
	runner.Outputs[commandKey(
		"ssh",
		"-i", "/test/key",
		"-p", "20022",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "BatchMode=yes",
		"root@127.0.0.1",
		"/mnt/control/workload-helper",
		"status",
	)] = []byte("Running\n")
	spec := testWorkloadSpec()
	spec.DataPath = filepath.Join(deployment, "local", "data", "exa")

	status, err := adapter.Start(context.Background(), spec, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if status.Phase != RuntimePhaseRunning {
		t.Fatalf("unexpected runtime status: %#v", status)
	}
	if len(runner.Commands) != 4 {
		t.Fatalf("unexpected start command sequence: %#v", runner.Commands)
	}
	for index, command := range []struct {
		action string
		args   []string
	}{
		{
			action: "init",
			args:   []string{"init", "--state-dir", paths.State, "--config", paths.Config},
		},
		{
			action: "start",
			args:   []string{"start", "--state-dir", paths.State, "--config", paths.Config},
		},
	} {
		got := runner.Commands[index]
		if got.Name != "/test/local-vm" || !containsSequence(got.Args, command.args) {
			t.Fatalf("%s command = %#v", command.action, got)
		}
	}
}

func TestMacStopTakesWorkloadDownBeforeVM(t *testing.T) {
	t.Parallel()

	// Given
	providerState := []byte(`{
		"schemaVersion":1,
		"phase":"running",
		"ssh":{"address":"127.0.0.1","port":20022},
		"privateKeyPath":"/test/key",
		"hook":{"phase":"succeeded"}
	}`)
	runner := &fakeCommandRunner{
		Outputs:     map[string][]byte{},
		OutputError: map[string]error{},
		RunError:    map[string]error{},
		Missing:     map[string]bool{},
	}
	deployment := t.TempDir()
	adapter := NewMacVMAdapter(deployment, "/test/local-vm", runner)
	runner.Outputs[commandKey(
		"/test/local-vm",
		"status",
		"--state-dir",
		adapter.paths().State,
		"--json",
	)] = providerState

	// When
	spec := testWorkloadSpec()
	spec.DataPath = filepath.Join(deployment, "local", "data", "exa")
	err := adapter.Stop(context.Background(), spec, io.Discard, io.Discard)
	// Then
	if err != nil {
		t.Fatalf("expected stop to succeed, got %v", err)
	}
	if len(runner.Commands) != 3 {
		t.Fatalf("unexpected stop calls: %#v", runner.Commands)
	}
	if runner.Commands[1].Name != "ssh" ||
		!contains(runner.Commands[1].Args, "/mnt/control/workload-helper") ||
		!contains(runner.Commands[1].Args, "down") {
		t.Fatalf("expected helper down before VM stop, got %#v", runner.Commands)
	}
	if runner.Commands[2].Name != "/test/local-vm" ||
		len(runner.Commands[2].Args) == 0 ||
		runner.Commands[2].Args[0] != "stop" {
		t.Fatalf("expected local-vm stop last, got %#v", runner.Commands)
	}
}

func TestMacStatusDiscoversLivePodmanWorkloadState(t *testing.T) {
	t.Parallel()

	providerState := []byte(`{
		"schemaVersion":1,
		"phase":"running",
		"ssh":{"address":"127.0.0.1","port":20022},
		"privateKeyPath":"/test/key",
		"forwards":[
			{"name":"database","hostAddress":"127.0.0.1","hostPort":8563}
		],
		"hook":{"phase":"succeeded"}
	}`)
	runner := &fakeCommandRunner{
		Outputs:     map[string][]byte{},
		OutputError: map[string]error{},
		RunError:    map[string]error{},
		Missing:     map[string]bool{},
	}
	adapter := NewMacVMAdapter(t.TempDir(), "/test/local-vm", runner)
	runner.Outputs[commandKey(
		"/test/local-vm",
		"status",
		"--state-dir",
		adapter.paths().State,
		"--json",
	)] = providerState
	runner.Outputs[commandKey(
		"ssh",
		"-i", "/test/key",
		"-p", "20022",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "BatchMode=yes",
		"root@127.0.0.1",
		"/mnt/control/workload-helper",
		"status",
	)] = []byte("Running\n")

	status, err := adapter.Status(context.Background(), testWorkloadSpec())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != RuntimePhaseRunning {
		t.Fatalf("expected live workload status, got %#v", status)
	}
	if len(runner.Commands) != 2 || runner.Commands[1].Name != "ssh" {
		t.Fatalf("expected provider and remote workload discovery, got %#v", runner.Commands)
	}
}

func TestMacStatusPreservesProviderDegradedState(t *testing.T) {
	t.Parallel()

	providerState := []byte(`{
		"schemaVersion":1,
		"phase":"degraded",
		"message":"SSH readiness failed",
		"hook":{"phase":"pending"}
	}`)
	runner := &fakeCommandRunner{
		Outputs:     map[string][]byte{},
		OutputError: map[string]error{},
		RunError:    map[string]error{},
		Missing:     map[string]bool{},
	}
	adapter := NewMacVMAdapter(t.TempDir(), "/test/local-vm", runner)
	runner.Outputs[commandKey(
		"/test/local-vm",
		"status",
		"--state-dir",
		adapter.paths().State,
		"--json",
	)] = providerState

	status, err := adapter.Status(context.Background(), testWorkloadSpec())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != RuntimePhaseDegraded ||
		!strings.Contains(status.Message, "SSH readiness failed") {
		t.Fatalf("expected provider degradation to remain visible, got %#v", status)
	}
	if len(runner.Commands) != 1 {
		t.Fatalf("degraded provider must not be queried over SSH: %#v", runner.Commands)
	}
}

func TestWindowsStopIsIdempotentWhenWorkloadIsAbsent(t *testing.T) {
	t.Parallel()

	runner := &fakeCommandRunner{
		Outputs:     map[string][]byte{},
		OutputError: map[string]error{},
		RunError:    map[string]error{},
		Missing:     map[string]bool{},
	}
	adapter := NewWindowsPodmanAdapter(t.TempDir(), runner)
	spec := testWorkloadSpec()
	spec.DataPath = windowsTestDataPath
	name := WorkloadName(spec.DeploymentID)
	runner.RunError[commandKey(
		"podman",
		"pod",
		"exists",
		name,
	)] = fakeExitError(1)

	err := adapter.Stop(context.Background(), spec, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("expected an absent workload to be an idempotent stop, got %v", err)
	}
	if len(runner.Commands) != 1 {
		t.Fatalf("expected status-only stop, got %#v", runner.Commands)
	}
}

func TestWindowsStopRegeneratesManifestForRunningWorkload(t *testing.T) {
	t.Parallel()

	deployment := t.TempDir()
	runner := &fakeCommandRunner{
		Outputs:     map[string][]byte{},
		OutputError: map[string]error{},
		RunError:    map[string]error{},
		Missing:     map[string]bool{},
	}
	adapter := NewWindowsPodmanAdapter(deployment, runner)
	spec := testWorkloadSpec()
	spec.DataPath = windowsTestDataPath
	spec.SLCMounts = nil
	name := WorkloadName(spec.DeploymentID)
	runner.Outputs[commandKey(
		"podman",
		"pod",
		"inspect",
		"--format",
		"{{.State}}",
		name,
	)] = []byte("Running\n")

	err := adapter.Stop(context.Background(), spec, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("expected running workload stop to succeed, got %v", err)
	}
	manifestPath := filepath.Join(deployment, "local", "control", "workload.yaml")
	assertRecordedCommand(t, runner.Commands, "podman", "kube", "down", manifestPath)
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected disposable manifest to be regenerated: %v", err)
	}
}

//nolint:paralleltest // The test changes the process working directory.
func TestWindowsStartLoadsImageBeforeApplyingWorkload(t *testing.T) {
	t.Chdir(t.TempDir())

	deployment := t.TempDir()
	runner := &fakeCommandRunner{
		Outputs:     map[string][]byte{},
		OutputError: map[string]error{},
		RunError:    map[string]error{},
		Missing:     map[string]bool{},
	}
	adapter := NewWindowsPodmanAdapter(deployment, runner)
	spec := testWorkloadSpec()
	spec.DataPath = windowsTestDataPath
	spec.SLCMounts = nil
	name := WorkloadName(spec.DeploymentID)
	runner.Outputs[commandKey(
		podmanTestCommand, "pod", "inspect", "--format", "{{.State}}", name,
	)] = []byte("Running\n")
	runner.Outputs[commandKey(
		podmanTestCommand, "container", "inspect", "--format", "{{.Id}}", name+"-nano",
	)] = []byte("container-id\n")
	runner.Outputs[commandKey(
		podmanTestCommand, "port", name+"-nano", "8563/tcp",
	)] = []byte("127.0.0.1:28563\n")

	status, err := adapter.Start(context.Background(), spec, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if status.Phase != RuntimePhaseRunning ||
		status.Database.Port != 28563 ||
		status.ContainerID != "container-id" {
		t.Fatalf("unexpected runtime status: %#v", status)
	}
	manifestPath := filepath.Join(deployment, "local", "control", "workload.yaml")
	imagePath := filepath.Join(deployment, "local", "control", "nano-image.tar.gz")
	if len(runner.Commands) < 2 {
		t.Fatalf("missing workload commands: %#v", runner.Commands)
	}
	if got := runner.Commands[0]; got.Name != podmanTestCommand ||
		!containsSequence(got.Args, []string{"load", "--input", imagePath}) {
		t.Fatalf("first command did not load the image: %#v", got)
	}
	if got := runner.Commands[1]; got.Name != podmanTestCommand ||
		!containsSequence(got.Args, []string{"kube", "play", "--replace", manifestPath}) {
		t.Fatalf("second command did not apply the workload: %#v", got)
	}
}

//nolint:paralleltest // The test changes the process working directory.
func TestWindowsStartPullsMissingSLCBeforeApplyingWorkload(t *testing.T) {
	t.Chdir(t.TempDir())

	// Given
	deployment := t.TempDir()
	runner := &fakeCommandRunner{
		Outputs:     map[string][]byte{},
		OutputError: map[string]error{},
		RunError:    map[string]error{},
		Missing:     map[string]bool{},
	}
	adapter := NewWindowsPodmanAdapter(deployment, runner)
	spec := testWorkloadSpec()
	spec.DataPath = windowsTestDataPath
	slcImage := spec.SLCMounts[0].Image
	runner.RunError[commandKey(
		podmanTestCommand, "image", "exists", slcImage,
	)] = fakeExitError(1)
	name := WorkloadName(spec.DeploymentID)
	runner.Outputs[commandKey(
		podmanTestCommand, "pod", "inspect", "--format", "{{.State}}", name,
	)] = []byte("Running\n")

	// When
	if _, err := adapter.Start(context.Background(), spec, io.Discard, io.Discard); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Then
	assertRecordedCommand(t, runner.Commands, podmanTestCommand, "pull", slcImage)
	var pullIndex, playIndex = -1, -1
	for index, command := range runner.Commands {
		if command.Name != podmanTestCommand {
			continue
		}
		if containsSequence(command.Args, []string{"pull", slcImage}) {
			pullIndex = index
		}
		if containsSequence(command.Args, []string{"kube", "play", "--replace"}) {
			playIndex = index
		}
	}
	if pullIndex < 0 || playIndex < 0 || pullIndex >= playIndex {
		t.Fatalf("SLC pull must precede kube play: %#v", runner.Commands)
	}
	if !adapter.Capabilities().SLC {
		t.Fatal("Windows adapter did not advertise SLC support")
	}
}

//nolint:paralleltest // The test changes the process working directory.
func TestWindowsStartDoesNotApplyWorkloadAfterImageLoadFailure(t *testing.T) {
	t.Chdir(t.TempDir())

	deployment := t.TempDir()
	runner := &fakeCommandRunner{
		Outputs:     map[string][]byte{},
		OutputError: map[string]error{},
		RunError:    map[string]error{},
		Missing:     map[string]bool{},
	}
	adapter := NewWindowsPodmanAdapter(deployment, runner)
	spec := testWorkloadSpec()
	spec.DataPath = windowsTestDataPath
	spec.SLCMounts = nil
	imagePath := filepath.Join(deployment, "local", "control", "nano-image.tar.gz")
	runner.RunError[commandKey(
		podmanTestCommand, "load", "--input", imagePath,
	)] = errors.New("image load failed")

	_, err := adapter.Start(context.Background(), spec, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "load embedded Nano image") {
		t.Fatalf("expected image load failure, got %v", err)
	}
	for _, command := range runner.Commands {
		if command.Name == podmanTestCommand &&
			containsSequence(command.Args, []string{"kube", "play"}) {
			t.Fatalf("workload was applied after image load failed: %#v", runner.Commands)
		}
	}
}

func TestRuntimeDestroyPreservesPersonalOwnedData(t *testing.T) {
	t.Parallel()

	deployment := t.TempDir()
	dataPath := filepath.Join(deployment, "local", "data", "exa")
	if err := os.MkdirAll(dataPath, privateDirMode); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dataPath, "preserve")
	if err := os.WriteFile(marker, []byte("data"), privateFileMode); err != nil {
		t.Fatal(err)
	}
	runner := &fakeCommandRunner{
		Outputs:     map[string][]byte{},
		OutputError: map[string]error{},
		RunError:    map[string]error{},
		Missing:     map[string]bool{},
	}
	adapter := NewMacVMAdapter(deployment, "/test/local-vm", runner)
	spec := testWorkloadSpec()
	spec.DataPath = dataPath

	if err := adapter.Destroy(context.Background(), spec, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "data" {
		t.Fatalf("runtime destroy modified Personal-owned data: data=%q err=%v", data, err)
	}
}

func containsSequence(values, expected []string) bool {
	if len(expected) > len(values) {
		return false
	}
	for offset := 0; offset <= len(values)-len(expected); offset++ {
		matches := true
		for index := range expected {
			if values[offset+index] != expected[index] {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}

	return false
}

func assertRecordedCommand(
	t *testing.T,
	commands []recordedCommand,
	name string,
	argumentPrefix ...string,
) {
	t.Helper()
	for _, command := range commands {
		if command.Name != name || len(command.Args) < len(argumentPrefix) {
			continue
		}
		matches := true
		for index, argument := range argumentPrefix {
			if command.Args[index] != argument {
				matches = false
				break
			}
		}
		if matches {
			return
		}
	}
	t.Fatalf("expected command %s %#v in %#v", name, argumentPrefix, commands)
}
