// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"regexp"

	"github.com/spf13/pflag"
)

// A pflag variable type that only accepts strings safe to use as a literal
// deployment directory name (see internal/config.NamedDeploymentDirPath).

var deploymentNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type DeploymentNameValue struct {
	target *string
}

func NewDeploymentNameValue(target *string) *DeploymentNameValue {
	*target = ""

	return &DeploymentNameValue{target: target}
}

func (v *DeploymentNameValue) String() string {
	if v.target == nil {
		return ""
	}

	return *v.target
}

func (*DeploymentNameValue) Type() string { return "string" }

func (v *DeploymentNameValue) Set(value string) error {
	if !deploymentNamePattern.MatchString(value) {
		return fmt.Errorf(
			"invalid deployment name %q: only letters, digits, '-', and '_' are allowed",
			value,
		)
	}

	*v.target = value

	return nil
}

func DeploymentNameVar(fs *pflag.FlagSet, target *string, name, shorthand, usage string) {
	fs.VarP(NewDeploymentNameValue(target), name, shorthand, usage)
}
