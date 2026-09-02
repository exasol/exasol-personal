// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/localinstall"
)

const migrationSparseFileSize = int64(64 * 1024 * 1024)

func TestMacDataMigrationCopiesAndRetainsLegacyData(t *testing.T) {
	t.Parallel()

	// Given
	migration := newTestMacDataMigration(t)
	wantTime := time.Unix(1577934245, 0)
	writeMigrationFixture(t, migration.sourceDir, wantTime)
	if err := os.MkdirAll(migration.targetDir, dirMode); err != nil {
		t.Fatal(err)
	}
	stopCalls := 0

	// When
	err := migration.prepare(context.Background(), io.Discard, io.Discard, func() error {
		stopCalls++

		return nil
	})
	// Then
	if err != nil {
		t.Fatalf("prepare migration failed: %v", err)
	}
	if stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", stopCalls)
	}
	assertMigrationFixture(t, migration.targetDir, wantTime)
	if _, err := os.Stat(filepath.Join(migration.sourceDir, "sparse")); err != nil {
		t.Fatalf("legacy source changed before finalization: %v", err)
	}
	assertFileContents(
		t, filepath.Join(migration.targetDir, hostLayoutMarkerName), hostLayoutMarkerContents,
	)

	// When
	err = migration.finalize(context.Background(), io.Discard, io.Discard)
	// Then
	if err != nil {
		t.Fatalf("finalize migration failed: %v", err)
	}
	if _, err := os.Stat(migration.sourceDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy source still active after finalization: %v", err)
	}
	assertMigrationFixture(t, migration.backupDir, wantTime)
}

func TestMacDataMigrationRetriesInterruptedCopy(t *testing.T) {
	t.Parallel()

	// Given
	migration := newTestMacDataMigration(t)
	writeMigrationFixture(t, migration.sourceDir, time.Unix(1577934245, 0))
	failing := &copyFailingEnvironment{
		ExecutionEnvironment: migration.environment,
		failNextCopy:         true,
	}
	migration.environment = failing
	stopCalls := 0
	stopNano := func() error {
		stopCalls++

		return nil
	}

	// When
	firstErr := migration.prepare(context.Background(), io.Discard, io.Discard, stopNano)
	failing.failNextCopy = false
	secondErr := migration.prepare(context.Background(), io.Discard, io.Discard, stopNano)

	// Then
	if firstErr == nil || !strings.Contains(firstErr.Error(), migration.sourceDir) ||
		!strings.Contains(firstErr.Error(), migration.targetDir) {
		t.Fatalf("copy failure does not identify both locations: %v", firstErr)
	}
	if secondErr != nil {
		t.Fatalf("migration retry failed: %v", secondErr)
	}
	if stopCalls != 2 {
		t.Fatalf("stop calls = %d, want 2", stopCalls)
	}
	if failing.copyCalls != 2 {
		t.Fatalf("copy calls = %d, want 2", failing.copyCalls)
	}
	if _, err := os.Stat(filepath.Join(migration.sourceDir, "sparse")); err != nil {
		t.Fatalf("legacy source changed during retry: %v", err)
	}
}

func TestMacDataMigrationResumesAfterPublication(t *testing.T) {
	t.Parallel()

	// Given
	migration := newTestMacDataMigration(t)
	writeMigrationFixture(t, migration.sourceDir, time.Unix(1577934245, 0))
	counting := &copyFailingEnvironment{ExecutionEnvironment: migration.environment}
	migration.environment = counting
	if err := migration.prepare(context.Background(), io.Discard, io.Discard, func() error {
		return nil
	}); err != nil {
		t.Fatalf("initial migration failed: %v", err)
	}
	stopCalls := 0

	// When
	err := migration.prepare(context.Background(), io.Discard, io.Discard, func() error {
		stopCalls++

		return nil
	})
	// Then
	if err != nil {
		t.Fatalf("migration resume failed: %v", err)
	}
	if stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", stopCalls)
	}
	if counting.copyCalls != 1 {
		t.Fatalf("copy calls = %d, want the published data to be reused", counting.copyCalls)
	}
}

