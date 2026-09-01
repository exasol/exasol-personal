// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"net/url"
	"strings"
)

type Locator struct {
	URL string
	Ref string
}

func (l Locator) Scheme() string {
	idx := strings.Index(l.URL, "://")
	if idx < 0 {
		return ""
	}

	return l.URL[:idx]
}

func (l Locator) String() string {
	if strings.TrimSpace(l.Ref) == "" {
		return l.URL
	}

	return l.URL + "@" + l.Ref
}

type Descriptor struct {
	Locator      Locator
	Sha256       string
	Extract      bool
	Subpath      string
	DownloadPath string
}

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

// The first @ in an SCP-style Git URL belongs to the user name.
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
