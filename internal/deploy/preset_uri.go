// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"net/url"
	"strings"
)

// parsePresetURI splits an optional trailing "#<subpath>" fragment off a
// preset source URI. The returned cleanURI has the fragment removed; subpath
// is the percent-decoded subdirectory selector (empty when no fragment was
// present).
//
// The fragment is parsed as the last "#" in the URI so callers do not need to
// know which scheme the URI uses. cleanURI is then passed through unchanged to
// the source classifiers, so ref suffix and scheme detection are unaffected.
func parsePresetURI(uri string) (cleanURI, subpath string) { //nolint:nonamedreturns
	idx := strings.LastIndex(uri, "#")
	if idx < 0 {
		return uri, ""
	}

	cleanURI = uri[:idx]
	rawSubpath := uri[idx+1:]

	// Percent-decode so encoded characters (%2F, %23) work. On decoding
	// failure return the raw string so the caller can produce a meaningful
	// validation error downstream.
	decoded, err := url.PathUnescape(rawSubpath)
	if err != nil {
		return cleanURI, rawSubpath
	}

	return cleanURI, decoded
}
