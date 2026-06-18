// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package saas

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToCIDR(t *testing.T) {
	t.Parallel()

	cidr, err := ToCIDR("203.0.113.7")
	require.NoError(t, err)
	require.Equal(t, "203.0.113.7/32", cidr)

	cidr, err = ToCIDR("2001:db8::1")
	require.NoError(t, err)
	require.Equal(t, "2001:db8::1/128", cidr)

	cidr, err = ToCIDR("203.0.113.0/24")
	require.NoError(t, err)
	require.Equal(t, "203.0.113.0/24", cidr)

	_, err = ToCIDR("not-an-ip")
	require.Error(t, err)
}

func TestDetectEgressIP_PlainBody(t *testing.T) {
	t.Parallel()

	var gotPath, gotAuth string
	doer := fakeDoer{fn: func(req *http.Request) (*http.Response, error) {
		gotPath = req.URL.Path
		gotAuth = req.Header.Get("Authorization")

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("203.0.113.9\n")),
			Header:     make(http.Header),
		}, nil
	}}

	client := NewClient("ORG-1", "tok", WithHTTPDoer(doer), WithBaseURL("https://api.test/api/v1"))
	ip, err := client.DetectEgressIP(context.Background())
	require.NoError(t, err)
	require.Equal(t, "203.0.113.9", ip)
	require.Equal(t, "/api/v1/internal/my_ip", gotPath)
	require.Equal(t, "Bearer tok", gotAuth)
}

func TestDetectEgressIP_JSONBody(t *testing.T) {
	t.Parallel()

	doer := fakeDoer{fn: func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ip":"203.0.113.10"}`)),
			Header:     make(http.Header),
		}, nil
	}}

	client := NewClient("ORG-1", "tok", WithHTTPDoer(doer))
	ip, err := client.DetectEgressIP(context.Background())
	require.NoError(t, err)
	require.Equal(t, "203.0.113.10", ip)
}

func TestDetectEgressIP_InvalidBody(t *testing.T) {
	t.Parallel()

	doer := fakeDoer{fn: func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("garbage")),
			Header:     make(http.Header),
		}, nil
	}}

	client := NewClient("ORG-1", "tok", WithHTTPDoer(doer))
	_, err := client.DetectEgressIP(context.Background())
	require.Error(t, err)
}
