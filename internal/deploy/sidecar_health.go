// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/sidecar"
)

const (
	sidecarHealthStarting = "starting"
	sidecarHealthServing  = "serving"
	sidecarHealthFailing  = "failing"
)

type sidecarHealth struct {
	State   string
	Failure string
}

func probeSidecar(
	ctx context.Context,
	manager *localinstall.SidecarRuntime,
	container localinstall.ContainerListing,
	definition sidecar.Container,
	now time.Time,
) sidecarHealth {
	definition = definition.Defaults()
	probe := definition.ReadinessProbe
	if probe == nil {
		return sidecarHealth{}
	}
	elapsed := now.Sub(container.StartedAt)
	if elapsed < seconds(probe.InitialDelaySeconds) {
		return sidecarHealth{State: sidecarHealthStarting}
	}
	err := runProbe(ctx, manager, container, definition.Ports, probe)
	switch {
	case err == nil:
		return sidecarHealth{State: sidecarHealthServing}
	case elapsed < seconds(probe.Window()):
		return sidecarHealth{State: sidecarHealthStarting, Failure: err.Error()}
	default:
		return sidecarHealth{State: sidecarHealthFailing, Failure: err.Error()}
	}
}

func runProbe(
	ctx context.Context,
	manager *localinstall.SidecarRuntime,
	container localinstall.ContainerListing,
	ports []sidecar.Port,
	probe *sidecar.Probe,
) error {
	ctx, cancel := context.WithTimeout(ctx, seconds(probe.TimeoutSeconds))
	defer cancel()
	switch {
	case probe.Exec != nil:
		return manager.Exec(ctx, container, probe.Exec.Command)
	case probe.HTTPGet != nil:
		return httpProbe(ctx, probe.HTTPGet, probeAddress(probe.HTTPGet.Port, ports))
	default:
		return tcpProbe(ctx, probeAddress(probe.TCPSocket.Port, ports))
	}
}

// Validation guarantees a published port, whose declared host address is the
// one a user reaches on every runtime.
func probeAddress(reference sidecar.PortReference, ports []sidecar.Port) string {
	port, _ := reference.Port(ports)

	return hostAddress(port.HostIP, port.HostPort)
}

func httpProbe(ctx context.Context, action *sidecar.HTTPGetAction, address string) error {
	target := (&url.URL{
		Scheme: map[string]string{sidecar.HTTP: "http", sidecar.HTTPS: "https"}[action.Scheme],
		Host:   address,
		Path:   action.Path,
	}).String()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	client := &http.Client{
		// The sidecar is on loopback, so a user's proxy settings do not apply.
		Transport: &http.Transport{
			Proxy: nil,
			// Kubernetes skips verification too; a local service rarely has a
			// certificate for its loopback address.
			//nolint:gosec // The probe checks availability, not identity.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	_ = response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("GET %s returned status %d", target, response.StatusCode)
	}

	return nil
}

func tcpProbe(ctx context.Context, address string) error {
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}

	return connection.Close()
}

func seconds(value int) time.Duration {
	return time.Duration(value) * time.Second
}
