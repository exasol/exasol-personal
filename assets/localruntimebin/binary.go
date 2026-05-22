// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntimebin

import (
	"errors"
	"os"
)

// WriteBinary writes the embedded local runtime binary to the given path.
func WriteBinary(path string) error {
	if !RunnerBinaryAvailable || len(RunnerBinary) == 0 {
		return errors.New("embedded Exasol Local runner is not available")
	}

	const perm = 0o700

	return os.WriteFile(path, RunnerBinary, perm)
}
