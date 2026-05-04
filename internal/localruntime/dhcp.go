// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// ErrLeaseDiscoveryTimeout is returned by DiscoverGuestIPv4 when no lease
// matching the VM's MAC address appears in the lease database within the
// configured timeout. The non-darwin case (lease database simply does not
// exist) collapses into the same code path: the file is missing, polling
// sees no leases, the timeout elapses, this error fires.
// validateLocalHostPlatform prevents non-darwin users from reaching this
// path in production.
var ErrLeaseDiscoveryTimeout = errors.New("local runtime guest IP discovery timed out")

// dhcpLeaseDatabasePath is the path the launcher reads to discover the
// guest IPv4 address. Overridable for tests.
var dhcpLeaseDatabasePath = "/var/db/dhcpd_leases"

// dhcpLeasePollInterval is the cadence used by DiscoverGuestIPv4 while it
// waits for a usable lease to appear.
const dhcpLeasePollInterval = 500 * time.Millisecond

// dhcpLease represents one parsed entry from the macOS bootpd lease
// database. macOS records DHCP leases under one of two `hw_address=`
// formats depending on the client:
//
//   - Legacy: `1,<MAC>` (htype=1 / Ethernet) — older systems and clients
//     that send a hardware-address client identifier. The `<MAC>` is the
//     full 6-byte hardware address. Captured into HWAddress.
//
//   - RFC 4361 client identifier: `ff,<IAID>:<DUID>` (htype=255). The
//     IAID is the first 4 bytes after the comma; for clients that follow
//     RFC 4361 (Linux's dhclient, Alpine's busybox-udhcpc) the IAID is
//     the last 4 bytes of the interface's MAC. The remaining bytes are
//     the DUID, which identifies the host's vmnet bridge, not the VM.
//     Captured into IAIDSuffix as a canonical 4-byte colon-separated
//     lowercase form (matches the last 4 bytes of the VM's MAC).
//
// The selector matches on either field so the launcher works against
// both formats without needing to know which one the running guest's
// DHCP client uses.
type dhcpLease struct {
	IPAddress  string
	HWAddress  string
	IAIDSuffix string
	LeaseTime  int64
}