func TestMacDataMigrationRejectsPopulatedSourceAndTarget(t *testing.T) {
	t.Parallel()

	// Given
	migration := newTestMacDataMigration(t)
	writeTestPath(t, filepath.Join(migration.sourceDir, "source"), "source")
	writeTestPath(t, filepath.Join(migration.targetDir, "target"), "target")
	stopCalled := false

	// When
	err := migration.prepare(context.Background(), io.Discard, io.Discard, func() error {
		stopCalled = true

		return nil
	})

	// Then
	if err == nil || !strings.Contains(err.Error(), migration.sourceDir) ||
		!strings.Contains(err.Error(), migration.targetDir) {
		t.Fatalf("conflict error does not identify both locations: %v", err)
	}
	if stopCalled {
		t.Fatal("Nano was stopped before the data conflict was rejected")
	}
	assertFileContents(t, filepath.Join(migration.sourceDir, "source"), "source")
	assertFileContents(t, filepath.Join(migration.targetDir, "target"), "target")
}

func TestMacDataMigrationMarksFreshHostLayoutAfterReadiness(t *testing.T) {
	t.Parallel()

	// Given
	migration := newTestMacDataMigration(t)
	writeTestPath(t, filepath.Join(migration.targetDir, "exasol.conf"), "fresh")

	// When
	err := migration.finalize(context.Background(), io.Discard, io.Discard)
	// Then
	if err != nil {
		t.Fatalf("finalizing fresh host layout failed: %v", err)
	}
	assertFileContents(
		t, filepath.Join(migration.targetDir, hostLayoutMarkerName), hostLayoutMarkerContents,
	)
}

type copyFailingEnvironment struct {
	localinstall.ExecutionEnvironment

	failNextCopy bool
	copyCalls    int
}

func (environment *copyFailingEnvironment) Run(
	ctx context.Context,
	stdin io.Reader,
	stdout, stderr io.Writer,
	command ...string,
) error {
	if len(command) > 0 && command[0] == "cp" {
		environment.copyCalls++
		if environment.failNextCopy {
			return errors.New("injected copy failure")
		}
	}

	return environment.ExecutionEnvironment.Run(ctx, stdin, stdout, stderr, command...)
}

func newTestMacDataMigration(t *testing.T) *macDataMigration {
	t.Helper()
	root := t.TempDir()

	migration := newMacDataMigration(localinstall.NewDirectExecutionEnvironment(nil))
	migration.sourceDir = filepath.Join(root, "guest", "exa")
	migration.targetDir = filepath.Join(root, "host", "exa")
	migration.stagingDir = filepath.Join(root, "host", ".exa-migration")
	migration.backupDir = filepath.Join(root, "guest", "exa.migrated-backup")

	return migration
}

func writeMigrationFixture(t *testing.T, directory string, modificationTime time.Time) {
	t.Helper()
	if err := os.MkdirAll(directory, dirMode); err != nil {
		t.Fatal(err)
	}
	sparsePath := filepath.Join(directory, "sparse")
	file, err := os.OpenFile(sparsePath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(migrationSparseFileSize); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte("x"), migrationSparseFileSize-1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(sparsePath, modificationTime, modificationTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("sparse", filepath.Join(directory, "link")); err != nil {
		t.Fatal(err)
	}
}

func assertMigrationFixture(t *testing.T, directory string, modificationTime time.Time) {
	t.Helper()
	fileInfo, err := os.Stat(filepath.Join(directory, "sparse"))
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Size() != migrationSparseFileSize {
		t.Fatalf("copied file size = %d, want %d", fileInfo.Size(), migrationSparseFileSize)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("copied file mode = %o, want 600", fileInfo.Mode().Perm())
	}
	if !fileInfo.ModTime().Equal(modificationTime) {
		t.Fatalf(
			"copied file modification time = %v, want %v",
			fileInfo.ModTime(), modificationTime,
		)
	}
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("unexpected stat type for copied sparse file")
	}
	if allocated := stat.Blocks * 512; allocated >= migrationSparseFileSize/2 {
		t.Fatalf("copied file allocated %d bytes for logical size %d",
			allocated, migrationSparseFileSize)
	}
	linkTarget, err := os.Readlink(filepath.Join(directory, "link"))
	if err != nil {
		t.Fatal(err)
	}
	if linkTarget != "sparse" {
		t.Fatalf("copied symbolic link target = %q, want sparse", linkTarget)
	}
}

func writeTestPath(t *testing.T, filePath, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), dirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(contents), markerFileMode); err != nil {
		t.Fatal(err)
	}
}

func assertFileContents(t *testing.T, filePath, want string) {
	t.Helper()
	contents, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != want {
		t.Fatalf("file %s = %q, want %q", filePath, contents, want)
	}
}
