// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"

	"github.com/exasol/exasol-personal/internal/util"
)

func main() {
	util.StartSignalHandler(func(_ os.Signal) {
		os.Exit(1)
	})

	err := Execute()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
