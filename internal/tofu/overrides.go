// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/zclconf/go-cty/cty"
)

// ParseOverrideStrings converts raw string overrides into cty.Value overrides.
//
// If a variable is present in defaults and has a known primitive type, the value
// is parsed accordingly. Unknown variables are accepted and treated as strings.
func ParseOverrideStrings(
	defaults map[string]*Variable,
	overrides map[string]string,
) (map[string]cty.Value, error) {
	out := make(map[string]cty.Value, len(overrides))
	for key, raw := range overrides {
		val, ok := defaults[key]
		if !ok || val == nil || strings.TrimSpace(val.Type) == "" {
			out[key] = cty.StringVal(raw)
			continue
		}

		switch strings.TrimSpace(val.Type) {
		case "string":
			out[key] = cty.StringVal(raw)
		case "bool":
			s := strings.ToLower(strings.TrimSpace(raw))
			switch s {
			case "true":
				out[key] = cty.BoolVal(true)
			case "false":
				out[key] = cty.BoolVal(false)
			default:
				return nil, fmt.Errorf(
					"invalid bool value for %q: %q (expected true/false)",
					key,
					raw,
				)
			}
		case "number":
			num := new(big.Float)
			if _, ok := num.SetString(strings.TrimSpace(raw)); !ok {
				return nil, fmt.Errorf(
					"invalid number value for %q: %w: %q",
					key,
					ErrInvalidNumberOverride,
					raw,
				)
			}
			out[key] = cty.NumberVal(num)
		default:
			return nil, fmt.Errorf("unsupported variable type %q for %q", val.Type, key)
		}
	}

	return out, nil
}

var ErrInvalidNumberOverride = errors.New("invalid number")
