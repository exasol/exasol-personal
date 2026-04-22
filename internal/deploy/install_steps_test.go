// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/presets"
)

func TestBuildInstallTasks_PreservesRemoteAndLocalSteps(t *testing.T) {
	t.Parallel()

	// Given
	manifest := &presets.InstallManifest{
		Install: presets.InstallSteps{
			{
				RemoteExec: &presets.RemoteExecTask{
					Description: "remote",
					Filename:    "monitor.sh",
					Node:        "n11",
				},
			},
			{
				LocalCommand: &presets.LocalCommandTask{
					Description: "local",
					Command:     []string{"echo", "hello"},
				},
			},
		},
	}

	// When
	tasks := buildInstallTasks(manifest)

	// Then
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].RemoteExec == nil {
		t.Fatal("expected first task to be remoteExec")
	}
	if tasks[0].RemoteExec.Filename != filepath.Join("installation", "monitor.sh") {
		t.Fatalf("unexpected remote filename: %q", tasks[0].RemoteExec.Filename)
	}
	if tasks[1].LocalCommand == nil {
		t.Fatal("expected second task to be localCommand")
	}
	if got := tasks[1].LocalCommand.Command; len(got) != 2 || got[0] != "echo" || got[1] != "hello" {
		t.Fatalf("unexpected local command: %#v", got)
	}
}
