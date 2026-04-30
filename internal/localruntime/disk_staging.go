// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var stageDiskCopyCommand = func(source string, target string) *exec.Cmd {
	return exec.Command("cp", "-c", source, target)
}

func (r *Runtime) StagedDiskImagePath() string {
	return r.layout.DiskImagePath()
}

func (r *Runtime) EFIVarsPath() string {
	return r.layout.EFIVarsPath()
}

// StageDiskImage copies the cached disk image into a per-deployment writable
// path. Subsequent calls with the same source identity are no-ops; a different
// source identity triggers a fresh copy.
func (r *Runtime) StageDiskImage(sourcePath string) (string, error) {
	source := strings.TrimSpace(sourcePath)
	if source == "" {
		return "", errors.New("local runtime disk image source path is required")
	}

	target := r.layout.DiskImagePath()
	identityPath := r.layout.DiskIdentityPath()

	if err := os.MkdirAll(filepath.Dir(target), localRuntimeDirMode); err != nil {
		return "", fmt.Errorf("failed to create local runtime VM dir: %w", err)
	}

	identity, err := diskSourceIdentity(source)
	if err != nil {
		return "", err
	}

	if isCachedFile(target) {
		recorded, readErr := os.ReadFile(identityPath)
		if readErr == nil && strings.TrimSpace(string(recorded)) == identity {
			return target, nil
		}
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return "", fmt.Errorf("failed to read staged disk identity: %w", readErr)
		}

		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("failed to remove stale staged disk image: %w", err)
		}
	}

	if err := copyStagedDisk(source, target); err != nil {
		return "", err
	}

	if err := os.WriteFile(
		identityPath,
		[]byte(identity+"\n"),
		localRuntimeFileMode,
	); err != nil {
		return "", fmt.Errorf("failed to write staged disk identity: %w", err)
	}

	return target, nil
}

func diskSourceIdentity(source string) (string, error) {
	info, err := os.Stat(source)
	if err != nil {
		return "", fmt.Errorf("failed to stat cached disk image: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("cached disk image path %q is a directory", source)
	}

	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%d\x00", source, info.Size(), info.ModTime().UnixNano())

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyStagedDisk(source string, target string) error {
	cmd := stageDiskCopyCommand(source, target)
	if cmd != nil {
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to reset staged disk image before fallback copy: %w", err)
	}

	return streamCopyFile(source, target, localRuntimeFileMode)
}

func streamCopyFile(sourcePath string, targetPath string, mode os.FileMode) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source disk image: %w", err)
	}
	defer sourceFile.Close()

	targetFile, err := os.OpenFile(
		targetPath,
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY,
		mode,
	)
	if err != nil {
		return fmt.Errorf("failed to create staged disk image: %w", err)
	}

	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		_ = targetFile.Close()
		return fmt.Errorf("failed to copy disk image to staged path: %w", err)
	}

	if err := targetFile.Close(); err != nil {
		return fmt.Errorf("failed to finalize staged disk image: %w", err)
	}

	return nil
}
