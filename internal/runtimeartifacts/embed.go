// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

//go:generate go run ../../tools/resourceembedder

// embeddedResources holds resource data compiled directly into the binary,
// populated via Register by generated files under assets/resources/generated.
// It stays empty in any process that never imports that package, which is
// what keeps the build-time generator's fetches always real.
var embeddedResources = map[string][]byte{}

// Register makes data available for resourceID's embedded resolution. Called
// from generated files' init() functions. A call with empty data is a no-op:
// that's how a platform without a declared artifact for a resource stays
// unregistered, without needing a separate placeholder mechanism.
func Register(resourceID string, data []byte) {
	if len(data) == 0 {
		return
	}

	embeddedResources[resourceID] = data
}

func lookupEmbedded(resourceID string) ([]byte, bool) {
	data, ok := embeddedResources[resourceID]

	return data, ok
}
