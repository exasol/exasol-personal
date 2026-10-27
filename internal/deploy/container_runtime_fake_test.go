// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

// Answering the runtime's commands lets these tests drive the real driver,
// so its labels, fingerprints and matching are the ones under test.
type containerRuntimeFake struct {
	mutex      sync.Mutex
	containers map[string]*fakeContainer
	networks   map[string]bool
	// failures maps a subcommand ("run", "rm", "ps") to the error it returns.
	failures map[string]error
	runCalls int
	removed  []string
	database string
	// Read while the driver's environment file still exists.
	lastEnv string
	// A database that predates the per-deployment service network.
	databaseOffNetwork bool
	// Containers created from now on report having started this long ago.
	age   time.Duration
	execs [][]string
}

type fakeContainer struct {
	name    string
	id      string
	labels  map[string]string
	ports   []fakePort
	running bool
	env     string
	started time.Time
}

func newContainerRuntimeFake(database string) *containerRuntimeFake {
	return &containerRuntimeFake{
		containers: map[string]*fakeContainer{
			database: {name: database, id: database, running: true, labels: map[string]string{}},
		},
		networks: map[string]bool{},
		failures: map[string]error{},
		database: database,
	}
}

func (fake *containerRuntimeFake) Run(
	_ context.Context, _ io.Reader, stdout, _ io.Writer, command ...string,
) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	args := command[1:]
	// Record the attempt before any injected failure, because the tests ask
	// what the driver tried to do, not what the runtime let it finish.
	switch args[0] {
	case "run":
		fake.runCalls++
		if err := fake.captureEnvironment(args); err != nil {
			return err
		}
	case "rm":
		fake.removed = append(fake.removed, args[len(args)-1])
	case "exec":
		fake.execs = append(fake.execs, slices.Clone(args[1:]))
	default:
	}
	if err := fake.failures[args[0]]; err != nil {
		return err
	}
	switch args[0] {
	case "ps":
		return fake.list(stdout, args)
	case "run":
		return fake.create(args)
	case "rm":
		return fake.remove(args)
	case "network":
		return fake.network(stdout, args)
	case "container":
		return fake.inspect(stdout, args)
	case "exec":
		return nil
	default:
		return fmt.Errorf("unexpected container command: %q", command)
	}
}

func (fake *containerRuntimeFake) LeaveServiceNetwork() {
	// Such a database needs an explicit restart before sidecars reach it.
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.databaseOffNetwork = true
}

// Sidecars reports the containers the driver created, keyed by sidecar name.
func (fake *containerRuntimeFake) Sidecars() map[string]*fakeContainer {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	sidecars := map[string]*fakeContainer{}
	for _, container := range fake.containers {
		if name := container.labels["com.exasol.launcher.sidecar-name"]; name != "" {
			sidecars[name] = container
		}
	}

	return sidecars
}

// Add registers a sidecar container the deployment did not ask for.
func (fake *containerRuntimeFake) Add(name string) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	container := fake.database + "-sidecar-" + name
	fake.containers[container] = &fakeContainer{
		name: container, id: container, running: true,
		labels: map[string]string{
			"com.exasol.launcher.sidecar-owner": fake.database,
			"com.exasol.launcher.sidecar-name":  name,
		},
	}
}

// Duplicate registers a second container reporting the same sidecar.
func (fake *containerRuntimeFake) Duplicate(name string) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	original := fake.containers[fake.database+"-sidecar-"+name]
	copied := *original
	copied.name += "-duplicate"
	copied.id += "-duplicate"
	copied.labels = maps.Clone(original.labels)
	fake.containers[copied.name] = &copied
}

// Clear removes every sidecar container, leaving the database alone.
func (fake *containerRuntimeFake) Clear() {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	for name, container := range fake.containers {
		if container.labels["com.exasol.launcher.sidecar-name"] != "" {
			delete(fake.containers, name)
		}
	}
}

// LastEnvironment reports what the most recent creation attempt carried.
func (fake *containerRuntimeFake) LastEnvironment() string {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	return fake.lastEnv
}

// Execs reports the commands run inside containers, each led by its container.
func (fake *containerRuntimeFake) Execs() [][]string {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	return slices.Clone(fake.execs)
}

func (fake *containerRuntimeFake) Fail(subcommand string, err error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.failures[subcommand] = err
}

func (fake *containerRuntimeFake) Calls() (int, []string) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	return fake.runCalls, slices.Clone(fake.removed)
}

// Stop marks every sidecar container as no longer running, as a host that went
// away would leave them.
func (fake *containerRuntimeFake) Stop() {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	for _, container := range fake.containers {
		container.running = false
	}
}

func (container *fakeContainer) matches(filters []string) bool {
	for _, filter := range filters {
		key, value, _ := strings.Cut(filter, "=")
		switch key {
		case "label":
			label, want, _ := strings.Cut(value, "=")
			if container.labels[label] != want {
				return false
			}
		case "name":
			if !strings.Contains(container.name, strings.Trim(value, "^$")) {
				return false
			}
		default:
		}
	}

	return true
}

