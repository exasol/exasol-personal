// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// UserConfirmationValidator is a function that takes an user input as a String,
// and returns if its a possitive confirmation from the user.
type UserConfirmationValidator func(input string) bool

// confirmYes is an implementation of the UserConfirmationValidator type that simply returns true
// if the user answered y or yes.
func confirmYes(input string) bool {
	input = strings.ToLower(strings.TrimSpace(input))
	return input == "y" || input == "yes"
}

// Print without linter complaints.
func safePrint(str string) {
	fmt.Print(str) // nolint:revive, forbidigo
}

// askForUserConfirmation asks the user for confirmation via stdout, and uses validators
// (Currently only accounting the first one on the variadic) to
// check if the confirmation is positive. If prompt is an empty string, a default prompt
// is emitted.
func askForUserConfirmation(prompt string, validators ...UserConfirmationValidator) bool {
	validate := confirmYes
	if len(validators) > 0 {
		validate = validators[0]
	}
	if prompt == "" {
		prompt = "Continue? [y/N]"
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s: ", prompt) //nolint

	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.ToLower(strings.TrimSpace(response))

	return validate(response)
}

// ctyToJSONString renders a cty.Value to JSON string, for nice defaults in help.
func ctyToJSONString(value cty.Value) string {
	repr, err := ctyjson.Marshal(value, value.Type())
	if err != nil {
		return "<error>"
	}

	return string(repr)
}

// Note: orderedInfrastructureVariables and the custom help wrapper were removed when
// infrastructure variables became regular Cobra flags.

func looksLikePathPresetArg(arg string) bool {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return false
	}
	// We treat anything path-like as a filesystem path to avoid ambiguity when a folder
	// named like an embedded preset happens to exist in the current working directory.
	//
	// Users can always force "name" selection by passing a plain identifier like "aws".
	return strings.HasPrefix(arg, ".") || strings.HasPrefix(arg, "~") ||
		strings.ContainsAny(arg, `/\\`)
}

func presetRefFromArg(arg string) deploy.PresetRef {
	arg = strings.TrimSpace(arg)
	if looksLikePathPresetArg(arg) {
		return deploy.PresetRef{Path: arg}
	}

	return deploy.PresetRef{Name: arg}
}

func defaultInstallationPresetRef() deploy.PresetRef {
	return deploy.PresetRef{Name: presets.DefaultInstallation}
}

func defaultedPresetRefFromOptionalArg(
	args []string,
	index int,
	def deploy.PresetRef,
) deploy.PresetRef {
	if index < 0 || index >= len(args) {
		return def
	}
	if strings.TrimSpace(args[index]) == "" {
		return def
	}

	return presetRefFromArg(args[index])
}

func presetNamesForHelp(listname string, names []string) string {
	namesList := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		namesList = append(namesList, name)
	}
	sort.Strings(namesList)
	if len(namesList) == 0 {
		return "(none)"
	}

	return "Available " + listname + " presets: " + strings.Join(namesList, ", ")
}
