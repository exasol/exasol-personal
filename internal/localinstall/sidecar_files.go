// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/exasol/exasol-personal/internal/sidecar"
)

const (
	// Only the container sees its copy, so every user it runs as may read it,
	// and only its root user may change it.
	volumeFileMode    = 0o444
	volumeDirMode     = 0o555
	appliedRecordMode = 0o600
)

// filesArchive lays out the volume files relative to the container root. Only
// the mount and the directories below it get entries, so directories the image
// already has keep their permissions.
func filesArchive(files []sidecar.File) ([]byte, error) {
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	written := map[string]bool{}
	for _, file := range files {
		name := strings.TrimPrefix(file.Path, "/")
		for _, directory := range mountDirectories(file) {
			if written[directory] {
				continue
			}
			written[directory] = true
			if err := writer.WriteHeader(&tar.Header{
				Typeflag: tar.TypeDir, Name: directory + "/", Mode: volumeDirMode,
			}); err != nil {
				return nil, err
			}
		}
		content := []byte(file.Content)
		if err := writer.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg, Name: name, Mode: volumeFileMode, Size: int64(len(content)),
		}); err != nil {
			return nil, err
		}
		if _, err := writer.Write(content); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	return archive.Bytes(), nil
}

func mountDirectories(file sidecar.File) []string {
	mount := strings.TrimPrefix(file.MountPath, "/")
	directories := []string{mount}
	relative := strings.TrimPrefix(file.Path, file.MountPath+"/")
	for directory := path.Dir(relative); directory != "."; directory = path.Dir(directory) {
		directories = append(directories, mount+"/"+directory)
	}
	slices.Sort(directories)

	return directories
}

// The record holds a digest of the sensitive values a container was given, so
// a rotated password replaces a container whose definition is unchanged while
// the label stays free of it.
func (runtime *SidecarRuntime) recordPath(name string) string {
	return filepath.Join(runtime.EnvDir.HostPath, name+".applied")
}

func appliedDigest(resolved sidecar.Resolved, files []sidecar.File) string {
	digest := sha256.New()
	for _, variable := range resolved.Container.Env {
		if resolved.Sensitive(variable.Name) {
			_, _ = fmt.Fprintf(digest, "env %s=%s\x00", variable.Name, *variable.Value)
		}
	}
	for _, file := range files {
		_, _ = fmt.Fprintf(digest, "file %s=%s\x00", file.Path, file.Content)
	}

	return hex.EncodeToString(digest.Sum(nil))
}

func appliedValues(resolved sidecar.Resolved, files []sidecar.File) bool {
	return len(files) > 0 || slices.ContainsFunc(
		resolved.Container.Env,
		func(variable sidecar.EnvVar) bool { return resolved.Sensitive(variable.Name) },
	)
}

func (runtime *SidecarRuntime) appliedCurrent(
	name string, resolved sidecar.Resolved, files []sidecar.File,
) (bool, error) {
	tracked := appliedValues(resolved, files)
	if runtime.EnvDir.HostPath == "" {
		return !tracked, nil
	}
	recorded, err := os.ReadFile(runtime.recordPath(name))
	if errors.Is(err, os.ErrNotExist) {
		return !tracked, nil
	}
	if err != nil {
		return false, fmt.Errorf("sidecar %s: read applied record: %w", name, err)
	}

	return tracked && string(recorded) == appliedDigest(resolved, files), nil
}

func (runtime *SidecarRuntime) recordApplied(
	name string, resolved sidecar.Resolved, files []sidecar.File,
) error {
	if !appliedValues(resolved, files) {
		return runtime.removeRecord(name)
	}
	if runtime.EnvDir.HostPath == "" {
		return errors.New("sidecar runtime was created without an environment directory")
	}
	if err := os.MkdirAll(runtime.EnvDir.HostPath, envDirMode); err != nil {
		return fmt.Errorf("sidecar %s: prepare applied record: %w", name, err)
	}
	if err := os.WriteFile(
		runtime.recordPath(name), []byte(appliedDigest(resolved, files)), appliedRecordMode,
	); err != nil {
		return fmt.Errorf("sidecar %s: write applied record: %w", name, err)
	}

	return nil
}

// PruneRecords removes the records of sidecars outside desired, so a container
// removed without the launcher leaves no record behind once its sidecar goes.
func (runtime *SidecarRuntime) PruneRecords(desired []string) error {
	if runtime.EnvDir.HostPath == "" {
		return nil
	}
	entries, err := os.ReadDir(runtime.EnvDir.HostPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list applied records: %w", err)
	}
	var failures []error
	for _, entry := range entries {
		name, recorded := strings.CutSuffix(entry.Name(), ".applied")
		if !recorded || entry.IsDir() || slices.Contains(desired, name) {
			continue
		}
		failures = append(failures, runtime.removeRecord(name))
	}

	return errors.Join(failures...)
}

func (runtime *SidecarRuntime) removeRecord(name string) error {
	if runtime.EnvDir.HostPath == "" {
		return nil
	}
	err := os.Remove(runtime.recordPath(name))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("sidecar %s: remove applied record: %w", name, err)
	}

	return nil
}

// createWithFiles copies the volume files into the container before its
// process starts, so they never pass through its configuration.
func (runtime *SidecarRuntime) createWithFiles(
	ctx context.Context, spec ContainerSpec, archive []byte, outErr io.Writer,
) error {
	if err := runtime.client.CreateStopped(ctx, spec, io.Discard, outErr); err != nil {
		return err
	}
	if err := runtime.client.CopyTo(
		ctx, spec.Name, "/", bytes.NewReader(archive), io.Discard, outErr,
	); err != nil {
		return err
	}

	return runtime.client.Start(ctx, spec.Name, io.Discard, outErr)
}
