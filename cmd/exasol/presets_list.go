// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

var presetsListFlags = struct {
	TypeFilter string
}{}

func filterPresetCatalog(typeFilter string) (PresetCatalog, error) {
	filter := normalizePresetTypeFilter(typeFilter)
	cat := GetPresetCatalog()
	filtered := PresetCatalog{}

	if filter != "" {
		if _, ok := findPresetTypeHandler(filter); !ok {
			return PresetCatalog{}, fmt.Errorf(
				"invalid preset type %q (expected one of: %s)",
				typeFilter,
				allowedPresetTypes(),
			)
		}
	}

	for _, h := range presetTypeHandlers {
		if filter != "" && filter != h.Type {
			continue
		}
		h.SetOnCatalog(&filtered, h.GetFromCatalog(cat))
	}

	return filtered, nil
}

func renderPresetListJSON(writer io.Writer, catalog PresetCatalog) error {
	enc := json.NewEncoder(writer)
	enc.SetIndent("", "  ")

	return enc.Encode(catalog)
}

func formatPresetLine(preset Preset) string {
	text := strings.TrimSpace(preset.Name)
	desc := strings.TrimSpace(preset.Description)
	var suffix string
	switch {
	case text != "" && desc != "" && text != desc:
		suffix = text + ": " + desc
	case desc != "":
		suffix = desc
	case text != "":
		suffix = text
	default:
		suffix = "(no description)"
	}

	return fmt.Sprintf("  %s - %s\n", preset.ID, suffix)
}

func renderPresetSection(writer io.Writer, header string, presetsList []Preset) error {
	if _, err := fmt.Fprintf(writer, "%s\n", header); err != nil {
		return err
	}

	if len(presetsList) == 0 {
		_, err := fmt.Fprint(writer, "  (none)\n")
		return err
	}

	for _, p := range presetsList {
		if strings.TrimSpace(p.ID) == "" {
			continue
		}
		if _, err := fmt.Fprint(writer, formatPresetLine(p)); err != nil {
			return err
		}
	}

	return nil
}

func renderPresetListText(writer io.Writer, typeFilter string, catalog PresetCatalog) error {
	filter := normalizePresetTypeFilter(typeFilter)
	if filter != "" {
		if _, ok := findPresetTypeHandler(filter); !ok {
			return fmt.Errorf(
				"invalid preset type %q (expected one of: %s)",
				typeFilter,
				allowedPresetTypes(),
			)
		}
	}

	wroteAny := false
	writeSection := func(header string, presetsList []Preset) error {
		if wroteAny {
			if _, err := fmt.Fprintln(writer); err != nil {
				return err
			}
		}
		wroteAny = true

		return renderPresetSection(writer, header, presetsList)
	}

	for _, h := range presetTypeHandlers {
		if filter != "" && filter != h.Type {
			continue
		}
		if err := writeSection(h.Header, h.GetFromCatalog(catalog)); err != nil {
			return err
		}
	}

	return nil
}

var presetsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available presets",
	Long: `List available embedded presets.

By default, the output is human-readable.
Use the '--json' option to print the output in JSON format.
`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		catalog, err := filterPresetCatalog(presetsListFlags.TypeFilter)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		if commonFlags.OutputJson {
			return renderPresetListJSON(out, catalog)
		}

		return renderPresetListText(out, presetsListFlags.TypeFilter, catalog)
	},
}

func registerPresetsListCmd(parent *cobra.Command) {
	parent.AddCommand(presetsListCmd)

	presetsListCmd.Flags().StringVarP(
		&presetsListFlags.TypeFilter,
		"type", "t", "",
		"Filter by preset type: infrastructure, installation",
	)

	registerOutputFlags(presetsListCmd, commonFlags)
}
