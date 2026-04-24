// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"fmt"
	"os"

	localconfig "github.com/exasol/exasol-personal/internal/localruntime/config"
	localstate "github.com/exasol/exasol-personal/internal/localruntime/state"
)

type Runtime struct {
	layout       localconfig.Layout
	store        localstate.Store
	allocatePort func(excluded map[int]struct{}) (int, error)
}

type ConnectionPorts struct {
	DB int
	UI int
}

const (
	dbPortName = "db"
	uiPortName = "ui"
)

func New(deploymentDir string) *Runtime {
	layout := localconfig.NewLayout(deploymentDir)

	return &Runtime{
		layout:       layout,
		store:        localstate.NewStore(layout.StateFile()),
		allocatePort: localconfig.AllocatePort,
	}
}

func (r *Runtime) Layout() localconfig.Layout {
	return r.layout
}

func (r *Runtime) EnsureRoot() error {
	for _, dir := range []string{
		r.layout.RuntimeRoot(),
		r.layout.ConfigDir(),
		r.layout.BootstrapDir(),
		r.layout.ControlDir(),
		r.layout.DataDir(),
		r.layout.LogsDir(),
		r.layout.VMDir(),
		r.layout.PayloadDir(),
		r.layout.PayloadBootDir(),
		r.layout.PayloadShareDir(),
	} {
		if err := os.MkdirAll(dir, localRuntimeDirMode); err != nil {
			return fmt.Errorf("failed to create local runtime dir %q: %w", dir, err)
		}
	}

	return nil
}

func (r *Runtime) LoadState() (*localstate.State, error) {
	return r.store.Read()
}

func (r *Runtime) SaveState(state *localstate.State) error {
	return r.store.Write(state)
}

func (r *Runtime) EnsureConnectionPorts() (*ConnectionPorts, error) {
	if err := r.EnsureRoot(); err != nil {
		return nil, err
	}

	state, err := r.LoadState()
	if err != nil {
		return nil, err
	}
	changed := false

	if state.Ports == nil {
		state.Ports = make(map[string]int)
		changed = true
	}
	for _, name := range []string{dbPortName, uiPortName} {
		if state.Ports[name] > 0 {
			continue
		}

		port, err := r.allocatePort(excludedPorts(state.Ports))
		if err != nil {
			return nil, err
		}

		state.Ports[name] = port
		changed = true
	}

	if changed {
		if err := r.SaveState(state); err != nil {
			return nil, err
		}
	}

	return &ConnectionPorts{
		DB: state.Ports[dbPortName],
		UI: state.Ports[uiPortName],
	}, nil
}

func excludedPorts(ports map[string]int) map[int]struct{} {
	excluded := make(map[int]struct{}, len(ports))
	for _, port := range ports {
		if port <= 0 {
			continue
		}

		excluded[port] = struct{}{}
	}

	return excluded
}

func (r *Runtime) Controller() Controller {
	return NewController(r.layout)
}
