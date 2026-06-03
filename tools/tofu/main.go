// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/exasol/exasol-personal/internal/tofu"
)

func main() {
	cacheDir := flag.String("cache-dir", filepath.Join(os.TempDir(), "exasol-personal-runtime"), "Directory used to cache runtime resources")
	flag.Parse()

	binaryPath, err := tofu.ResolveBinaryPath(context.Background(), *cacheDir)
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
