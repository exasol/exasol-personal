// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

const hetznerActionTimeoutMinutes = 5

const hcloudTokenEnvVar = "HCLOUD_TOKEN"

func hetznerStopServers(ctx context.Context, instanceIDs []string) error {
	token := os.Getenv(hcloudTokenEnvVar)
	if token == "" {
		return fmt.Errorf("%s environment variable is not set", hcloudTokenEnvVar)
	}

	client := hcloud.NewClient(hcloud.WithToken(token))

	for _, idStr := range instanceIDs {
		serverID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid server ID %q: %w", idStr, err)
		}

		server := &hcloud.Server{ID: serverID}

		slog.Info("shutting down Hetzner server", "id", serverID)
		result, _, err := client.Server.Shutdown(ctx, server)
		if err != nil {
			slog.Warn("graceful shutdown failed, forcing poweroff", "id", serverID, "error", err)
			result, _, err = client.Server.Poweroff(ctx, server)
			if err != nil {
				return fmt.Errorf("failed to power off server %d: %w", serverID, err)
			}
		}

		if err := waitForAction(ctx, client, result); err != nil {
			return fmt.Errorf("server %d stop action failed: %w", serverID, err)
		}

		slog.Info("Hetzner server stopped", "id", serverID)
	}

	return nil
}

func hetznerStartServers(ctx context.Context, instanceIDs []string) error {
	token := os.Getenv(hcloudTokenEnvVar)
	if token == "" {
		return fmt.Errorf("%s environment variable is not set", hcloudTokenEnvVar)
	}

	client := hcloud.NewClient(hcloud.WithToken(token))

	for _, idStr := range instanceIDs {
		serverID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid server ID %q: %w", idStr, err)
		}

		server := &hcloud.Server{ID: serverID}

		slog.Info("powering on Hetzner server", "id", serverID)
		result, _, err := client.Server.Poweron(ctx, server)
		if err != nil {
			return fmt.Errorf("failed to power on server %d: %w", serverID, err)
		}

		if err := waitForAction(ctx, client, result); err != nil {
			return fmt.Errorf("server %d start action failed: %w", serverID, err)
		}

		slog.Info("Hetzner server started", "id", serverID)
	}

	return nil
}

func waitForAction(ctx context.Context, client *hcloud.Client, action *hcloud.Action) error {
	if action == nil {
		return nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, hetznerActionTimeoutMinutes*time.Minute)
	defer cancel()

	err := client.Action.WaitForFunc(timeoutCtx, func(update *hcloud.Action) error {
		if update.Status == hcloud.ActionStatusError {
			return fmt.Errorf("action %d failed: %s", update.ID, update.ErrorMessage)
		}

		return nil
	}, action)

	return err
}
