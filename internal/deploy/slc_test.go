// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
	"reflect"
	"runtime"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

// newDeployedLocalTestDeployment builds a local-backend deployment whose
// workflow state is past "initialized" (SLC operations require an actual
// deployment to modify), seeded with installedSLCs if any are given.
func newDeployedLocalTestDeployment(
	t *testing.T,
	installedSLCs []config.InstalledSLC,
) config.DeploymentDir {
	t.Helper()

	deployment := newLocalTestDeployment(t)
	state := &config.ExasolPersonalState{
		DeploymentVersion: "0.0.0",
		InstalledSLCs:     installedSLCs,
	}
	stopped := &config.WorkflowStateStopped{}
	if err := state.SetWorkflowStateAndWrite(stopped, deployment); err != nil {
		t.Fatalf("failed to write launcher state: %v", err)
	}

	return deployment
}

func TestInstallSLCRejectsNonLocalDeployment(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("failed to create infrastructure dir: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: tofu
`)

	_, err := InstallSLC(context.Background(), deployment, "python3", false, false, nil)

	if !errors.Is(err, ErrSLCNotSupported) {
		t.Fatalf("expected ErrSLCNotSupported for a non-local deployment, got %v", err)
	}
}

func TestInstallSLCRejectsDeploymentNotYetDeployed(t *testing.T) {
	t.Parallel()

	deployment := newLocalTestDeployment(t)
	state := &config.ExasolPersonalState{DeploymentVersion: "0.0.0"}
	initialized := &config.WorkflowStateInitialized{}
	if err := state.SetWorkflowStateAndWrite(initialized, deployment); err != nil {
		t.Fatalf("failed to write launcher state: %v", err)
	}

	_, err := InstallSLC(context.Background(), deployment, "python3", false, false, nil)

	if !errors.Is(err, ErrDeploymentNotPresent) {
		t.Fatalf("expected ErrDeploymentNotPresent, got %v", err)
	}
}

func TestUpdateSLCRejectsNonLocalDeployment(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("failed to create infrastructure dir: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: tofu
`)

	_, err := UpdateSLC(context.Background(), deployment, "python3", false, false, nil)

	if !errors.Is(err, ErrSLCNotSupported) {
		t.Fatalf("expected ErrSLCNotSupported for a non-local deployment, got %v", err)
	}
}

func TestUpdateSLCRejectsDeploymentNotYetDeployed(t *testing.T) {
	t.Parallel()

	deployment := newLocalTestDeployment(t)
	state := &config.ExasolPersonalState{DeploymentVersion: "0.0.0"}
	initialized := &config.WorkflowStateInitialized{}
	if err := state.SetWorkflowStateAndWrite(initialized, deployment); err != nil {
		t.Fatalf("failed to write launcher state: %v", err)
	}

	_, err := UpdateSLC(context.Background(), deployment, "python3", false, false, nil)

	if !errors.Is(err, ErrDeploymentNotPresent) {
		t.Fatalf("expected ErrDeploymentNotPresent, got %v", err)
	}
}

