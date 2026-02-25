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

	"gopkg.in/yaml.v3"
)

var ErrNoFileMatchedGlobPattern = errors.New("no file matched the pattern")

func findGlob(dir string, pattern string) (string, error) {
	globPath := filepath.Join(dir, pattern)
	matches, err := filepath.Glob(globPath)
	if err != nil {
		return "", err
	}
	if len(matches) > 0 {
		return matches[0], nil
	}

	return "", fmt.Errorf("%w: %s", ErrNoFileMatchedGlobPattern, globPath)
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

func readConfig[T any](path string, name string) (*T, error) {
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
