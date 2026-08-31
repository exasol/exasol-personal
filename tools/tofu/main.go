// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"

	"github.com/exasol/exasol-personal/assets/resourcedata"
	"github.com/exasol/exasol-personal/internal/resource"
)

func main() {
	flag.Parse()

	manager, err := resource.NewResourceManagerWithSpec(resourcedata.ResourcesYAML)
	if err != nil {
		log.Fatal(err)
	}
	binaryPath, err := manager.Request(context.Background(), "tofu")
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
