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

const testCurrentVMIP = "192.168.64.4"

// fixtureLeaseLegacy uses the historical `1,<MAC>` form. Some macOS
// versions and other DHCP daemons write this format.
const fixtureLeaseLegacy = `{
        name=exasol-vm
        ip_address=192.168.64.4
        hw_address=1,aa:fe:29:a7:24:ff
        identifier=1,aa:fe:29:a7:24:ff
        lease=0x683783b0
}`

// fixtureLeaseRFC4361 reproduces the format observed on macOS 15 with
// VZ NAT: client-id type 255 (`ff`), followed by an IAID derived from
// the last 4 bytes of the MAC (`29:a7:24:ff` here is the suffix of
// `aa:fe:29:a7:24:ff`), followed by a DUID-LLT containing vmnet's
// host-side identifier.
const fixtureLeaseRFC4361 = `{
        name=exasol-vm
        ip_address=192.168.64.4
        hw_address=ff,29:a7:24:ff:0:1:0:1:31:86:54:f0:52:54:0:12:34:56
        identifier=ff,29:a7:24:ff:0:1:0:1:31:86:54:f0:52:54:0:12:34:56
        lease=0x69f83b1e
}`

// fixtureMultipleLeasesRFC4361 mirrors a real-world lease database
// after several install/destroy cycles: stale entries persist with
// later expirations than the current VM's lease.
const fixtureMultipleLeasesRFC4361 = `{
        name=exasol-vm
        ip_address=192.168.64.3
        hw_address=ff,94:72:c8:a8:0:1:0:1:31:86:54:f0:52:54:0:12:34:56
        lease=0x69f99999
}
{
        name=exasol-vm
        ip_address=192.168.64.4
        hw_address=ff,29:a7:24:ff:0:1:0:1:31:86:54:f0:52:54:0:12:34:56
        lease=0x69f83b1e
}
{
        name=exasol-vm
        ip_address=192.168.64.5
        hw_address=ff,2f:7d:f5:3a:0:1:0:1:31:86:54:f0:52:54:0:12:34:56
        lease=0x69f70000
}`

func TestParseDHCPLeases_LegacyFormatCapturesFullMAC(t *testing.T) {
	t.Parallel()

	leases := parseDHCPLeases([]byte(fixtureLeaseLegacy))
	if len(leases) != 1 {
		t.Fatalf("expected 1 lease, got %d", len(leases))
	}
	if leases[0].IPAddress != testCurrentVMIP {
		t.Fatalf("unexpected ip: %q", leases[0].IPAddress)
	}
	if leases[0].HWAddress != "aa:fe:29:a7:24:ff" {
		t.Fatalf("unexpected canonical MAC: %q", leases[0].HWAddress)
	}
	if leases[0].IAIDSuffix != "" {
		t.Fatalf("expected empty IAIDSuffix for legacy format, got %q", leases[0].IAIDSuffix)
	}
}

func TestParseDHCPLeases_RFC4361CapturesIAIDSuffix(t *testing.T) {
	t.Parallel()

	leases := parseDHCPLeases([]byte(fixtureLeaseRFC4361))
	if len(leases) != 1 {
		t.Fatalf("expected 1 lease, got %d", len(leases))
	}
	if leases[0].IPAddress != testCurrentVMIP {
		t.Fatalf("unexpected ip: %q", leases[0].IPAddress)
	}
	if leases[0].HWAddress != "" {
		t.Fatalf("expected empty HWAddress for RFC 4361 format, got %q", leases[0].HWAddress)
	}
	if leases[0].IAIDSuffix != "29:a7:24:ff" {
		t.Fatalf("unexpected IAID suffix: %q", leases[0].IAIDSuffix)
	}
}

func TestSelectLeaseByMAC_LegacyExactMatch(t *testing.T) {
	t.Parallel()

	leases := parseDHCPLeases([]byte(fixtureLeaseLegacy))
	winner, ok := selectLeaseByMAC(leases, "aa:fe:29:a7:24:ff")
	if !ok {
		t.Fatal("expected a selected lease for matching legacy MAC")
	}
	if winner.IPAddress != testCurrentVMIP {
		t.Fatalf("unexpected ip: %q", winner.IPAddress)
	}
}

func TestSelectLeaseByMAC_RFC4361MatchByMACSuffix(t *testing.T) {
	t.Parallel()

	leases := parseDHCPLeases([]byte(fixtureLeaseRFC4361))
	// Full MAC of the running VM; the lease only stores the IAID
	// (last 4 bytes). Matching must succeed against the suffix.
	winner, ok := selectLeaseByMAC(leases, "aa:fe:29:a7:24:ff")
	if !ok {
		t.Fatal("expected suffix match to find the lease")
	}
	if winner.IPAddress != testCurrentVMIP {
		t.Fatalf("unexpected ip: %q", winner.IPAddress)
	}
}

func TestSelectLeaseByMAC_RFC4361IgnoresStaleEntriesWithLaterExpiration(t *testing.T) {
	t.Parallel()

	// The stale .3 entry has a later expiration than the running
	// .4 entry. Matching by MAC must still pick .4.
	leases := parseDHCPLeases([]byte(fixtureMultipleLeasesRFC4361))
	winner, ok := selectLeaseByMAC(leases, "aa:fe:29:a7:24:ff")
	if !ok {
		t.Fatal("expected suffix match to find the running VM's lease")
	}
	if winner.IPAddress != testCurrentVMIP {
		t.Fatalf("expected stale entries to be ignored, got %q", winner.IPAddress)
	}
}

