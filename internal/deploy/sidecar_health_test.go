// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

// probedCatalog publishes hostPort on loopback and declares the given probes.
func probedCatalog(t *testing.T, hostPort int, probes string) *sidecar.Catalog {
	t.Helper()
	catalog, err := sidecar.LoadCatalog(fmt.Appendf(nil, `version: 1
sidecars:
  example:
    description: Test service
    architectures: [amd64, arm64]
    container:
      image: docker.io/library/caddy:2-alpine
      ports:
        - name: web
          containerPort: 8080
          hostPort: %d
          hostIP: 127.0.0.1
%s`, hostPort, probes))
	require.NoError(t, err)

	return catalog
}

func servingPort(t *testing.T, status int) int {
	t.Helper()
	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(status)
		}),
	)
	t.Cleanup(server.Close)
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	number, err := strconv.Atoi(port)
	require.NoError(t, err)

	return number
}

// closedPort reports a loopback port that nothing listens on any more.
func closedPort(t *testing.T) int {
	t.Helper()
	server := httptest.NewServer(http.NotFoundHandler())
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	server.Close()
	number, err := strconv.Atoi(port)
	require.NoError(t, err)

	return number
}

// probedStatus enables the example sidecar and reports its status the given
// time after its container started.
func probedStatus(
	t *testing.T, catalog *sidecar.Catalog, elapsed time.Duration,
	prepare func(*sidecarRuntimeStub),
) (SidecarResult, *sidecarRuntimeStub) {
	t.Helper()
	operations, selected := newSidecarOperationsFixture(t)
	operations.templates = catalog
	selected.containers.age = elapsed
	if prepare != nil {
		prepare(selected)
	}
	_, err := operations.enable(t.Context(), "example")
	require.NoError(t, err)
	result, err := operations.status(t.Context(), "example")
	require.NoError(t, err)

	return result, selected
}

func TestSidecarHealthIsServingWhenReadinessSucceeds(t *testing.T) {
	t.Parallel()
	// Given
	port := servingPort(t, http.StatusOK)
	catalog := probedCatalog(
		t,
		port,
		"      readinessProbe: {httpGet: {port: web, path: /health}}\n",
	)
	// When
	result, _ := probedStatus(t, catalog, time.Second, nil)
	// Then
	require.Equal(t, sidecarHealthServing, result.Health)
	require.Empty(t, result.HealthError)
	require.Equal(t, "running", result.State)
}

func TestSidecarHealthIsStartingInsideTheProbeWindow(t *testing.T) {
	t.Parallel()
	// Given
	catalog := probedCatalog(t, closedPort(t), "      readinessProbe: {tcpSocket: {port: web}}\n")
	// When
	result, _ := probedStatus(t, catalog, 29*time.Second, nil)
	// Then
	require.Equal(t, sidecarHealthStarting, result.Health)
	require.Contains(t, result.HealthError, "connection refused")
}

func TestSidecarHealthIsFailingPastTheProbeWindow(t *testing.T) {
	t.Parallel()
	// Given
	port := servingPort(t, http.StatusServiceUnavailable)
	catalog := probedCatalog(t, port, "      readinessProbe: {httpGet: {port: 8080}}\n")
	// When
	result, _ := probedStatus(t, catalog, 30*time.Second, nil)
	// Then
	require.Equal(t, sidecarHealthFailing, result.Health)
	require.Contains(t, result.HealthError, "returned status 503")
	require.Equal(t, "running", result.State)
	require.True(t, result.Running)
}

func TestSidecarWithoutProbesReportsNoHealth(t *testing.T) {
	t.Parallel()
	// Given
	catalog := probedCatalog(t, closedPort(t), "")
	// When
	result, _ := probedStatus(t, catalog, time.Hour, nil)
	// Then
	require.Empty(t, result.Health)
	require.Empty(t, result.HealthError)
}

func TestSidecarProbeWaitsForItsInitialDelay(t *testing.T) {
	t.Parallel()
	// Given
	catalog := probedCatalog(t, closedPort(t),
		"      readinessProbe: {exec: {command: [check]}, initialDelaySeconds: 10}\n")
	// When
	result, selected := probedStatus(t, catalog, 9*time.Second, nil)
	// Then
	require.Equal(t, sidecarHealthStarting, result.Health)
	require.Empty(t, result.HealthError)
	require.Empty(t, selected.containers.Execs())
}

func TestSidecarExecProbeRunsInsideTheContainer(t *testing.T) {
	t.Parallel()
	// Given
	catalog := probedCatalog(t, closedPort(t),
		"      readinessProbe: {exec: {command: [check, $(HOME)]}}\n")
	// When
	result, selected := probedStatus(t, catalog, time.Second, nil)
	// Then
	require.Equal(t, sidecarHealthServing, result.Health)
	require.Contains(t, selected.containers.Execs(),
		[]string{sidecarTestOwner + "-sidecar-example", "check", "$(HOME)"})
}

