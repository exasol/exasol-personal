// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/presets"
)

func TestValidateRenderPresetTypeFilter_EmptyFilterIsValid(t *testing.T) {
	t.Parallel()

	if err := validateRenderPresetTypeFilter("", ""); err != nil {
		t.Fatalf("expected no error for an empty filter, got %v", err)
	}
}

func TestValidateRenderPresetTypeFilter_KnownTypeIsValid(t *testing.T) {
	t.Parallel()

	err := validateRenderPresetTypeFilter(
		presets.PresetTypeInfrastructure, presets.PresetTypeInfrastructure,
	)
	if err != nil {
		t.Fatalf("expected no error for a known preset type, got %v", err)
	}
}

func TestValidateRenderPresetTypeFilter_UnknownTypeListsAllowedTypes(t *testing.T) {
	t.Parallel()

	err := validateRenderPresetTypeFilter("bogus", "bogus")
	if err == nil {
		t.Fatal("expected an error for an unknown preset type")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("expected the invalid type to be echoed back, got %v", err)
	}
	if !strings.Contains(err.Error(), presets.PresetTypeInfrastructure) ||
		!strings.Contains(err.Error(), presets.PresetTypeInstallation) {
		t.Fatalf("expected the allowed types to be listed, got %v", err)
	}
}

func TestFilterPresetCatalog_All(t *testing.T) {
	t.Parallel()

	cat := GetPresetCatalog()
	filtered, err := filterPresetCatalog("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(filtered.Infrastructures) != len(cat.Infrastructures) {
		t.Fatalf(
			"unexpected number of infrastructure presets: got %d, expected %d",
			len(filtered.Infrastructures),
			len(cat.Infrastructures),
		)
	}
	if len(filtered.Installations) != len(cat.Installations) {
		t.Fatalf(
			"unexpected number of installation presets: got %d, expected %d",
			len(filtered.Installations),
			len(cat.Installations),
		)
	}

	for _, p := range filtered.Infrastructures {
		if p.ID == "" {
			t.Fatal("expected non-empty infrastructure preset id")
		}
	}
	for _, p := range filtered.Installations {
		if p.ID == "" {
			t.Fatal("expected non-empty installation preset id")
		}
	}
}

func TestFilterPresetCatalog_FilterInfrastructure(t *testing.T) {
	t.Parallel()

	cat := GetPresetCatalog()
	filtered, err := filterPresetCatalog(presets.PresetTypeInfrastructure)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filtered.Installations) != 0 {
		t.Fatalf("unexpected installations in infrastructure-only filter: %d",
			len(filtered.Installations))
	}
	if len(filtered.Infrastructures) != len(cat.Infrastructures) {
		t.Fatalf(
			"unexpected number of infrastructures: got %d, expected %d",
			len(filtered.Infrastructures),
			len(cat.Infrastructures),
		)
	}
}

func TestFilterPresetCatalog_FilterInstallation(t *testing.T) {
	t.Parallel()

	cat := GetPresetCatalog()
	filtered, err := filterPresetCatalog(presets.PresetTypeInstallation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filtered.Infrastructures) != 0 {
		t.Fatalf("unexpected infrastructures in installation-only filter: %d",
			len(filtered.Infrastructures))
	}
	if len(filtered.Installations) != len(cat.Installations) {
		t.Fatalf(
			"unexpected number of installations: got %d, expected %d",
			len(filtered.Installations),
			len(cat.Installations),
		)
	}
}

func TestFilterPresetCatalog_InvalidType(t *testing.T) {
	t.Parallel()

	_, err := filterPresetCatalog("nonsense")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRenderPresetListJSON_ValidJSON(t *testing.T) {
	t.Parallel()

	catalog, err := filterPresetCatalog("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var buf bytes.Buffer
	if err := renderPresetListJSON(&buf, catalog); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded PresetCatalog
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("expected valid json, got error: %v\njson: %s", err, buf.String())
	}
	if len(decoded.Infrastructures) != len(catalog.Infrastructures) {
		t.Fatalf(
			"unexpected decoded infrastructure length: got %d expected %d",
			len(decoded.Infrastructures),
			len(catalog.Infrastructures),
		)
	}
	if len(decoded.Installations) != len(catalog.Installations) {
		t.Fatalf(
			"unexpected decoded installation length: got %d expected %d",
			len(decoded.Installations),
			len(catalog.Installations),
		)
	}
}