func TestRemoveSLCRejectsNonLocalDeployment(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("failed to create infrastructure dir: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: tofu
`)

	_, err := RemoveSLC(context.Background(), deployment, "python3", false, false, nil)

	if !errors.Is(err, ErrSLCNotSupported) {
		t.Fatalf("expected ErrSLCNotSupported for a non-local deployment, got %v", err)
	}
}

func TestRemoveSLCRejectsDeploymentNotYetDeployed(t *testing.T) {
	t.Parallel()

	deployment := newLocalTestDeployment(t)
	state := &config.ExasolPersonalState{DeploymentVersion: "0.0.0"}
	initialized := &config.WorkflowStateInitialized{}
	if err := state.SetWorkflowStateAndWrite(initialized, deployment); err != nil {
		t.Fatalf("failed to write launcher state: %v", err)
	}

	_, err := RemoveSLC(context.Background(), deployment, "python3", false, false, nil)

	if !errors.Is(err, ErrDeploymentNotPresent) {
		t.Fatalf("expected ErrDeploymentNotPresent, got %v", err)
	}
}

func TestRemoveSLCReturnsNotFoundWhenAliasIsNotInstalled(t *testing.T) {
	t.Parallel()

	deployment := newDeployedLocalTestDeployment(t, nil)

	result, err := RemoveSLC(context.Background(), deployment, "python3", false, false, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.Found || result.Changed {
		t.Fatalf("expected not-found no-op result, got %+v", result)
	}
}

// RemoveSLC never touches the SLC catalog (unlike Install/Update), so its full
// restart=false logic is reachable on every architecture, not just the
// catalog's arm64-only entries.
func TestRemoveSLCWithoutRestartRecordsRemovalAndDefersActivation(t *testing.T) {
	t.Parallel()

	installed := []config.InstalledSLC{
		{
			Language: "python",
			Flavor:   "python-3.12",
			Version:  "3.12",
			Image:    "docker.io/exasol/script-language-container:python-3.12",
			Target:   "/exa/slc/python-3.12",
			Aliases:  []string{"PYTHON3", "PYTHON312"},
		},
	}
	deployment := newDeployedLocalTestDeployment(t, installed)

	result, err := RemoveSLC(context.Background(), deployment, "python3", false, false, nil)
	if err != nil {
		t.Fatalf("expected removal to succeed, got %v", err)
	}

	if !result.Found || !result.Changed {
		t.Fatalf("expected a found, changed removal, got %+v", result)
	}
	if result.Outcome != SLCApplyDeferred {
		t.Fatalf("expected deferred activation without --restart, got %s", result.Outcome)
	}
	if result.Entry == nil || result.Entry.Flavor != installed[0].Flavor {
		t.Fatalf("expected removed entry to be reported, got %+v", result.Entry)
	}

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("failed to read state: %v", err)
	}
	if len(state.InstalledSLCs) != 0 {
		t.Fatalf("expected the SLC to be removed from state, got %+v", state.InstalledSLCs)
	}
}

func TestRemoveSLCWithoutRestartLeavesOtherInstalledSLCsUntouched(t *testing.T) {
	t.Parallel()

	installed := []config.InstalledSLC{
		{Language: "python", Flavor: "python-3.12", Aliases: []string{"PYTHON3"}},
		{Language: "java", Flavor: "java-17", Aliases: []string{"JAVA"}},
	}
	deployment := newDeployedLocalTestDeployment(t, installed)

	_, err := RemoveSLC(context.Background(), deployment, "python3", false, false, nil)
	if err != nil {
		t.Fatalf("expected removal to succeed, got %v", err)
	}

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("failed to read state: %v", err)
	}
	if len(state.InstalledSLCs) != 1 || state.InstalledSLCs[0].Flavor != "java-17" {
		t.Fatalf("expected only java-17 to remain, got %+v", state.InstalledSLCs)
	}
}

func TestInstalledFlavors(t *testing.T) {
	t.Parallel()

	t.Run("missing state file is tolerated as nothing installed", func(t *testing.T) {
		t.Parallel()

		got, err := installedFlavors(config.NewDeploymentDir(t.TempDir()))
		if err != nil {
			t.Fatalf("expected no error for a missing state file, got %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected an empty set, got %v", got)
		}
	})

	t.Run("installed flavors are reported lower-cased", func(t *testing.T) {
		t.Parallel()

		deployment := config.NewDeploymentDir(t.TempDir())
		state := &config.ExasolPersonalState{
			DeploymentVersion: "0.0.0",
			InstalledSLCs:     []config.InstalledSLC{{Flavor: "Python-3.12"}},
		}
		if err := config.WriteExasolPersonalState(state, deployment); err != nil {
			t.Fatalf("failed to write state: %v", err)
		}

		got, err := installedFlavors(deployment)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got["python-3.12"] {
			t.Fatalf("expected python-3.12 to be reported installed, got %v", got)
		}
	})

	t.Run("corrupt state file surfaces an error", func(t *testing.T) {
		t.Parallel()

		deployment := config.NewDeploymentDir(t.TempDir())
		path := deployment.ExasolPersonalStatePath()
		if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
			t.Fatalf("failed to write corrupt state: %v", err)
		}

		if _, err := installedFlavors(deployment); err == nil {
			t.Fatal("expected an error for a corrupt state file, got nil")
		}
	})
}

// requireDeploymentPresent guards SLC change operations: a deployment that has only been
// initialized (never deployed) must be rejected with ErrDeploymentNotPresent, and only a
// deployed deployment is allowed to proceed.
func TestRequireDeploymentPresent(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		state   any
		wantErr bool
	}{
		"initialized but not deployed": {state: &config.WorkflowStateInitialized{}, wantErr: true},
		"deployed and stopped":         {state: &config.WorkflowStateStopped{}, wantErr: false},
		"deployed and running":         {state: &config.WorkflowStateRunning{}, wantErr: false},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			deployment := config.NewDeploymentDir(t.TempDir())
			state := &config.ExasolPersonalState{DeploymentVersion: "0.0.0"}
			if err := state.SetWorkflowStateAndWrite(test.state, deployment); err != nil {
				t.Fatalf("failed to write launcher state: %v", err)
			}

			err := requireDeploymentPresent(deployment)
			if test.wantErr && !errors.Is(err, ErrDeploymentNotPresent) {
				t.Fatalf("expected ErrDeploymentNotPresent, got %v", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestUpsertInstalledSLCAppendsSortedAndReplacesSameFlavor(t *testing.T) {
	t.Parallel()

	existing := []config.InstalledSLC{
		{Language: "python", Flavor: "python-3.12", Image: "img:old", Aliases: []string{"PYTHON3"}},
	}

	withJava := upsertInstalledSLC(existing, config.InstalledSLC{
		Language: "java", Flavor: "java-17", Image: "img:java", Aliases: []string{"JAVA"},
	})
	if len(withJava) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(withJava))
	}
	if withJava[0].Flavor != "java-17" || withJava[1].Flavor != "python-3.12" {
		t.Errorf("expected sorted [java-17, python-3.12], got [%s, %s]",
			withJava[0].Flavor, withJava[1].Flavor)
	}

	replaced := upsertInstalledSLC(withJava, config.InstalledSLC{
		Language: "python", Flavor: "python-3.12", Image: "img:new",
		Aliases: []string{"PYTHON3", "PYTHON312"},
	})
	if len(replaced) != 2 {
		t.Fatalf("expected replace to keep 2 entries, got %d", len(replaced))
	}
	for _, entry := range replaced {
		if entry.Flavor == "python-3.12" && entry.Image != "img:new" {
			t.Errorf("expected python-3.12 image to be replaced with img:new, got %q", entry.Image)
		}
	}
}

func TestFindInstalledSLCMatchesAliasLanguageAndFlavor(t *testing.T) {
	t.Parallel()

	installed := []config.InstalledSLC{
		{Language: "python", Flavor: "python-3.12", Aliases: []string{"PYTHON3", "PYTHON312"}},
	}

	cases := map[string]int{
		"python3":     0,
		"PYTHON312":   0,
		"python":      0,
		"python-3.12": 0,
		"java":        -1,
	}
	for needle, want := range cases {
		if got := findInstalledSLC(installed, needle); got != want {
			t.Errorf("findInstalledSLC(%q) = %d, want %d", needle, got, want)
		}
	}
}

func TestFindInstalledByImage(t *testing.T) {
	t.Parallel()

	installed := []config.InstalledSLC{
		{Flavor: "python-3.12", Image: "docker.io/x:pytag"},
		{Flavor: "java-17", Image: "docker.io/x:javatag"},
	}

	if got := findInstalledByImage(installed, "docker.io/x:javatag"); got != 1 {
		t.Errorf("findInstalledByImage(existing) = %d, want 1", got)
	}
	if got := findInstalledByImage(installed, "docker.io/x:missing"); got != -1 {
		t.Errorf("findInstalledByImage(missing) = %d, want -1", got)
	}
}

func TestLocalRunnerSlcArgsFromState(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{
		InstalledSLCs: []config.InstalledSLC{
			{Flavor: "python-3.12", Image: "docker.io/x:pytag", Target: "/exa/slc/python-3.12"},
			{Flavor: "java-17", Image: "docker.io/x:javatag", Target: "/exa/slc/java-17"},
		},
	}
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		t.Fatalf("failed to write state: %v", err)
	}

	args, err := localRunnerSlcArgs(deployment)
	if err != nil {
		t.Fatalf("localRunnerSlcArgs error: %v", err)
	}

	want := []string{
		"--slc", "docker.io/x:pytag=/exa/slc/python-3.12",
		"--slc", "docker.io/x:javatag=/exa/slc/java-17",
	}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestSLCApplyOutcomeStringAndMarshalText(t *testing.T) {
	t.Parallel()

	cases := map[SLCApplyOutcome]string{
		SLCApplyNone:        "none",
		SLCApplyRestarted:   "restarted",
		SLCApplyStarted:     "started",
		SLCApplyDeferred:    "deferred",
		SLCApplyOutcome(99): "unknown",
	}
	for outcome, want := range cases {
		if got := outcome.String(); got != want {
			t.Errorf("String() for %d = %q, want %q", outcome, got, want)
		}
		text, err := outcome.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText() error: %v", err)
		}
		if string(text) != want {
			t.Errorf("MarshalText() for %d = %q, want %q", outcome, string(text), want)
		}
	}
}

func TestConfirmOrCancel(t *testing.T) {
	t.Parallel()

	if err := confirmOrCancel(nil); err != nil {
		t.Fatalf("expected a nil confirm func to be treated as already-approved, got %v", err)
	}

	approved := func() (bool, error) { return true, nil }
	if err := confirmOrCancel(approved); err != nil {
		t.Fatalf("expected approval to succeed, got %v", err)
	}

	declined := func() (bool, error) { return false, nil }
	if err := confirmOrCancel(declined); !errors.Is(err, ErrSLCOperationCancelled) {
		t.Fatalf("expected ErrSLCOperationCancelled, got %v", err)
	}

	failing := func() (bool, error) { return false, errors.New("prompt failed") }
	if err := confirmOrCancel(failing); err == nil || errors.Is(err, ErrSLCOperationCancelled) {
		t.Fatalf("expected the confirm function's own error to propagate, got %v", err)
	}
}

func TestEntriesFromInstalledAndToInstalledSLCRoundTrip(t *testing.T) {
	t.Parallel()

	const flavor = "python-3.12"
	installed := []config.InstalledSLC{
		{
			Language: "python",
			Flavor:   flavor,
			Version:  "3.12",
			Image:    "docker.io/x:pytag",
			Target:   "/exa/slc/" + flavor,
			Aliases:  []string{"PYTHON3"},
		},
	}

	entries := entriesFromInstalled(installed)

	if len(entries) != 1 || entries[0].Flavor != flavor ||
		entries[0].Image != "docker.io/x:pytag" {
		t.Fatalf("unexpected entries: %+v", entries)
	}

	roundTripped := toInstalledSLC(entries[0])
	if !reflect.DeepEqual(roundTripped, installed[0]) {
		t.Fatalf("expected round-trip to reproduce the original entry, got %+v, want %+v",
			roundTripped, installed[0])
	}
}

func TestInstalledEntriesWrapsStateInstalledSLCs(t *testing.T) {
	t.Parallel()

	const flavor = "python-3.12"
	state := &config.ExasolPersonalState{
		InstalledSLCs: []config.InstalledSLC{{Flavor: flavor}},
	}

	entries := installedEntries(state)

	if len(entries) != 1 || entries[0].Flavor != flavor {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestSLCStatusesSurfacesCatalogErrorsCleanly(t *testing.T) {
	t.Parallel()

	// The embedded SLC catalog only declares arm64 entries today; on any
	// other architecture (this test always runs on the CI/dev host's real
	// GOARCH), SLCStatuses must surface that as a clean error rather than
	// panicking or silently returning an empty list.
	deployment := config.NewDeploymentDir(t.TempDir())

	_, err := SLCStatuses(deployment)

	if runtime.GOARCH == "arm64" {
		if err != nil {
			t.Fatalf("expected no error on a supported architecture, got %v", err)
		}
	} else if err == nil {
		t.Fatal("expected an architecture-unsupported error on an unsupported GOARCH")
	}
}

// TestIsLocalDeploymentRunning_UnresolvableRunnerReturnsFalse mirrors
// TestLocalVMStoppedStatus_UnresolvableRunnerReturnsNil in status_test.go:
// the embedded exasol-local-runner artifact only resolves on darwin/arm64,
// so on any other platform isLocalDeploymentRunning must degrade to false
// rather than propagate the resolution error.
func TestIsLocalDeploymentRunning_UnresolvableRunnerReturnsFalse(t *testing.T) {
	t.Parallel()

	if localRunnerResolvesOnThisPlatform(t) {
		t.Skip("the embedded runner resolves on this platform; this covers the failure path")
	}

	deployment := newLocalTestDeployment(t)

	if isLocalDeploymentRunning(context.Background(), deployment) {
		t.Fatal("expected false when the local runner can't be resolved")
	}
}
