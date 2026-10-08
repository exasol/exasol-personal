// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/exasol/exasol-personal/internal/util"
	"gopkg.in/yaml.v3"
)

var ErrNoFileMatchedGlobPattern = errors.New("no file matched the pattern")

// findExistingFile checks if a given filename exists in dir.
//
// It returns (fullPath, true, nil) if the file exists, (fullPath, false, nil)
// if it does not exist, and ("", false, err) for unexpected errors.
func findExistingFile(dir, filename string) (string, bool, error) {
	fullPath := filepath.Join(dir, filename)
	_, err := os.Stat(fullPath)
	if err == nil {
		return fullPath, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return fullPath, false, nil
	}

	return "", false, err
}

var ErrMissingConfigFile = errors.New("failed to load config file")

func writeConfig(config any, path string, name string) error {
	configFile, err := os.OpenFile(
		path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0o600) // nolint: mnd
	if err != nil {
		slog.Error(
			"failed to open config file",
			"type", name,
			"path", path,
		)

		return err
	}

	defer configFile.Close()

	var data []byte

	ext := filepath.Ext(path)
	switch ext {
	case ".yaml":
		data, err = yaml.Marshal(config)
		if err != nil {
			slog.Error("failed to encode yaml config file",
				"type", name,
				"path", path,
				"error", err.Error())

			return err
		}
	case ".json":
		data, err = json.Marshal(config)
		if err != nil {
			slog.Error("failed to encode json config file",
				"type", name,
				"path", path,
				"error", err.Error())

			return err
		}
	default:
		slog.Error("unrecognized config file path extension while writing", "extension", ext)
		panic("unrecognized config file path extension while writing")
	}

	_, err = configFile.Write(data)
	if err != nil {
		slog.Error("failed to write to config file",
			"type", name,
			"path", path,
			"error", err.Error())

		return err
	}

	slog.Debug("new config file written", "type", name, "path", path)

	return nil
}

// beforeAtomicRename lets tests act between writing the temporary file and
// replacing the target.
var beforeAtomicRename = func() { /* no-op outside tests */ }

// WriteFileAtomically replaces path with data so that readers and crashes see
// either the previous content or the new content, never a partial file.
// Callers serialize writes to path, as deployment files are written under the
// deployment lock, so any existing temporary file for path is stale.
func WriteFileAtomically(path string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	temporaryPrefix := "." + filepath.Base(path) + ".tmp-"
	if err := removeStaleTemporaries(directory, temporaryPrefix); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, temporaryPrefix+"*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	// The launcher exits right after its signal handlers run, so a deferred
	// removal alone would leave the temporary copy behind on an interrupt.
	removeTemporary := util.EnsureOnInterrupt(func() { _ = os.Remove(temporaryPath) })
	defer removeTemporary()

	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	beforeAtomicRename()
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}

	return syncDirectory(directory)
}

// temporaryFiles lists the files in directory whose names start with prefix.
// The directory is read literally, so path characters are never treated as
// glob syntax.
func temporaryFiles(directory, prefix string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
	}

	return paths, nil
}

func removeStaleTemporaries(directory, prefix string) error {
	stale, err := temporaryFiles(directory, prefix)
	if err != nil {
		return err
	}
	for _, path := range stale {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove stale temporary file %s: %w", path, err)
		}
	}

	return nil
}

// syncDirectory makes a completed rename durable. Windows cannot open a
// directory for syncing, and NTFS journals the rename itself.
func syncDirectory(directory string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()

	return handle.Sync()
}

func readConfig[T any](path, name string) (*T, error) {
	configFile, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf(
				"%w: failed to open %s file \"%s\"", ErrMissingConfigFile, name, path)
		}

		return nil, err
	}

	slog.Debug("reading config file", "file", path)

	defer configFile.Close()

	var config T

	ext := filepath.Ext(path)
	switch ext {
	case ".yaml":
		decoder := yaml.NewDecoder(configFile)
		err = decoder.Decode(&config)
		if err != nil {
			return nil, err
		}
	case ".json":
		decoder := json.NewDecoder(configFile)
		err = decoder.Decode(&config)
		if err != nil {
			return nil, err
		}
	default:
		slog.Error("unrecognized config file path extension while writing", "extension", ext)
		panic("unrecognized config file path extension while reading")
	}

	return &config, nil
}