func TestSidecarFailingExecProbeReportsItsCause(t *testing.T) {
	t.Parallel()
	// Given
	catalog := probedCatalog(t, closedPort(t),
		"      readinessProbe: {exec: {command: [check]}, periodSeconds: 1, failureThreshold: 1}\n")
	// When
	result, _ := probedStatus(t, catalog, 2*time.Second, func(selected *sidecarRuntimeStub) {
		selected.containers.Fail("exec", errors.New("check failed"))
	})
	// Then
	require.Equal(t, sidecarHealthFailing, result.Health)
	require.Equal(t, "check failed", result.HealthError)
}

func TestSidecarStoppedContainerReportsNoHealth(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	operations.templates = probedCatalog(t, closedPort(t),
		"      readinessProbe: {tcpSocket: {port: web}}\n")
	_, err := operations.enable(t.Context(), "example")
	require.NoError(t, err)
	selected.containers.Stop()
	// When
	result, err := operations.status(t.Context(), "example")
	// Then
	require.NoError(t, err)
	require.False(t, result.Running)
	require.Empty(t, result.Health)
}

func TestSidecarEditedProbeNeedsNoRecreation(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	operations.templates = probedCatalog(t, closedPort(t),
		"      readinessProbe: {tcpSocket: {port: web}}\n")
	_, err := operations.enable(t.Context(), "example")
	require.NoError(t, err)
	created, _ := selected.containers.Calls()
	document, err := config.ReadSidecars(operations.deployment)
	require.NoError(t, err)
	definition := document.Sidecars["example"]
	definition.Container.ReadinessProbe = &sidecar.Probe{
		Exec: &sidecar.ExecAction{Command: []string{"check"}},
	}
	document.Sidecars["example"] = definition
	require.NoError(t, config.WriteSidecars(operations.deployment, document))
	// When
	result, err := operations.status(t.Context(), "example")
	// Then
	require.NoError(t, err)
	require.Equal(t, sidecarHealthServing, result.Health)
	require.Equal(t, sidecarReconciliationComplete, result.Reconciliation)
	recreated, _ := selected.containers.Calls()
	require.Equal(t, created, recreated)
}

// waitingFixture enables a sidecar whose exec readiness probe fails.
func waitingFixture(
	t *testing.T, port int, probe string,
) (*deploymentSidecars, *sidecarRuntimeStub, SidecarResult) {
	t.Helper()
	operations, selected := newSidecarOperationsFixture(t)
	operations.templates = probedCatalog(t, port, probe)
	selected.containers.Fail("exec", errors.New("not ready"))
	result, err := operations.enable(t.Context(), "example")
	require.NoError(t, err)
	require.Equal(t, sidecarHealthStarting, result.Health)

	return operations, selected, result
}

func TestSidecarEnableWaitsUntilServing(t *testing.T) {
	t.Parallel()
	port := closedPort(t)
	synctest.Test(t, func(t *testing.T) {
		// Given
		started := time.Now()
		operations, selected, first := waitingFixture(t, port,
			"      readinessProbe: {exec: {command: [check]}}\n")
		go func() {
			time.Sleep(3500 * time.Millisecond)
			selected.containers.Fail("exec", nil)
		}()
		// When
		result, err := operations.awaitHealth(t.Context(), "example", first)
		// Then
		require.NoError(t, err)
		require.Equal(t, sidecarHealthServing, result.Health)
		require.Equal(t, 4*time.Second, time.Since(started))
	})
}

func TestSidecarEnableReportsFailingAfterTheProbeWindow(t *testing.T) {
	t.Parallel()
	port := closedPort(t)
	synctest.Test(t, func(t *testing.T) {
		// Given
		started := time.Now()
		operations, _, first := waitingFixture(t, port,
			"      readinessProbe: {exec: {command: [check]}, periodSeconds: 2}\n")
		// When
		result, err := operations.awaitHealth(t.Context(), "example", first)
		// Then
		require.NoError(t, err)
		require.Equal(t, sidecarHealthFailing, result.Health)
		require.Equal(t, "not ready", result.HealthError)
		require.Equal(t, 6*time.Second, time.Since(started))
	})
}

func TestSidecarEnableWaitEndsWhileRestartsKeepItStarting(t *testing.T) {
	t.Parallel()
	port := closedPort(t)
	synctest.Test(t, func(t *testing.T) {
		// Given
		started := time.Now()
		operations, selected, first := waitingFixture(t, port,
			"      readinessProbe: {exec: {command: [check]}, periodSeconds: 2}\n")
		done := make(chan struct{})
		// Restarts fall between polls, so every poll sees a fresh container.
		go func() {
			for wait := 500 * time.Millisecond; ; wait = time.Second {
				select {
				case <-done:
					return
				case <-time.After(wait):
					selected.containers.Restart()
				}
			}
		}()
		// When
		result, err := operations.awaitHealth(t.Context(), "example", first)
		close(done)
		// Then
		require.NoError(t, err)
		require.Equal(t, sidecarHealthStarting, result.Health)
		require.Equal(t, 6*time.Second, time.Since(started))
	})
}

func TestSidecarEnableWaitStopsWhenCancelled(t *testing.T) {
	t.Parallel()
	// Given
	operations, _, first := waitingFixture(t, closedPort(t),
		"      readinessProbe: {exec: {command: [check]}}\n")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// When
	_, err := operations.awaitHealth(ctx, "example", first)
	// Then
	require.ErrorIs(t, err, context.Canceled)
}
