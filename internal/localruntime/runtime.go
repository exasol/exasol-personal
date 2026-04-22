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

func (r *Runtime) AllocatePort(name string) (int, error) {
	if err := r.EnsureRoot(); err != nil {
		return 0, err
	}

	state, err := r.LoadState()
	if err != nil {
		return 0, err
	}

	if state.Ports == nil {
		state.Ports = make(map[string]int)
	}
	if port, exists := state.Ports[name]; exists && port > 0 {
		return port, nil
	}

	excluded := make(map[int]struct{}, len(state.Ports))
	for _, port := range state.Ports {
		if port > 0 {
			excluded[port] = struct{}{}
		}
	}

	port, err := r.allocatePort(excluded)
	if err != nil {
		return 0, err
	}

	state.Ports[name] = port
	if err := r.SaveState(state); err != nil {
		return 0, err
	}

	return port, nil
}

func (r *Runtime) Controller() Controller {
	return NewController(r.layout)
}
