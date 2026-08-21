// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

//go:generate go run ../../tools/resourceembedder

// embeddedResources holds resource data compiled directly into the binary,
// populated via Register by generated files under assets/resources/generated.
// It stays empty in any process that never imports that package, which is
// what keeps the build-time generator's fetches always real.
var embeddedResources = map[string][]byte{}

// embeddedHashes backs each resource's cache identity with its build-time
// content hash instead of a runtime-computed one, so an upgraded binary's
// different embedded content always resolves to its own cache entry.
var embeddedHashes = map[string]string{}

// Register makes data available for resourceID's embedded resolution, along
// with the content hash the generator computed over it at build time.
// Called from generated files' init() functions. A call with empty data is a
// no-op: that's how a platform without a declared artifact for a resource
// stays unregistered, without needing a separate placeholder mechanism.
func Register(resourceID string, data []byte, sha256Hex string) {
	if len(data) == 0 {
		return
	}

	embeddedResources[resourceID] = data
	embeddedHashes[resourceID] = sha256Hex
}

func lookupEmbedded(resourceID string) ([]byte, bool) {
	data, ok := embeddedResources[resourceID]

	return data, ok
}

func lookupEmbeddedHash(resourceID string) (string, bool) {
	hash, ok := embeddedHashes[resourceID]

	return hash, ok
}
