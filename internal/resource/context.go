// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import "context"

type resolverContextKey struct{}

func NewContext(ctx context.Context, resolver *Resolver) context.Context {
	return context.WithValue(ctx, resolverContextKey{}, resolver)
}

func FromContext(ctx context.Context) *Resolver {
	resolver, ok := ctx.Value(resolverContextKey{}).(*Resolver)
	if !ok {
		panic("resource: no Resolver attached to context")
	}

	return resolver
}
