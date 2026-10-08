// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar

import (
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	"github.com/distribution/reference"
)

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func (container Container) Validate(path string) error {
	if len(container.Name) > 63 || !dnsLabel.MatchString(container.Name) ||
		container.Name == "database" {
		return fmt.Errorf("%s.name: expected a DNS label other than database", path)
	}
	if _, err := reference.ParseNormalizedNamed(container.Image); err != nil {
		return fmt.Errorf("%s.image: invalid container image reference", path)
	}
	if !slices.Contains([]string{"", Always, IfNotPresent, Never}, container.ImagePullPolicy) {
		return fmt.Errorf("%s.imagePullPolicy: expected Always, IfNotPresent, or Never", path)
	}
	if !slices.Contains([]string{"", Always, OnFailure, Never}, container.RestartPolicy) {
		return fmt.Errorf("%s.restartPolicy: expected Always, OnFailure, or Never", path)
	}
	if strings.ContainsRune(container.WorkingDir, '\x00') {
		return fmt.Errorf("%s.workingDir: contains a NUL character", path)
	}
	for _, field := range []struct {
		name   string
		values []string
	}{{"command", container.Command}, {"args", container.Args}} {
		for index, value := range field.values {
			if strings.ContainsRune(value, '\x00') {
				return fmt.Errorf("%s.%s[%d]: contains a NUL character", path, field.name, index)
			}
		}
	}
	if err := validateEnv(container.Env, path+".env"); err != nil {
		return err
	}
	names := make(map[string]bool)
	for index, port := range container.Ports {
		location := fmt.Sprintf("%s.ports[%d]", path, index)
		if port.Name != "" {
			if len(port.Name) > 15 || !dnsLabel.MatchString(port.Name) || names[port.Name] {
				return fmt.Errorf(
					"%s.name: expected a unique port DNS label of at most 15 characters",
					location,
				)
			}
			names[port.Name] = true
		}
		if err := port.Validate(location); err != nil {
			return err
		}
	}

	return nil
}

func validateEnv(environment []EnvVar, path string) error {
	names := make(map[string]bool)
	for index, env := range environment {
		location := fmt.Sprintf("%s[%d]", path, index)
		if env.Name == "" || strings.ContainsAny(env.Name, "=\x00") || names[env.Name] {
			return fmt.Errorf(
				"%s.name: expected a unique environment name without '=' or NUL",
				location,
			)
		}
		names[env.Name] = true
		if env.Value != nil && strings.ContainsRune(*env.Value, '\x00') {
			return fmt.Errorf("%s.value: contains a NUL character", location)
		}
		if env.ValueFrom == nil {
			continue
		}
		if env.Value != nil {
			return fmt.Errorf("%s: value and valueFrom are mutually exclusive", location)
		}
		ref := env.ValueFrom.SecretKeyRef
		if ref == nil || ref.Name == "" || ref.Key == "" {
			return fmt.Errorf("%s.valueFrom.secretKeyRef: name and key are required", location)
		}
	}

	return nil
}

func (port Port) Validate(path string) error {
	if port.ContainerPort < 1 || port.ContainerPort > 65535 {
		return fmt.Errorf("%s.containerPort: expected 1 through 65535", path)
	}
	if port.HostPort < 0 || port.HostPort > 65535 {
		return fmt.Errorf("%s.hostPort: expected 0 through 65535", path)
	}
	if port.Protocol != "" && port.Protocol != "TCP" {
		return fmt.Errorf("%s.protocol: only TCP is supported", path)
	}
	if port.HostIP != "" {
		address, err := netip.ParseAddr(port.HostIP)
		if err != nil || !address.IsLoopback() {
			return fmt.Errorf("%s.hostIP: local sidecars support only loopback publication", path)
		}
	}

	return nil
}
