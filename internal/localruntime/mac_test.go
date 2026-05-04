// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"net"
	"testing"
)

func TestGenerateLocallyAdministeredMAC_ReturnsLAAUnicast(t *testing.T) {
	t.Parallel()

	mac, err := generateLocallyAdministeredMAC()
	if err != nil {
		t.Fatalf("expected MAC generation to succeed, got %v", err)
	}
	parsed, err := net.ParseMAC(mac)
	if err != nil {
		t.Fatalf("expected generated MAC to be parseable, got %v", err)
	}
	if len(parsed) != 6 {
		t.Fatalf("expected 6-octet MAC, got %d octets", len(parsed))
	}
	if parsed[0]&0x02 == 0 {
		t.Fatalf(
			"expected locally-administered bit to be set in first octet, got %#x",
			parsed[0],
		)
	}
	if parsed[0]&0x01 != 0 {
		t.Fatalf(
			"expected multicast bit to be cleared in first octet, got %#x",
			parsed[0],
		)
	}
}

func TestGenerateLocallyAdministeredMAC_RandomEachCall(t *testing.T) {
	t.Parallel()

	first, err := generateLocallyAdministeredMAC()
	if err != nil {
		t.Fatalf("expected first MAC to be generated, got %v", err)
	}
	second, err := generateLocallyAdministeredMAC()
	if err != nil {
		t.Fatalf("expected second MAC to be generated, got %v", err)
	}
	if first == second {
		t.Fatalf("expected different MACs across calls, both were %q", first)
	}
}