// DiscoverGuestIPv4 polls the host's DHCP lease database until a lease
// whose `hw_address=` matches the supplied VM MAC appears, then returns
// that lease's IP. Returns ErrLeaseDiscoveryTimeout if no matching lease
// appears before the configured timeout.
func (*Runtime) DiscoverGuestIPv4(
	ctx context.Context,
	mac string,
	timeout time.Duration,
) (string, error) {
	canonicalMAC, err := canonicalizeMAC(mac)
	if err != nil {
		return "", err
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(dhcpLeasePollInterval)
	defer ticker.Stop()

	for {
		ip, err := readDiscoveredIPOnce(dhcpLeaseDatabasePath, canonicalMAC)
		if err == nil {
			return ip, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf(
				"%w: no lease for MAC %s found in %q after %s",
				ErrLeaseDiscoveryTimeout,
				canonicalMAC,
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

func readDiscoveredIPOnce(path string, canonicalMAC string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errLeaseNotYetAvailable("lease database not present")
		}

		return "", fmt.Errorf("failed to read DHCP lease database %q: %w", path, err)
	}

	leases := parseDHCPLeases(data)
	lease, ok := selectLeaseByMAC(leases, canonicalMAC)
	if !ok {
		return "", errLeaseNotYetAvailable("no matching lease in database")
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
		hasIdent bool
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
		case "hw_address":
			ident, ok := parseLeaseHWAddress(strings.TrimSpace(value))
			if ok {
				lease.HWAddress = ident.FullMAC
				lease.IAIDSuffix = ident.IAIDSuffix
				hasIdent = true
			}
		case "lease":
			parsed, err := parseHexInt64(strings.TrimSpace(value))
			if err == nil {
				lease.LeaseTime = parsed
				hasLease = true
			}
		default:
			// Other fields (name, identifier, etc.) are not needed for
			// discovery — ignore them.
		}
	}

	return lease, hasIP && hasIdent && hasLease
}

// leaseHWAddress is the parsed identifier extracted from a `hw_address=`
// line. Exactly one of FullMAC or IAIDSuffix is populated.
type leaseHWAddress struct {
	FullMAC    string
	IAIDSuffix string
}

// parseLeaseHWAddress decodes a `hw_address=<htype>,<bytes>` value from
// the macOS bootpd lease database. Returns the full MAC for the legacy
// `1,<MAC>` format, or the 4-byte IAID suffix for the RFC 4361
// `ff,<IAID>:<DUID>` format. Both forms are returned in canonical
// lowercase colon-separated form. The boolean is false for unrecognized
// htypes or malformed values.
func parseLeaseHWAddress(value string) (leaseHWAddress, bool) {
	htype, rest, found := strings.Cut(value, ",")
	if !found {
		return leaseHWAddress{}, false
	}
	rest = strings.TrimSpace(rest)
	switch strings.TrimSpace(htype) {
	case "1":
		canonical, err := canonicalizeMAC(rest)
		if err != nil {
			return leaseHWAddress{}, false
		}

		return leaseHWAddress{FullMAC: canonical}, true
	case "ff":
		suffix, err := parseIAIDSuffix(rest)
		if err != nil {
			return leaseHWAddress{}, false
		}

		return leaseHWAddress{IAIDSuffix: suffix}, true
	default:
		return leaseHWAddress{}, false
	}
}

// parseIAIDSuffix returns the canonical lowercase form of the first 4
// octets of a colon-separated hex byte sequence — the RFC 4361 IAID,
// which Linux DHCP clients derive from the last 4 bytes of the MAC.
func parseIAIDSuffix(value string) (string, error) {
	const iaidOctetCount = 4

	parts := strings.Split(value, ":")
	if len(parts) < iaidOctetCount {
		return "", fmt.Errorf(
			"IAID prefix in %q has fewer than %d octets",
			value, iaidOctetCount,
		)
	}
	bytes := make([]byte, iaidOctetCount)
	for i := range iaidOctetCount {
		parsed, err := strconv.ParseUint(parts[i], 16, 8)
		if err != nil {
			return "", fmt.Errorf("invalid IAID octet %q: %w", parts[i], err)
		}
		bytes[i] = byte(parsed)
	}

	return fmt.Sprintf(
		"%02x:%02x:%02x:%02x",
		bytes[0], bytes[1], bytes[2], bytes[3],
	), nil
}

func parseHexInt64(value string) (int64, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "0x")
	value = strings.TrimPrefix(value, "0X")

	return strconv.ParseInt(value, 16, 64)
}

// canonicalizeMAC normalizes a MAC address string to a lowercase
// colon-separated form so comparisons between launcher-provided values and
// lease-file values succeed regardless of original formatting.
func canonicalizeMAC(value string) (string, error) {
	hw, err := net.ParseMAC(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("invalid MAC address %q: %w", value, err)
	}

	return hw.String(), nil
}

// selectLeaseByMAC returns the lease whose recorded identifier matches
// the running VM's MAC. Matches both the legacy full-MAC form and the
// RFC 4361 IAID-suffix form (last 4 bytes of MAC).
func selectLeaseByMAC(leases []dhcpLease, canonicalMAC string) (dhcpLease, bool) {
	suffix := lastFourBytesOfMAC(canonicalMAC)
	for _, lease := range leases {
		if lease.HWAddress != "" && lease.HWAddress == canonicalMAC {
			return lease, true
		}
		if lease.IAIDSuffix != "" && suffix != "" && lease.IAIDSuffix == suffix {
			return lease, true
		}
	}

	return dhcpLease{}, false
}

// lastFourBytesOfMAC returns the last 4 octets of a canonical 6-byte
// MAC as a lowercase colon-separated string for comparison against
// RFC 4361 IAID suffixes. Returns "" if the input is not a 6-octet MAC.
func lastFourBytesOfMAC(canonicalMAC string) string {
	const ethernetOctets = 6

	parts := strings.Split(canonicalMAC, ":")
	if len(parts) != ethernetOctets {
		return ""
	}

	return strings.Join(parts[ethernetOctets-4:], ":")
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
