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

func Resolve(container Container, sources Sources) (Resolved, error) {
	if err := container.Validate("container"); err != nil {
		return Resolved{}, err
	}
	result := container.Defaults()
	result.Env = nil
	values := make(map[string]string)
	sensitive := []string{}
	for _, env := range container.Env {
		value := ""
		if env.ValueFrom != nil {
			ref := env.ValueFrom.SecretKeyRef
			resolved, available := sources[ref.Name][ref.Key]
			if !available {
				if ref.Optional {
					continue
				}

				return Resolved{}, fmt.Errorf(
					"sidecar %s: required source %s key %s is unavailable",
					container.Name,
					ref.Name,
					ref.Key,
				)
			}
			value = resolved
		} else if env.Value != nil {
			sensitive = append(sensitive, *env.Value)
			value = Expand(*env.Value, values)
		}
		if strings.ContainsRune(value, '\x00') {
			return Resolved{}, fmt.Errorf(
				"sidecar %s: environment %s contains NUL",
				container.Name,
				env.Name,
			)
		}
		values[env.Name] = value
		sensitive = append(sensitive, value)
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

func WithoutDatabasePassword(container Container) Container {
	container = container.Clone()
	container.Env = slices.DeleteFunc(container.Env, func(env EnvVar) bool {
		if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
			return false
		}
		ref := env.ValueFrom.SecretKeyRef

		return ref.Name == DatabaseSource && ref.Key == "password"
	})

	return container
}

//nolint:mnd // The expansion prefix consists of two bytes: $(.
func Expand(input string, values map[string]string) string {
	var output strings.Builder
	for index := 0; index < len(input); index++ {
		if input[index] == '$' && index+1 < len(input) {
			switch input[index+1] {
			case '$':
				_ = output.WriteByte('$')
				index++

				continue
			case '(':
				end := strings.IndexByte(input[index+2:], ')')
				if end >= 0 {
					end += index + 2
					reference := input[index+2 : end]
					if value, exists := values[reference]; exists {
						_, _ = output.WriteString(value)
					} else {
						_, _ = output.WriteString(input[index : end+1])
					}
					index = end

					continue
				}
			default:
			}
		}
		_ = output.WriteByte(input[index])
	}

	return output.String()
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
