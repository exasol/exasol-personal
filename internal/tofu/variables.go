// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

type Variable struct {
	Type        string
	Description string
	Value       cty.Value
	Required    bool
	Optional    bool
	Order       int
}

// writeVarsFileWithOverrides writes a tfvars file with defaults and user overrides.
func writeVarsFileWithOverrides(
	tfvarsFullPath string,
	defaults map[string]*Variable,
	overrides map[string]cty.Value,
) (retErr error) {
	// Ensure parent dir exists.
	const directoryPermissions = 0o700
	if err := os.MkdirAll(filepath.Dir(tfvarsFullPath), directoryPermissions); err != nil {
		return err
	}

	// The vars file may contain secrets (e.g. passwords). Keep it user-readable only.
	const varsFilePermissions = 0o600
	varsFile, err := os.OpenFile(
		tfvarsFullPath,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		varsFilePermissions,
	)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := varsFile.Close(); cerr != nil && retErr == nil {
			retErr = fmt.Errorf("failed to close tfvars file %q: %w", tfvarsFullPath, cerr)
		}
	}()

	// Build the output vars set starting from parsed defaults (if any).
	outVars := map[string]*Variable{}
	for name, variable := range defaults {
		if variable == nil {
			continue
		}
		// Shallow copy is fine; we only override Value below.
		outVars[name] = &Variable{
			Type:        variable.Type,
			Description: variable.Description,
			Value:       variable.Value,
		}
	}

	for key, val := range overrides {
		if tv, exists := outVars[key]; exists {
			tv.Value = val
		} else {
			// Allow user-provided vars even if not present in parsed defaults.
			outVars[key] = &Variable{Value: val}
		}
	}

	return WriteVarsFile(outVars, varsFile)
}

func defaultEvalCtx() *hcl.EvalContext {
	return &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"string": cty.StringVal("string"),
			"number": cty.StringVal("number"),
			"bool":   cty.StringVal("bool"),
		},
	}
}

var (
	ErrFailedHclParsing = errors.New("failed to parse hcl")
	ErrFailedHclDecode  = errors.New("failed to decode hcl")
)

// ParseVarFile parses a tofu variables file and returns the variables declared within.
// nolint: godox
// TODO this could be made more lenient.
// Process the var file dynamically instead of trying to decode into structs.
func ParseVarFile(varFile []byte, filenameForLogs string) (map[string]*Variable, error) {
	result := make(map[string]*Variable)

	type VariableBlock struct {
		Name        string `hcl:"name,label"`
		Type        string `hcl:"type"`
		Description string `hcl:"description,optional"`
		Default     any    `hcl:"default,optional"`
		Sensitve    bool   `hcl:"sensitive,optional"`
	}

	type Variables struct {
		Variables []VariableBlock `hcl:"variable,block"`
	}

	vars := Variables{}

	parser := hclparse.NewParser()

	file, diags := parser.ParseHCL(varFile, filenameForLogs)
	if diags.HasErrors() {
		return result, fmt.Errorf("%w: %s", ErrFailedHclParsing, diags.Error())
	}

	evalCtx := defaultEvalCtx()

	diags = gohcl.DecodeBody(file.Body, evalCtx, &vars)

	if diags.HasErrors() {
		return result, fmt.Errorf("%w: %s", ErrFailedHclDecode, diags.Error())
	}

	for idx, variable := range vars.Variables {
		attr, ok := variable.Default.(*hcl.Attribute)
		if !ok {
			continue
		}

		val, diags := attr.Expr.Value(nil)
		if diags.HasErrors() {
			continue
		}

		result[variable.Name] = &Variable{
			Description: variable.Description,
			Type:        variable.Type,
			Value:       val,
			Order:       idx + 1,
		}
	}

	return result, nil
}

// WriteVarsFile writes a tfvars file based on the provided variables map.
func WriteVarsFile(vars map[string]*Variable, writer io.Writer) error {
	file := hclwrite.NewEmptyFile()

	// Get the body of the file
	body := file.Body()

	for k, v := range vars {
		body.SetAttributeValue(k, v.Value)
	}

	_, err := writer.Write(file.Bytes())

	return err
}
