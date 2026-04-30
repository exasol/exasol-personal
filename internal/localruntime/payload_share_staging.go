// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	guestPayloadShareTag   = "hostshare"
	guestPayloadShareMount = "/mnt/host"
)

//go:embed start.sh
var embeddedStartScript []byte

// StagePayloadShare populates the per-deployment payload-share directory with
// the launcher-authored start script and the cached .run binary. The start
// script is rewritten on every call so launcher-version updates always take
// effect; the .run is skipped on checksum match.
func (r *Runtime) StagePayloadShare(cachedRunPath string) error {
	source := strings.TrimSpace(cachedRunPath)
	if source == "" {
		return errors.New("local runtime payload run source path is required")
	}

	shareDir := r.layout.PayloadShareDir()
	if err := os.MkdirAll(shareDir, localRuntimeDirMode); err != nil {
		return fmt.Errorf("failed to create local runtime payload share dir: %w", err)
	}

	if err := writeStartScript(r.layout.PayloadStartScriptPath()); err != nil {
		return err
	}

	return stageRunBinary(source, r.layout.PayloadRunPath(), r.layout.PayloadRunChecksumPath())
}

func writeStartScript(targetPath string) error {
	if err := os.WriteFile(targetPath, embeddedStartScript, localRuntimeExecFileMode); err != nil {
		return fmt.Errorf("failed to write staged start script: %w", err)
	}
	if err := os.Chmod(targetPath, localRuntimeExecFileMode); err != nil {
		return fmt.Errorf("failed to mark staged start script executable: %w", err)
	}

	return nil
}

func stageRunBinary(sourcePath string, targetPath string, checksumPath string) error {
	expected, err := fileSHA256(sourcePath)
	if err != nil {
		return err
	}

	reuseable, err := stagedRunReusable(targetPath, checksumPath, expected)
	if err != nil {
		return err
	}
	if reuseable {
		return os.Chmod(targetPath, localRuntimeExecFileMode)
	}

	if err := streamCopyFile(sourcePath, targetPath, localRuntimeExecFileMode); err != nil {
		return err
	}
	if err := os.Chmod(targetPath, localRuntimeExecFileMode); err != nil {
		return fmt.Errorf("failed to mark staged run executable: %w", err)
	}
	if err := os.WriteFile(
		checksumPath,
		[]byte(expected+"\n"),
		localRuntimeFileMode,
	); err != nil {
		return fmt.Errorf("failed to write staged run checksum: %w", err)
	}

	return nil
}

func stagedRunReusable(targetPath string, checksumPath string, expected string) (bool, error) {
	if !isCachedFile(targetPath) {
		return false, nil
	}
	recorded, err := os.ReadFile(checksumPath)
	if err == nil {
		return strings.TrimSpace(string(recorded)) == expected, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("failed to read staged run checksum: %w", err)
	}
	if removeErr := os.Remove(targetPath); removeErr != nil &&
		!errors.Is(removeErr, os.ErrNotExist) {
		return false, fmt.Errorf("failed to remove stale staged run: %w", removeErr)
	}

	return false, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open run binary for hashing: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to hash run binary: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
