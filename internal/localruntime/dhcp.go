// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ErrLeaseDiscoveryTimeout is returned by DiscoverGuestIPv4 when no usable
// lease appears in the lease database within the configured timeout. The
// non-darwin case (lease database simply does not exist) collapses into the
// same code path: the file is missing, polling sees no leases, the timeout
// elapses, this error fires. validateLocalHostPlatform prevents non-darwin
// users from reaching this path in production.
var ErrLeaseDiscoveryTimeout = errors.New("local runtime guest IP discovery timed out")

// dhcpLeaseDatabasePath is the path the launcher reads to discover the
// guest IPv4 address. Overridable for tests.
var dhcpLeaseDatabasePath = "/var/db/dhcpd_leases"

// dhcpLeasePollInterval is the cadence used by DiscoverGuestIPv4 while it
// waits for a usable lease to appear.
const dhcpLeasePollInterval = 500 * time.Millisecond

type dhcpLease struct {
	IPAddress string
	LeaseTime int64
}

// DiscoverGuestIPv4 polls the host's DHCP lease database until a usable
// lease appears, then returns the IP from the most-recently-issued lease.
// Returns ErrLeaseDiscoveryTimeout if no usable lease appears before the
// configured timeout, or ErrLeaseDatabaseUnsupported on platforms that do
// not provide a readable lease database.
func (*Runtime) DiscoverGuestIPv4(ctx context.Context, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(dhcpLeasePollInterval)
	defer ticker.Stop()

	for {
		ip, err := readDiscoveredIPOnce(dhcpLeaseDatabasePath)
		if err == nil {
			return ip, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf(
				"%w: no usable lease found in %q after %s",
				ErrLeaseDiscoveryTimeout,
				dhcpLeaseDatabasePath,
				timeout,
			)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

func readDiscoveredIPOnce(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errLeaseNotYetAvailable("lease database not present")
		}

		return "", fmt.Errorf("failed to read DHCP lease database %q: %w", path, err)
	}

	leases := parseDHCPLeases(data)
	lease, ok := selectMostRecentLease(leases)
	if !ok {
		return "", errLeaseNotYetAvailable("no parseable leases in database")
	}

	return lease.IPAddress, nil
}

func parseDHCPLeases(data []byte) []dhcpLease {
	leases := make([]dhcpLease, 0)
	contents := string(data)

	for {
		openIdx := strings.IndexByte(contents, '{')
		closeIdx := strings.IndexByte(contents, '}')
		if openIdx < 0 || closeIdx < 0 || closeIdx < openIdx {
			break
		}
		block := contents[openIdx+1 : closeIdx]
		contents = contents[closeIdx+1:]

		if lease, ok := parseDHCPLeaseBlock(block); ok {
			leases = append(leases, lease)
		}
	}

	return leases
}

func parseDHCPLeaseBlock(block string) (dhcpLease, bool) {
	var (
		lease    dhcpLease
		hasIP    bool
		hasLease bool
	)
	for _, raw := range strings.Split(block, "\n") {
		line := strings.TrimSpace(raw)
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "ip_address":
			lease.IPAddress = strings.TrimSpace(value)
			hasIP = lease.IPAddress != ""
		case "lease":
			parsed, err := parseHexInt64(strings.TrimSpace(value))
			if err == nil {
				lease.LeaseTime = parsed
				hasLease = true
			}
		default:
			// Other fields (name, hw_address, identifier, etc.) are not
			// needed for IP discovery — ignore them.
		}
	}

	return lease, hasIP && hasLease
}

func parseHexInt64(value string) (int64, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "0x")
	value = strings.TrimPrefix(value, "0X")

	return strconv.ParseInt(value, 16, 64)
}

func selectMostRecentLease(leases []dhcpLease) (dhcpLease, bool) {
	if len(leases) == 0 {
		return dhcpLease{}, false
	}
	best := leases[0]
	for _, lease := range leases[1:] {
		if lease.LeaseTime > best.LeaseTime {
			best = lease
		}
	}

	return best, true
}

// leaseNotYetAvailableError wraps a transient "no lease yet" condition as an
// error so DiscoverGuestIPv4 keeps polling. The wrapped sentinel is hidden
// from callers — they only see ErrLeaseDiscoveryTimeout if the timeout
// elapses.
type leaseNotYetAvailableError struct{ reason string }

func (e leaseNotYetAvailableError) Error() string {
	return "lease not yet available: " + e.reason
}

func errLeaseNotYetAvailable(reason string) error {
	return leaseNotYetAvailableError{reason: reason}
}
