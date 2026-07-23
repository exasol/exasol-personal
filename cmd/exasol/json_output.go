// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

func addJSONTerminalOutput(payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	addTerminalOutput(string(data))

	return nil
}

func addRenderedTerminalOutput(render func(io.Writer) error) error {
	var output bytes.Buffer
	if err := render(&output); err != nil {
		return err
	}
	addTerminalOutput(strings.TrimRight(output.String(), "\n"))

	return nil
}
