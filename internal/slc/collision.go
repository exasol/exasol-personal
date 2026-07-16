// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package slc

import (
	"fmt"
	"strings"
)

// CheckInstallable determines whether a candidate SLC can be installed given the set of
// already-installed SLCs.
//
// The database refuses to start if two mounted SLCs declare the same alias (it throws at
// engine init), so the installed set must stay disjoint across all declared aliases.
//
// It returns replacesFlavor=true when the candidate is a (new version of an)
// already-installed flavor; the caller should then replace that entry rather than add a
// second one. It returns an error when the candidate shares any alias with a *different*
// installed flavor.
//
//nolint:nonamedreturns // named returns document the replace-vs-add / collision result.
func CheckInstallable(installed []Entry, candidate Entry) (replacesFlavor bool, err error) {
	for _, existing := range installed {
		if strings.EqualFold(existing.Flavor, candidate.Flavor) {
			replacesFlavor = true

			continue
		}

		if shared := sharedAlias(existing.Aliases, candidate.Aliases); shared != "" {
			return false, fmt.Errorf(
				"cannot install %s: alias %q is already provided by installed SLC %s",
				candidate.Flavor, shared, existing.Flavor,
			)
		}
	}

	return replacesFlavor, nil
}

func sharedAlias(a, b []string) string {
	for _, x := range a {
		for _, y := range b {
			if strings.EqualFold(x, y) {
				return strings.ToUpper(x)
			}
		}
	}

	return ""
}
