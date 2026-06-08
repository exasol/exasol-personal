// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"

	"github.com/exasol/exasol-personal/internal/tofu"
)

func main() {
	flag.Parse()

	binaryPath, err := tofu.ResolveBinaryPath(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	cmd := exec.CommandContext(context.Background(), binaryPath, flag.Args()...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}
