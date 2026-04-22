// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build !darwin || !arm64

package vm

import "context"

type driver struct{}

func New() Driver {
	return driver{}
}

func (driver) Start(context.Context, MachineConfig) error {
	return ErrUnsupportedPlatform
}

func (driver) Stop(context.Context) error {
	return ErrUnsupportedPlatform
}

func (driver) Wait(context.Context) error {
	return ErrUnsupportedPlatform
}

func (driver) Running() bool {
	return false
}
