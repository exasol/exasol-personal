// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package task_runner

import (
	"bytes"
	"text/template"
)

func commandSubstitutions(input string, substitutions map[string]string) (string, error) {
	tmpl, err := template.New("").Parse(input)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer

	err = tmpl.Execute(&buf, substitutions)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
