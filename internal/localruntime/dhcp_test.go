// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const fixtureSingleLease = `{
        name=exasol-vm
        ip_address=192.168.64.3
        hw_address=1,86:d0:94:72:c8:a8
        identifier=1,86:d0:94:72:c8:a8
        lease=0x683783b0
}`

const fixtureMultipleLeases = `{
        name=stale-vm
        ip_address=192.168.64.2
        hw_address=1,7e:53:90:5f:8b:c8
        lease=0x68377000
}
{
        name=exasol-vm
        ip_address=192.168.64.4
        hw_address=1,aa:04:66:9a:15:2e
        lease=0x683783b0
}
{
        name=earliest-vm
        ip_address=192.168.64.5
        hw_address=1,11:22:33:44:55:66
        lease=0x68376000
}`

func TestParseDHCPLeases_SingleLease(t *testing.T) {
	t.Parallel()

	leases := parseDHCPLeases([]byte(fixtureSingleLease))
	if len(leases) != 1 {
		t.Fatalf("expected 1 lease, got %d", len(leases))
	}
	if leases[0].IPAddress != "192.168.64.3" {
		t.Fatalf("unexpected ip: %q", leases[0].IPAddress)
	}
	if leases[0].LeaseTime != 0x683783b0 {
		t.Fatalf("unexpected lease time: %#x", leases[0].LeaseTime)
	}
}

func TestSelectMostRecentLease_PicksHighestExpiration(t *testing.T) {
	t.Parallel()

	leases := parseDHCPLeases([]byte(fixtureMultipleLeases))
	if len(leases) != 3 {
		t.Fatalf("expected 3 leases, got %d", len(leases))
	}
	winner, ok := selectMostRecentLease(leases)
	if !ok {
		t.Fatal("expected a selected lease")
	}
	if winner.IPAddress != "192.168.64.4" {
		t.Fatalf("expected most recent lease at .4, got %q", winner.IPAddress)
	}
}

func TestSelectMostRecentLease_EmptyInputReturnsFalse(t *testing.T) {
	t.Parallel()

	_, ok := selectMostRecentLease(nil)
	if ok {
		t.Fatal("expected ok=false for empty input")
	}
}

func TestParseDHCPLeases_SkipsBlocksMissingFields(t *testing.T) {
	t.Parallel()

	body := `{
        name=missing-ip
        hw_address=1,aa:bb:cc:dd:ee:ff
        lease=0x12345
}
{
        ip_address=192.168.64.7
        lease=0xdeadbeef
}
{
        ip_address=192.168.64.8
        hw_address=1,11:22:33:44:55:66
}`

	leases := parseDHCPLeases([]byte(body))
	if len(leases) != 1 {
		t.Fatalf(
			"expected only the complete block to parse, got %d leases: %#v",
			len(leases), leases,
		)
	}
	if leases[0].IPAddress != "192.168.64.7" {
		t.Fatalf("unexpected ip: %q", leases[0].IPAddress)
	}
}

func TestParseDHCPLeases_TolratesUnknownFields(t *testing.T) {
	t.Parallel()

	body := `{
        unknown=value
        ip_address=192.168.64.9
        another=0x1234
        lease=0xcafe
        trailing_field=
}`

	leases := parseDHCPLeases([]byte(body))
	if len(leases) != 1 {
		t.Fatalf("expected 1 lease, got %d", len(leases))
	}
	if leases[0].IPAddress != "192.168.64.9" {
		t.Fatalf("unexpected ip: %q", leases[0].IPAddress)
	}
	if leases[0].LeaseTime != 0xcafe {
		t.Fatalf("unexpected lease time: %#x", leases[0].LeaseTime)
	}
}

//nolint:paralleltest // mutates package-level dhcpLeaseDatabasePath.
func TestDiscoverGuestIPv4_ReturnsIPWhenLeaseAppears(t *testing.T) {
	original := dhcpLeaseDatabasePath
	leasePath := filepath.Join(t.TempDir(), "dhcpd_leases")
	dhcpLeaseDatabasePath = leasePath
	t.Cleanup(func() { dhcpLeaseDatabasePath = original })

	// Write the lease file mid-poll so we exercise the wait-then-success path.
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(leasePath, []byte(fixtureSingleLease), 0o600)
	}()

	runtime := New(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ip, err := runtime.DiscoverGuestIPv4(ctx, 5*time.Second)
	if err != nil {
		t.Fatalf("expected discovery to succeed, got %v", err)
	}
	if ip != "192.168.64.3" {
		t.Fatalf("unexpected ip: %q", ip)
	}
}

//nolint:paralleltest // mutates package-level dhcpLeaseDatabasePath.
func TestDiscoverGuestIPv4_TimesOutWhenLeaseMissing(t *testing.T) {
	original := dhcpLeaseDatabasePath
	dhcpLeaseDatabasePath = filepath.Join(t.TempDir(), "never-exists")
	t.Cleanup(func() { dhcpLeaseDatabasePath = original })

	runtime := New(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := runtime.DiscoverGuestIPv4(ctx, 600*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, ErrLeaseDiscoveryTimeout) {
		t.Fatalf("expected ErrLeaseDiscoveryTimeout, got %v", err)
	}
}
