// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
)

var minimumPodmanVersion = semver.MustParse("5.8.0")

func readPodmanVersions(
	ctx context.Context,
	commands CommandRunner,
) (podmanVersions, error) {
	read := func(template, component string) (semver.Version, error) {
		data, err := commands.Output(ctx, "podman", "version", "--format", template)
		if err != nil {
			return semver.Version{}, fmt.Errorf(
				"failed to read Podman %s version: %w",
				component,
				err,
			)
		}
		version, err := semver.ParseTolerant(strings.TrimSpace(string(data)))
		if err != nil {
			return semver.Version{}, fmt.Errorf(
				"failed to parse Podman %s version %q: %w",
				component,
				data,
				err,
			)
		}

		return version, nil
	}
	clientVersion, err := read("{{.Client.Version}}", "client")
	if err != nil {
		return podmanVersions{}, err
	}
	serverVersion, err := read("{{.Server.Version}}", "server")
	if err != nil {
		return podmanVersions{}, err
	}

	return podmanVersions{Client: clientVersion, Server: serverVersion}, nil
}

type podmanVersions struct {
	Client semver.Version
	Server semver.Version
}
