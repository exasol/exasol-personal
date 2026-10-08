// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const DatabaseSource = "exasol-database"

type Sources map[string]map[string]string

type Resolved struct {
	Container Container
	redactor  *strings.Replacer
}

// The launcher prints the other connection values itself.
func secretReference(ref *SecretKeyRef) bool {
	return ref.Name == DatabaseSource && ref.Key == "password"
}

func Resolve(container Container, sources Sources) (Resolved, error) {
	if err := container.Validate("container"); err != nil {
		return Resolved{}, err
	}
	result := container.Defaults()
	result.Env = nil
	values := make(map[string]string)
	sensitive := []string{}
	for _, env := range container.Env {
		value, present, err := resolveEnv(container.Name, env, sources, values)
		if err != nil {
			return Resolved{}, err
		}
		if !present {
			continue
		}
		if strings.ContainsRune(value, '\x00') {
			return Resolved{}, fmt.Errorf(
				"sidecar %s: environment %s contains NUL",
				container.Name,
				env.Name,
			)
		}
		if env.ValueFrom != nil && secretReference(env.ValueFrom.SecretKeyRef) {
			sensitive = append(sensitive, value)
		}
		values[env.Name] = value
		result.Env = append(result.Env, EnvVar{Name: env.Name, Value: &value})
	}
	for index, value := range result.Command {
		result.Command[index] = Expand(value, values)
	}
	for index, value := range result.Args {
		result.Args[index] = Expand(value, values)
	}

	return Resolved{Container: result, redactor: newRedactor(sensitive)}, nil
}

// An absent optional source drops the variable instead of passing an empty one.
func resolveEnv(
	name string,
	env EnvVar,
	sources Sources,
	values map[string]string,
) (string, bool, error) {
	if env.ValueFrom == nil {
		if env.Value == nil {
			return "", true, nil
		}

		return Expand(*env.Value, values), true, nil
	}
	ref := env.ValueFrom.SecretKeyRef
	resolved, available := sources[ref.Name][ref.Key]
	if available {
		return resolved, true, nil
	}
	if ref.Optional {
		return "", false, nil
	}

	return "", false, fmt.Errorf(
		"sidecar %s: required source %s key %s is unavailable",
		name,
		ref.Name,
		ref.Key,
	)
}

func Expand(input string, values map[string]string) string {
	var output strings.Builder
	for index := 0; index < len(input); index++ {
		if input[index] != '$' || index+1 >= len(input) {
			_ = output.WriteByte(input[index])

			continue
		}
		if input[index+1] == '$' {
			_ = output.WriteByte('$')
			index++

			continue
		}
		expansion, end := expandReference(input, index, values)
		if end < 0 {
			_ = output.WriteByte(input[index])

			continue
		}
		_, _ = output.WriteString(expansion)
		index = end
	}

	return output.String()
}

// A negative end reports that no complete reference starts at index.
func expandReference(input string, index int, values map[string]string) (string, int) {
	const prefix = "$("
	if !strings.HasPrefix(input[index:], prefix) {
		return "", -1
	}
	start := index + len(prefix)
	end := strings.IndexByte(input[start:], ')')
	if end < 0 {
		return "", -1
	}
	end += start
	if value, exists := values[input[start:end]]; exists {
		return value, end
	}

	return input[index : end+1], end
}

func newRedactor(values []string) *strings.Replacer {
	var variants []string
	for _, value := range values {
		if value == "" {
			continue
		}
		variants = append(variants, value)
		quoted := strconv.Quote(value)
		variants = append(variants, quoted[1:len(quoted)-1])
		variants = append(variants, strings.ReplaceAll(value, "'", "'\"'\"'"))
		variants = append(variants, strings.ReplaceAll(value, "'", "''"))
	}
	slices.SortFunc(
		variants,
		func(left, right string) int { return cmp.Compare(len(right), len(left)) },
	)
	const pairWidth = 2
	pairs := make([]string, 0, len(variants)*pairWidth)
	for _, value := range slices.Compact(variants) {
		pairs = append(pairs, value, "[redacted]")
	}

	return strings.NewReplacer(pairs...)
}

func (resolved Resolved) Redact(message string) string {
	if resolved.redactor == nil {
		return message
	}

	return resolved.redactor.Replace(message)
}