func TestSelectLeaseByMAC_ReturnsFalseWhenNoSuffixMatch(t *testing.T) {
	t.Parallel()

	leases := parseDHCPLeases([]byte(fixtureMultipleLeasesRFC4361))
	_, ok := selectLeaseByMAC(leases, "aa:bb:cc:dd:ee:ff")
	if ok {
		t.Fatal("expected ok=false when no lease matches the requested MAC")
	}
}

func TestSelectLeaseByMAC_EmptyInputReturnsFalse(t *testing.T) {
	t.Parallel()

	_, ok := selectLeaseByMAC(nil, "aa:bb:cc:dd:ee:ff")
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
}
{
        name=complete
        ip_address=192.168.64.9
        hw_address=1,01:02:03:04:05:06
        lease=0xcafe
}`

	leases := parseDHCPLeases([]byte(body))
	if len(leases) != 1 {
		t.Fatalf(
			"expected only the complete block to parse, got %d leases: %#v",
			len(leases), leases,
		)
	}
	if leases[0].IPAddress != "192.168.64.9" {
		t.Fatalf("unexpected ip: %q", leases[0].IPAddress)
	}
	if leases[0].HWAddress != "01:02:03:04:05:06" {
		t.Fatalf("unexpected canonical MAC: %q", leases[0].HWAddress)
	}
}

func TestParseDHCPLeases_ToleratesUnknownFields(t *testing.T) {
	t.Parallel()

	body := `{
        unknown=value
        ip_address=192.168.64.9
        another=0x1234
        hw_address=1,aa:bb:cc:dd:ee:ff
        lease=0xcafe
        trailing_field=
}`

	leases := parseDHCPLeases([]byte(body))
	if len(leases) != 1 {
		t.Fatalf("expected 1 lease, got %d", len(leases))
	}
	if leases[0].HWAddress != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("unexpected canonical MAC: %q", leases[0].HWAddress)
	}
}

func TestCanonicalizeMAC_NormalizesFormatting(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"AA:BB:CC:DD:EE:FF": "aa:bb:cc:dd:ee:ff",
		"aa-bb-cc-dd-ee-ff": "aa:bb:cc:dd:ee:ff",
		"aabb.ccdd.eeff":    "aa:bb:cc:dd:ee:ff",
	}
	for input, expected := range cases {
		got, err := canonicalizeMAC(input)
		if err != nil {
			t.Fatalf("canonicalizeMAC(%q) errored: %v", input, err)
		}
		if got != expected {
			t.Fatalf("canonicalizeMAC(%q): got %q, want %q", input, got, expected)
		}
	}
}

func TestParseIAIDSuffix_NormalizesShortFormOctets(t *testing.T) {
	t.Parallel()

	// Real macOS bootpd writes single-digit octets without leading
	// zeros (e.g., `0:1` instead of `00:01`). The parser must accept
	// this form and emit the canonical zero-padded equivalent.
	got, err := parseIAIDSuffix("0:1:2f:7d:f5:3a:0:1:0:1")
	if err != nil {
		t.Fatalf("expected parse to succeed, got %v", err)
	}
	if got != "00:01:2f:7d" {
		t.Fatalf("unexpected canonical IAID suffix: %q", got)
	}
}

//nolint:paralleltest // mutates package-level dhcpLeaseDatabasePath.
func TestDiscoverGuestIPv4_ReturnsIPWhenLeaseAppears(t *testing.T) {
	original := dhcpLeaseDatabasePath
	leasePath := filepath.Join(t.TempDir(), "dhcpd_leases")
	dhcpLeaseDatabasePath = leasePath
	t.Cleanup(func() { dhcpLeaseDatabasePath = original })

	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(leasePath, []byte(fixtureLeaseRFC4361), 0o600)
	}()

	runtime := New(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ip, err := runtime.DiscoverGuestIPv4(ctx, "aa:fe:29:a7:24:ff", 5*time.Second)
	if err != nil {
		t.Fatalf("expected discovery to succeed, got %v", err)
	}
	if ip != testCurrentVMIP {
		t.Fatalf("unexpected ip: %q", ip)
	}
}

//nolint:paralleltest // mutates package-level dhcpLeaseDatabasePath.
func TestDiscoverGuestIPv4_TimesOutWhenMACAbsent(t *testing.T) {
	original := dhcpLeaseDatabasePath
	leasePath := filepath.Join(t.TempDir(), "dhcpd_leases")
	dhcpLeaseDatabasePath = leasePath
	t.Cleanup(func() { dhcpLeaseDatabasePath = original })

	if err := os.WriteFile(leasePath, []byte(fixtureLeaseRFC4361), 0o600); err != nil {
		t.Fatalf("expected fixture to be written, got %v", err)
	}

	runtime := New(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := runtime.DiscoverGuestIPv4(ctx, "11:22:33:44:55:66", 600*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, ErrLeaseDiscoveryTimeout) {
		t.Fatalf("expected ErrLeaseDiscoveryTimeout, got %v", err)
	}
}

//nolint:paralleltest // mutates package-level dhcpLeaseDatabasePath.
func TestDiscoverGuestIPv4_TimesOutWhenLeaseFileNeverAppears(t *testing.T) {
	original := dhcpLeaseDatabasePath
	dhcpLeaseDatabasePath = filepath.Join(t.TempDir(), "never-exists")
	t.Cleanup(func() { dhcpLeaseDatabasePath = original })

	runtime := New(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := runtime.DiscoverGuestIPv4(ctx, "aa:fe:29:a7:24:ff", 600*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, ErrLeaseDiscoveryTimeout) {
		t.Fatalf("expected ErrLeaseDiscoveryTimeout, got %v", err)
	}
}
