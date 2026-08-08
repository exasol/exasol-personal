// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/presets"
)

// fullLifecycleRunnerScript builds a fake exasol-local-runner responding to
// every verb startLocalRuntime/stopLocalRuntime/destroyLocalRuntime issue.
// "start" writes a fake VM state file directly (mirroring writeFakeVMState),
// since startPreparedLocalRuntime removes any prior state before running
// "start" and then immediately reads it back.
func fullLifecycleRunnerScript(statePath string) string {
	stateJSON := `{"vm_name":"exasol-local-vm","vm_ip":"127.0.0.1",` +
		`"ports":{"ssh":20022,"db":8563,"ui":443}}`

	return "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  version) printf 'v1.0.0\\n' ;;\n" +
		"  init) mkdir -p vm ;;\n" +
		"  start) cat > " + statePath + " <<'EOF'\n" + stateJSON + "\nEOF\n" +
		"    ;;\n" +
		"  stop) exit 0 ;;\n" +
		"  *) exit 0 ;;\n" +
		"esac\n"
}

// newTestLocalBackend builds a localBackend against a real local-backend
// deployment, backed by a fake exasol-local-runner that supports the full
// version/init/start/stop lifecycle.
func newTestLocalBackend(t *testing.T) *localBackend {
	t.Helper()
	skipOnWindows(t)
	t.Setenv(localAllowUnsupportedEnv, "1")
	t.Setenv(localSkipDatabaseWaitEnv, "1")

	deployment := newLocalTestDeployment(t)
	ensureLocalRuntimeWorkDir(t, deployment)
	state := &config.ExasolPersonalState{DeploymentVersion: "0.0.0", VersionCheckEnabled: false}
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		t.Fatalf("failed to write launcher state: %v", err)
	}

	paths := localruntime.NewPaths(deployment)
	manager := newTestManagerForRunner(t, []byte(fullLifecycleRunnerScript(paths.StatePath)))
	manifest := &presets.InfrastructureManifest{Backend: backendTypeLocal}

	backend := newLocalBackend(deployment, manifest, localruntime.New(deployment, manager))

	return backend
}

// nolint: paralleltest // avoids concurrent extract+exec of the fake runner (ETXTBSY flakes).
func TestLocalBackendDeploySucceedsThroughPrepareAndStart(t *testing.T) {
	backend := newTestLocalBackend(t)

	var out, outErr bytes.Buffer

	err := backend.Deploy(context.Background(), &out, &outErr, DeployOptions{})
	if err != nil {
		t.Fatalf("expected deploy to succeed, got %v", err)
	}

	info, err := config.ReadDeploymentInfo(backend.deployment)
	if err != nil {
		t.Fatalf("expected deployment info to be written, got %v", err)
	}
	if info.Connection == nil || info.Connection.DBPort != 8563 {
		t.Fatalf("expected connection info from the fake VM state, got %+v", info.Connection)
	}
}

// nolint: paralleltest // avoids concurrent extract+exec of the fake runner (ETXTBSY flakes).
func TestLocalBackendStartSucceedsThroughPrepareAndStart(t *testing.T) {
	backend := newTestLocalBackend(t)

	var out, outErr bytes.Buffer

	err := backend.Start(context.Background(), &out, &outErr, 0)
	if err != nil {
		t.Fatalf("expected start to succeed, got %v", err)
	}
}

// nolint: paralleltest // avoids concurrent extract+exec of the fake runner (ETXTBSY flakes).
func TestLocalBackendStopUpdatesDeploymentState(t *testing.T) {
	backend := newTestLocalBackend(t)

	if err := config.WriteDeploymentInfo(backend.deployment.Root(), &config.DeploymentInfo{
		Backend:      localDeploymentBackend,
		DeploymentId: "local",
	}); err != nil {
		t.Fatalf("failed to seed deployment info: %v", err)
	}

	var out, outErr bytes.Buffer
	if err := backend.Stop(context.Background(), &out, &outErr); err != nil {
		t.Fatalf("expected stop to succeed, got %v", err)
	}

	info, err := config.ReadDeploymentInfo(backend.deployment)
	if err != nil {
		t.Fatalf("failed to read deployment info: %v", err)
	}
	if info.DeploymentState != StatusStopped {
		t.Fatalf("expected deployment state %q, got %q", StatusStopped, info.DeploymentState)
	}
}

// nolint: paralleltest // avoids concurrent extract+exec of the fake runner (ETXTBSY flakes).
func TestLocalBackendDestroyRemovesRuntimeAndArtifacts(t *testing.T) {
	backend := newTestLocalBackend(t)

	if err := config.WriteDeploymentInfo(backend.deployment.Root(), &config.DeploymentInfo{
		Backend:      localDeploymentBackend,
		DeploymentId: "local",
	}); err != nil {
		t.Fatalf("failed to seed deployment info: %v", err)
	}
	if err := config.WriteSecrets(backend.deployment.Root(), &config.Secrets{
		DbPassword: "x",
	}); err != nil {
		t.Fatalf("failed to seed secrets: %v", err)
	}

	var out, outErr bytes.Buffer
	if err := backend.Destroy(context.Background(), &out, &outErr); err != nil {
		t.Fatalf("expected destroy to succeed, got %v", err)
	}

	if _, err := os.Stat(backend.deployment.NodeDetailsPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected node details to be removed, got %v", err)
	}
	if _, err := os.Stat(backend.deployment.SecretsPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected secrets to be removed, got %v", err)
	}
}
