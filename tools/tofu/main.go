// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

func main() {
	flag.Parse()

	spec, err := runtimeartifacts.ParseSpec(resources.ResourcesYAML)
	if err != nil {
		log.Fatal(err)
	}
	manager, err := runtimeartifacts.NewResourceManager(spec)
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
