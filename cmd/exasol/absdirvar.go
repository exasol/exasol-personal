// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/pflag"
)

// A pflag variable type that is always an absolute and existing directory

type AbsDirValue struct {
	target *string
}

func NewAbsDirValue(target *string, defaultValue string) *AbsDirValue {
	*target, _ = filepath.Abs(defaultValue)
	return &AbsDirValue{target: target}
}

func (v *AbsDirValue) String() string {
	if v.target == nil {
		return ""
	}

	return *v.target
}

func (*AbsDirValue) Type() string { return "file-path" }

func (v *AbsDirValue) Set(s string) error {
	abs, err := filepath.Abs(s)
	if err != nil {
		return fmt.Errorf("abs(%q): %w", s, err)
	}
	abs = filepath.Clean(abs)

	fi, err := os.Stat(abs)
	if err == nil && !fi.IsDir() {
		return fmt.Errorf("%q is not a directory", abs)
	}

	*v.target = abs

	return nil
}

func AbsDirVar(fs *pflag.FlagSet, target *string, name, shorthand, defaultValue, usage string) {
	fs.VarP(NewAbsDirValue(target, defaultValue), name, shorthand, usage)
}
