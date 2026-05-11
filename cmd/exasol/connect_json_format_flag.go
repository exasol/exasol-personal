// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"

	"github.com/exasol/exasol-personal/internal/connect"
	"github.com/spf13/pflag"
)

type JSONFormatValue struct {
	target *connect.JSONFormat
}

func NewJSONFormatValue(
	target *connect.JSONFormat,
	defaultValue connect.JSONFormat,
) *JSONFormatValue {
	if target != nil {
		*target = defaultValue
	}

	return &JSONFormatValue{target: target}
}

func (v *JSONFormatValue) String() string {
	if v == nil || v.target == nil {
		return ""
	}

	return v.target.String()
}

func (*JSONFormatValue) Type() string {
	return "string"
}

func (v *JSONFormatValue) Set(input string) error {
	if v == nil || v.target == nil {
		return errors.New("json format target is nil")
	}

	format, err := connect.ParseJSONFormat(input)
	if err != nil {
		return err
	}

	*v.target = format

	return nil
}

func JSONFormatVarP(
	flagSet *pflag.FlagSet,
	target *connect.JSONFormat,
	name string,
	shorthand string,
	defaultValue connect.JSONFormat,
	usage string,
) {
	flagSet.VarP(NewJSONFormatValue(target, defaultValue), name, shorthand, usage)

	flag := flagSet.Lookup(name)
	if flag == nil {
		return
	}

	flag.NoOptDefVal = defaultValue.String()
}
