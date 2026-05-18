// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package hetzner

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// Handler defines deletion behavior for a resource type.
type Handler interface {
	Delete(ctx context.Context, ref ResourceRef) error
}

var ErrUnsupportedResource = errors.New("unsupported resource type for deletion")

// registry maps resource types to concrete handlers.
var (
	registry     = map[ResourceType]Handler{}
	registryOnce sync.Once
)

// initHandlers initializes handler singletons exactly once, safe for concurrent use.
func initHandlers() {
	registryOnce.Do(func() {
		client := newHCloudClient()

		registry[ResourceServer] = &serverHandler{client: client}
		registry[ResourceVolume] = &volumeHandler{client: client}
		registry[ResourceNetwork] = &networkHandler{client: client}
		registry[ResourceFirewall] = &firewallHandler{client: client}
		registry[ResourceSSHKey] = &sshKeyHandler{client: client}
	})
}

func deleteResource(ctx context.Context, ref ResourceRef) error {
	initHandlers()
	h, ok := registry[ref.Type]
	if !ok {
		return ErrUnsupportedResource
	}

	return h.Delete(ctx, ref)
}

// Server Handler
type serverHandler struct {
	client *hcloud.Client
}

func (h *serverHandler) Delete(ctx context.Context, ref ResourceRef) error {
	id, err := strconv.ParseInt(ref.ID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid server ID: %w", err)
	}

	server := &hcloud.Server{ID: id}

	// Delete the server (also deletes associated primary IPs)
	result, _, err := h.client.Server.DeleteWithResult(ctx, server)
	if err != nil {
		if hcloud.IsError(err, hcloud.ErrorCodeNotFound) {
			return nil
		}
		return fmt.Errorf("failed to delete server: %w", err)
	}

	// Wait for the action to complete
	if err := h.client.Action.WaitFor(ctx, result.Action); err != nil {
		return fmt.Errorf("failed to wait for server deletion: %w", err)
	}

	return nil
}

// Volume Handler
type volumeHandler struct {
	client *hcloud.Client
}

func (h *volumeHandler) Delete(ctx context.Context, ref ResourceRef) error {
	id, err := strconv.ParseInt(ref.ID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid volume ID: %w", err)
	}

	volume := &hcloud.Volume{ID: id}

	// Detach volume first if attached
	vol, _, err := h.client.Volume.GetByID(ctx, id)
	if err != nil {
		if hcloud.IsError(err, hcloud.ErrorCodeNotFound) {
			return nil
		}
		return fmt.Errorf("failed to get volume: %w", err)
	}

	if vol == nil {
		return nil
	}

	if vol.Server != nil {
		action, _, err := h.client.Volume.Detach(ctx, volume)
		if err != nil {
			if !hcloud.IsError(err, hcloud.ErrorCodeNotFound) {
				return fmt.Errorf("failed to detach volume: %w", err)
			}
		} else if action != nil {
			if err := h.client.Action.WaitFor(ctx, action); err != nil {
				return fmt.Errorf("failed to wait for volume detach: %w", err)
			}
		}
	}

	_, err = h.client.Volume.Delete(ctx, volume)
	if err != nil {
		if hcloud.IsError(err, hcloud.ErrorCodeNotFound) {
			return nil
		}
		return fmt.Errorf("failed to delete volume: %w", err)
	}

	return nil
}

// Network Handler
type networkHandler struct {
	client *hcloud.Client
}

func (h *networkHandler) Delete(ctx context.Context, ref ResourceRef) error {
	id, err := strconv.ParseInt(ref.ID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid network ID: %w", err)
	}

	network := &hcloud.Network{ID: id}

	_, err = h.client.Network.Delete(ctx, network)
	if err != nil {
		if hcloud.IsError(err, hcloud.ErrorCodeNotFound) {
			return nil
		}
		return fmt.Errorf("failed to delete network: %w", err)
	}

	return nil
}

// Firewall Handler
type firewallHandler struct {
	client *hcloud.Client
}

func (h *firewallHandler) Delete(ctx context.Context, ref ResourceRef) error {
	id, err := strconv.ParseInt(ref.ID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid firewall ID: %w", err)
	}

	firewall := &hcloud.Firewall{ID: id}

	_, err = h.client.Firewall.Delete(ctx, firewall)
	if err != nil {
		if hcloud.IsError(err, hcloud.ErrorCodeNotFound) {
			return nil
		}
		return fmt.Errorf("failed to delete firewall: %w", err)
	}

	return nil
}

// SSH Key Handler
type sshKeyHandler struct {
	client *hcloud.Client
}

func (h *sshKeyHandler) Delete(ctx context.Context, ref ResourceRef) error {
	id, err := strconv.ParseInt(ref.ID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid SSH key ID: %w", err)
	}

	sshKey := &hcloud.SSHKey{ID: id}

	_, err = h.client.SSHKey.Delete(ctx, sshKey)
	if err != nil {
		if hcloud.IsError(err, hcloud.ErrorCodeNotFound) {
			return nil
		}
		return fmt.Errorf("failed to delete SSH key: %w", err)
	}

	return nil
}