func flagValue(args []string, flag string) string {
	if index := slices.Index(args, flag); index >= 0 && index+1 < len(args) {
		return args[index+1]
	}

	return ""
}

func flagValues(args []string, flag string) []string {
	var values []string
	for index, arg := range args {
		if arg == flag && index+1 < len(args) {
			values = append(values, args[index+1])
		}
	}

	return values
}

// parseFakePublication reads the host:container/protocol form the encoder emits.
// fakePort carries the field names the container runtime reports, because the
// fake stands in for the runtime rather than for the launcher's view of it.
//
//nolint:tagliatelle // Mirrors the runtime's own schema.
type fakePort struct {
	HostIP        string `json:"host_ip"`
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

func parseFakePublication(publication string) (fakePort, error) {
	mapping, protocol, _ := strings.Cut(publication, "/")
	parts := strings.Split(mapping, ":")
	if len(parts) < 2 {
		return fakePort{}, fmt.Errorf("unexpected publication %q", publication)
	}
	port := fakePort{Protocol: protocol}
	if len(parts) > 2 {
		port.HostIP = strings.Join(parts[:len(parts)-2], ":")
	}
	if _, err := fmt.Sscanf(parts[len(parts)-2], "%d", &port.HostPort); err != nil {
		return fakePort{}, err
	}
	if _, err := fmt.Sscanf(parts[len(parts)-1], "%d", &port.ContainerPort); err != nil {
		return fakePort{}, err
	}

	return port, nil
}

func (fake *containerRuntimeFake) list(stdout io.Writer, args []string) error {
	filters := flagValues(args, "--filter")
	listings := []map[string]any{}
	for _, name := range slices.Sorted(maps.Keys(fake.containers)) {
		container := fake.containers[name]
		if !container.matches(filters) {
			continue
		}
		state := "exited"
		if container.running {
			state = "running"
		}
		listings = append(listings, map[string]any{
			"id": container.id, "names": []string{name}, "labels": container.labels,
			"state": state, "ports": container.ports,
			"startedAt": container.started.Unix(),
		})
	}

	return json.NewEncoder(stdout).Encode(listings)
}

func (fake *containerRuntimeFake) create(args []string) error {
	name := flagValue(args, "--name")
	container := &fakeContainer{
		name: name, id: name, labels: map[string]string{}, running: true, env: fake.lastEnv,
		started: time.Now().Add(-fake.age),
	}
	for _, label := range flagValues(args, "--label") {
		key, value, _ := strings.Cut(label, "=")
		container.labels[key] = value
	}
	for _, publication := range flagValues(args, "--publish") {
		port, err := parseFakePublication(publication)
		if err != nil {
			return err
		}
		container.ports = append(container.ports, port)
	}
	fake.containers[name] = container

	return nil
}

// captureEnvironment reads the environment file the driver removes once the
// container is created.
func (fake *containerRuntimeFake) captureEnvironment(args []string) error {
	fake.lastEnv = ""
	envFile := flagValue(args, "--env-file")
	if envFile == "" {
		return nil
	}
	content, err := os.ReadFile(envFile)
	if err != nil {
		return err
	}
	fake.lastEnv = string(content)

	return nil
}

func (fake *containerRuntimeFake) remove(args []string) error {
	delete(fake.containers, args[len(args)-1])

	return nil
}

func (fake *containerRuntimeFake) network(stdout io.Writer, args []string) error {
	switch args[1] {
	case "create":
		fake.networks[args[len(args)-1]] = true

		return nil
	case "inspect", "ls":
		networks := []map[string]any{}
		for _, name := range slices.Sorted(maps.Keys(fake.networks)) {
			networks = append(networks, map[string]any{
				"name": name, "driver": "bridge", "dns_enabled": true,
				"labels": map[string]string{
					"com.exasol.launcher.sidecar-owner": fake.database,
				},
			})
		}

		return json.NewEncoder(stdout).Encode(networks)
	case "connect", "rm":
		return nil
	default:
		return fmt.Errorf("unexpected network command: %q", args)
	}
}

// inspect answers the database container's network attachment.
func (fake *containerRuntimeFake) inspect(stdout io.Writer, _ []string) error {
	if fake.databaseOffNetwork {
		return json.NewEncoder(stdout).Encode([]map[string]any{{
			"hostConfig":      map[string]string{"networkMode": "pasta"},
			"networkSettings": map[string]any{"networks": map[string]any{}},
		}})
	}

	return json.NewEncoder(stdout).Encode([]map[string]any{{
		"hostConfig": map[string]string{"networkMode": "bridge"},
		"networkSettings": map[string]any{"networks": map[string]any{
			fake.database + "-services": map[string]any{"aliases": []string{"database"}},
		}},
	}})
}
