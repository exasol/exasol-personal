// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMacBootHookCreatesFreshVirtioFSDataTarget(t *testing.T) {
	t.Parallel()

	// Given
	fixture := newMacMigrationFixture(t)

	// When
	fixture.run(t, nil)

	// Then
	target := filepath.Join(fixture.dataRoot, "exa")
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("fresh target was not created: info=%v err=%v", info, err)
	}
	fixture.assertApplied(t)
}

func TestMacBootHookMigratesLegacyDirectoryAtomically(t *testing.T) {
	t.Parallel()

	// Given
	fixture := newMacMigrationFixture(t)
	nested := filepath.Join(fixture.legacyData, "data", "row")
	if err := os.MkdirAll(filepath.Dir(nested), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("persisted-row"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("row", filepath.Join(filepath.Dir(nested), "latest")); err != nil {
		t.Fatal(err)
	}

	// When
	fixture.run(t, nil)

	// Then
	targetRow := filepath.Join(fixture.dataRoot, "exa", "data", "row")
	if data, err := os.ReadFile(targetRow); err != nil || string(data) != "persisted-row" {
		t.Fatalf("migrated row = %q, err=%v", data, err)
	}
	if data, err := os.ReadFile(nested); err != nil || string(data) != "persisted-row" {
		t.Fatalf("legacy source was modified: data=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(fixture.dataRoot, "exa.migrating")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("atomic staging directory remains after success: %v", err)
	}
	fixture.assertApplied(t)
}

func TestMacBootHookTreatsExistingTargetAsAuthoritative(t *testing.T) {
	t.Parallel()

	// Given
	fixture := newMacMigrationFixture(t)
	target := filepath.Join(fixture.dataRoot, "exa")
	if err := os.Mkdir(target, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "row"), []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(fixture.legacyData, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.legacyData, "row"), []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}

	// When
	fixture.run(t, []string{"LEGACY_CONTAINER_EXISTS=1"})

	// Then
	if data, err := os.ReadFile(filepath.Join(target, "row")); err != nil || string(data) != "current" {
		t.Fatalf("existing target was replaced: data=%q err=%v", data, err)
	}
	fixture.assertApplied(t)
}

func TestMacBootHookMigratesOverlayBackedLegacyContainer(t *testing.T) {
	t.Parallel()

	// Given
	fixture := newMacMigrationFixture(t)
	overlay := filepath.Join(fixture.root, "overlay")
	if err := os.Mkdir(overlay, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(overlay, "row"), []byte("overlay-row"), 0o600); err != nil {
		t.Fatal(err)
	}

	// When
	fixture.run(t, []string{
		"LEGACY_CONTAINER_EXISTS=1",
		"LEGACY_OVERLAY_SOURCE=" + overlay,
	})

	// Then
	target := filepath.Join(fixture.dataRoot, "exa", "row")
	if data, err := os.ReadFile(target); err != nil || string(data) != "overlay-row" {
		t.Fatalf("overlay row = %q, err=%v", data, err)
	}
	if _, err := os.Stat(fixture.legacyData + ".personal-migration-source"); err != nil {
		t.Fatalf("copied overlay source was not preserved: %v", err)
	}
	fixture.assertApplied(t)
}

func TestMacBootHookFailurePreservesLegacyDataAndDoesNotApply(t *testing.T) {
	t.Parallel()

	// Given
	fixture := newMacMigrationFixture(t)
	if err := os.MkdirAll(fixture.legacyData, 0o750); err != nil {
		t.Fatal(err)
	}
	legacyRow := filepath.Join(fixture.legacyData, "row")
	if err := os.WriteFile(legacyRow, []byte("keep-me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(fixture.bin, "cp"),
		[]byte("#!/bin/sh\nexit 7\n"),
		0o700,
	); err != nil {
		t.Fatal(err)
	}

	// When
	err := fixture.runError(nil)

	// Then
	if err == nil {
		t.Fatal("expected migration copy failure")
	}
	if data, readErr := os.ReadFile(legacyRow); readErr != nil || string(data) != "keep-me" {
		t.Fatalf("legacy source changed after failed migration: data=%q err=%v", data, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(fixture.dataRoot, "exa")); !errors.Is(
		statErr,
		os.ErrNotExist,
	) {
		t.Fatalf("failed migration installed a completion target: %v", statErr)
	}
	if _, statErr := os.Stat(fixture.applied); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("workload was applied after failed migration: %v", statErr)
	}
}

type macMigrationFixture struct {
	root       string
	dataRoot   string
	legacyData string
	helper     string
	script     string
	applied    string
	bin        string
}

func newMacMigrationFixture(t *testing.T) macMigrationFixture {
	t.Helper()
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	legacyData := filepath.Join(root, "var", "lib", "exa")
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(dataRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyData), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(bin, 0o750); err != nil {
		t.Fatal(err)
	}
	applied := filepath.Join(root, "applied")
	helper := filepath.Join(root, "workload-helper")
	helperScript := "#!/bin/sh\nset -eu\n[ \"$1\" = apply ]\ntouch " + shellQuote(applied) + "\n"
	if err := os.WriteFile(helper, []byte(helperScript), 0o700); err != nil {
		t.Fatal(err)
	}
	podman := `#!/bin/sh
set -eu
case "$1 $2" in
  "container exists")
    [ "${LEGACY_CONTAINER_EXISTS:-}" = 1 ]
    ;;
  "stop exasol-local-db")
    ;;
  "cp exasol-local-db:/exa/.")
    cp -a "$LEGACY_OVERLAY_SOURCE/." "$3/"
    ;;
  *)
    echo "unexpected podman command: $*" >&2
    exit 9
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "podman"), []byte(podman), 0o700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "boot-hook")
	if err := os.WriteFile(
		script,
		RenderMacBootHook(dataRoot, legacyData, helper),
		0o700,
	); err != nil {
		t.Fatal(err)
	}

	return macMigrationFixture{
		root:       root,
		dataRoot:   dataRoot,
		legacyData: legacyData,
		helper:     helper,
		script:     script,
		applied:    applied,
		bin:        bin,
	}
}

func (fixture macMigrationFixture) run(t *testing.T, extraEnv []string) {
	t.Helper()
	if err := fixture.runError(extraEnv); err != nil {
		t.Fatalf("boot hook failed: %v", err)
	}
}

func (fixture macMigrationFixture) runError(extraEnv []string) error {
	command := exec.Command("/bin/sh", fixture.script)
	command.Env = append(os.Environ(), "PATH="+fixture.bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	command.Env = append(command.Env, extraEnv...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, output)
	}

	return nil
}

func (fixture macMigrationFixture) assertApplied(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(fixture.applied); err != nil {
		t.Fatalf("workload helper was not applied: %v", err)
	}
}
