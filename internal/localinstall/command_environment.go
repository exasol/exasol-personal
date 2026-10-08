// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"
)

var shellEnvironmentName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func CommandEnvironment(
	env map[string]string, input io.Reader, command []string,
) ([]string, io.Reader, error) {
	if len(env) == 0 {
		return command, input, nil
	}
	var setup strings.Builder
	for _, name := range slices.Sorted(maps.Keys(env)) {
		value := env[name]
		if !shellEnvironmentName.MatchString(name) || strings.ContainsRune(value, '\x00') {
			return nil, nil, errors.New("invalid command environment")
		}
		_, _ = fmt.Fprintf(
			&setup,
			"export %s='%s'\n",
			name,
			strings.ReplaceAll(value, "'", "'\"'\"'"),
		)
	}
	if input == nil {
		input = strings.NewReader("")
	}
	// Read exactly the setup bytes so the child retains its original stdin.
	script := fmt.Sprintf(`eval "$(dd bs=1 count=%d 2>/dev/null)" && exec "$@"`, setup.Len())
	wrapped := append([]string{"sh", "-c", script, "sh"}, command...)

	return wrapped, io.MultiReader(strings.NewReader(setup.String()), input), nil
}
