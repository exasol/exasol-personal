// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package saas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// egressIPPath is the SaaS endpoint that reports the caller's public egress IP.
const egressIPPath = "/internal/my_ip"

// DetectEgressIP returns the caller's public egress IP as seen by the SaaS API.
// It calls the authenticated SaaS endpoint so the IP matches what SaaS would
// allowlist for outbound EXPORT connections.
func (c *Client) DetectEgressIP(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+egressIPPath, nil)
	if err != nil {
		return "", fmt.Errorf("building egress detection request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("detecting egress ip: %w", err)
	}
	defer resp.Body.Close()

	if err := statusError(resp); err != nil {
		return "", err
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256)) //nolint:mnd
	if err != nil {
		return "", fmt.Errorf("reading egress detection response: %w", err)
	}

	return parseEgressIP(body)
}

// parseEgressIP extracts an IP from the my_ip response, accepting either a plain
// IP body or a JSON object with an "ip" field.
func parseEgressIP(body []byte) (string, error) {
	raw := strings.TrimSpace(string(body))
	if net.ParseIP(raw) != nil {
		return raw, nil
	}

	var payload struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if ip := strings.TrimSpace(payload.IP); net.ParseIP(ip) != nil {
			return ip, nil
		}
	}

	return "", fmt.Errorf("egress detection returned an invalid ip: %q", raw)
}

// ToCIDR normalizes an IP or CIDR into CIDR form, defaulting a bare IPv4 to /32
// and a bare IPv6 to /128.
func ToCIDR(addr string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", errors.New("empty address")
	}

	if strings.Contains(addr, "/") {
		if _, _, err := net.ParseCIDR(addr); err != nil {
			return "", fmt.Errorf("invalid cidr %q: %w", addr, err)
		}

		return addr, nil
	}

	ip := net.ParseIP(addr)
	if ip == nil {
		return "", fmt.Errorf("invalid ip address %q", addr)
	}

	if ip.To4() != nil {
		return addr + "/32", nil
	}

	return addr + "/128", nil
}
