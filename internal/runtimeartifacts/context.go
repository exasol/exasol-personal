// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import "context"

type managerContextKey struct{}

// NewContext returns a copy of ctx carrying manager as the process's shared
// resource manager, retrievable via FromContext.
func NewContext(ctx context.Context, manager *Manager) context.Context {
	return context.WithValue(ctx, managerContextKey{}, manager)
}

// FromContext returns the resource manager attached to ctx by NewContext. It
// panics if none is attached: every real code path runs under a context
// Execute() (or a test) has already populated.
func FromContext(ctx context.Context) *Manager {
	manager, ok := ctx.Value(managerContextKey{}).(*Manager)
	if !ok {
		panic("runtimeartifacts: no Manager attached to context")
	}

	return manager
}
