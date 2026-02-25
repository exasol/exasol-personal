// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/spf13/cobra"
)

var presetsExportFlags = struct {
	To   string
	Type string
}{}

func requireEmptyDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("target directory %q does not exist", dir)
		}

		return fmt.Errorf("failed to access target directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("target path %q is not a directory", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read target directory %q: %w", dir, err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("target directory %q is not empty", dir)
	}

	return nil
}

func embeddedPresetExists(ids []string, name string) bool {
	needle := strings.TrimSpace(name)
	if needle == "" {
		return false
	}
	for _, id := range ids {
		if id == needle {
			return true
		}
	}

	return false
}

func resolveEmbeddedPresetHandler(presetName string, typeOpt string) (*presetTypeHandler, error) {
	needle := strings.TrimSpace(presetName)
	if needle == "" {
		return nil, errors.New("missing preset name")
	}

	requestedType := normalizePresetTypeFilter(typeOpt)
	if requestedType != "" {
		handler, ok := findPresetTypeHandler(requestedType)
		if !ok {
			return nil, fmt.Errorf(
				"invalid preset type %q (expected one of: %s)",
				typeOpt,
				allowedPresetTypes(),
			)
		}
		if !embeddedPresetExists(handler.ListEmbedded(), needle) {
			return nil, fmt.Errorf("unknown %s preset %q", handler.Type, needle)
		}

		return handler, nil
	}

	matches := make([]*presetTypeHandler, 0, 1)
	for i := range presetTypeHandlers {
		handler := &presetTypeHandlers[i]
		if embeddedPresetExists(handler.ListEmbedded(), needle) {
			matches = append(matches, handler)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("unknown preset %q", needle)
	case 1:
		return matches[0], nil
	default:
		types := make([]string, 0, len(matches))
		for _, handler := range matches {
			types = append(types, handler.Type)
		}

		return nil, fmt.Errorf(
			"preset %q exists for multiple types (%s); use --type to select one",
			needle,
			strings.Join(types, ", "),
		)
	}
}

var presetsExportCmd = &cobra.Command{
	Use:   "export <preset>",
	Short: "Export an embedded preset to a directory",
	Long: `Export an embedded preset to a local directory.

The target directory must exist and be empty.

By default, the current working directory is used.
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		presetName := args[0]
		target := presetsExportFlags.To
		if strings.TrimSpace(target) == "" {
			target = "."
		}

		if err := requireEmptyDir(target); err != nil {
			return err
		}

		handler, err := resolveEmbeddedPresetHandler(presetName, presetsExportFlags.Type)
		if err != nil {
			return err
		}

		ref := presets.PresetRef{Name: presetName}
		if err := presets.ExtractPreset(ref, target, handler.WriteDir); err != nil {
			return fmt.Errorf("failed to export preset %q: %w", presetName, err)
		}

		return nil
	},
}

func registerPresetsExportCmd(parent *cobra.Command) {
	parent.AddCommand(presetsExportCmd)

	presetsExportCmd.Flags().StringVar(
		&presetsExportFlags.To,
		"to",
		"",
		"Target directory to export to (must be empty)",
	)
	presetsExportCmd.Flags().StringVarP(
		&presetsExportFlags.Type,
		"type",
		"t",
		"",
		"Preset type (use when the same preset name exists for multiple types)",
	)
}
