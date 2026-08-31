// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"net/url"
	"strings"
)

// Locator addresses content a Source can fetch. Revision selection is split out
// of the URL here so no source has to parse a suffix out of it.
type Locator struct {
	// URL is the source location with any Git "@ref" and "#subpath" suffix
	// removed.
	URL string
	// Ref selects a revision within the source, such as a branch, tag, or
	// commit for a git repository.
	Ref string
}

// Scheme returns the locator's URL scheme, empty for a bare local path or an
// SCP-style git URL, neither of which carries one.
func (l Locator) Scheme() string {
	idx := strings.Index(l.URL, "://")
	if idx < 0 {
		return ""
	}

	return l.URL[:idx]
}

// String renders the locator as a single URL, reattaching the ref, for a
// source that takes one string rather than a Locator.
func (l Locator) String() string {
	if strings.TrimSpace(l.Ref) == "" {
		return l.URL
	}

	return l.URL + "@" + l.Ref
}

// Descriptor is one fully resolved resource: where its content comes from, what
// it should hash to, and how to present it.
type Descriptor struct {
	Locator Locator
	Sha256  string
	Extract bool
	Subpath string
}

// ParseURI splits the "url@ref#subpath" command-line shorthand into a
// Descriptor. Both suffixes are optional. A specification declaring the same
// url, ref, and subpath as separate fields yields an equivalent Descriptor.
func ParseURI(uri string) Descriptor {
	rest, subpath := splitSubpath(uri)
	rawURL, ref := splitGitRef(rest)

	return Descriptor{
		Locator: Locator{URL: rawURL, Ref: ref},
		Subpath: subpath,
	}
}

func splitGitRef(rawURL string) (location, ref string) { //nolint:nonamedreturns
	location, ref = splitRef(rawURL)
	if ref == "" || (!IsGitSourceURL(location) && !isLocalGitWorktree(location)) {
		return rawURL, ""
	}

	return location, ref
}

// splitSubpath removes a "#subpath" suffix, percent-decoding it so encoded
// characters such as %2F and %23 survive. A subpath that fails to decode is
// returned raw, leaving the caller to report it.
func splitSubpath(uri string) (rest, subpath string) { //nolint:nonamedreturns
	idx := strings.LastIndex(uri, "#")
	if idx < 0 {
		return uri, ""
	}

	rawSubpath := uri[idx+1:]
	decoded, err := url.PathUnescape(rawSubpath)
	if err != nil {
		return uri[:idx], rawSubpath
	}

	return uri[:idx], decoded
}

// splitRef removes an "@ref" suffix. In a git SCP URL (git@host:path) the first
// @ belongs to the scheme, so a ref separator exists only when a colon appears
// before the last @.
func splitRef(rawURL string) (location, ref string) { //nolint:nonamedreturns
	atIdx := strings.LastIndex(rawURL, "@")
	if atIdx < 0 {
		return rawURL, ""
	}
	if strings.HasPrefix(rawURL, "git@") && !strings.Contains(rawURL[:atIdx], ":") {
		return rawURL, ""
	}
	// A container image reference names its content with a digest after an @,
	// which belongs to the reference rather than being a ref suffix.
	if strings.HasPrefix(rawURL[atIdx+1:], "sha256:") {
		return rawURL, ""
	}

	return rawURL[:atIdx], rawURL[atIdx+1:]
}
