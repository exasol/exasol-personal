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

func TestNewClient_BaseURLFromEnv(t *testing.T) {
	// Not parallel: mutates process environment.
	t.Setenv(BaseURLEnv, "https://staging.exasol.test/api/v1/")

	var gotURL string
	doer := fakeDoer{fn: func(req *http.Request) (*http.Response, error) {
		gotURL = req.URL.String()
		return jsonResponse(http.StatusOK, `{"id":"ORG-1"}`), nil
	}}

	client := NewClient("ORG-1", "tok", WithHTTPDoer(doer))
	_, err := client.ValidateToken(context.Background())
	require.NoError(t, err)
	// Override is used (trailing slash trimmed) and WithBaseURL was not supplied.
	require.Equal(t, "https://staging.exasol.test/api/v1/accounts/ORG-1/databases", gotURL)
}

func TestNewClient_BaseURLDefaultsToProduction(t *testing.T) {
	// Not parallel: mutates process environment.
	t.Setenv(BaseURLEnv, "")

	client := NewClient("ORG-1", "tok")
	require.Equal(t, DefaultBaseURL, client.baseURL)
}

func TestNewClient_BaseURLEnvAppendsAPIPath(t *testing.T) {
	// Not parallel: mutates process environment.
	t.Setenv(BaseURLEnv, "https://test.cloud-dev.exasol.com")

	client := NewClient("ORG-1", "tok")
	require.Equal(t, "https://test.cloud-dev.exasol.com/api/v1", client.baseURL)
}

func jsonResponse(code int, body string) *http.Response {
	header := make(http.Header)
	header.Set("Content-Type", "application/json")

	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     header,
	}
}

// htmlResponse simulates the SaaS web app answering a wrong API path.
func htmlResponse(code int, body string) *http.Response {
	header := make(http.Header)
	header.Set("Content-Type", "text/html; charset=utf-8")

	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     header,
	}
}

func TestClientValidateToken_Success(t *testing.T) {
	t.Parallel()

	var gotAuth, gotPath string
	doer := fakeDoer{fn: func(req *http.Request) (*http.Response, error) {
		gotAuth = req.Header.Get("Authorization")
		gotPath = req.URL.Path

		return jsonResponse(http.StatusOK, `[]`), nil
	}}

	client := NewClient("ORG-1", "tok", WithHTTPDoer(doer), WithBaseURL("https://api.test/v1"))
	account, err := client.ValidateToken(context.Background())
	require.NoError(t, err)
	require.Equal(t, "ORG-1", account.ID)
	require.Equal(t, "Bearer tok", gotAuth)
	require.Equal(t, "/v1/accounts/ORG-1/databases", gotPath)
}

func TestClientValidateToken_Unauthorized(t *testing.T) {
	t.Parallel()

	doer := fakeDoer{fn: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, ``), nil
	}}

	client := NewClient("ORG-1", "bad", WithHTTPDoer(doer))
	_, err := client.ValidateToken(context.Background())
	require.ErrorIs(t, err, ErrUnauthorized)
}

func TestClientValidateToken_Forbidden(t *testing.T) {
	t.Parallel()

	doer := fakeDoer{fn: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusForbidden, `{"message":""}`), nil
	}}

	client := NewClient("ORG-1", "valid", WithHTTPDoer(doer))
	_, err := client.ValidateToken(context.Background())
	require.ErrorIs(t, err, ErrForbidden)
	require.Contains(t, err.Error(), "--account")
}

func TestClientValidateToken_NonJSONResponse(t *testing.T) {
	t.Parallel()

	doer := fakeDoer{fn: func(_ *http.Request) (*http.Response, error) {
		return htmlResponse(http.StatusOK, "<!DOCTYPE html><html></html>"), nil
	}}

	client := NewClient("ORG-1", "tok", WithHTTPDoer(doer))
	_, err := client.ValidateToken(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-JSON response")
}

func TestClientResolveDatabase(t *testing.T) {
	t.Parallel()

	// A running database resolves its host/port through clusters -> main -> connect.
	doer := fakeDoer{fn: func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/connect"):
			return jsonResponse(
				http.StatusOK,
				`{"dns":"db-1.exasol.test","port":8563,"jdbc":"jdbc:exa:db-1.exasol.test:8563"}`,
			), nil
		case strings.HasSuffix(req.URL.Path, "/clusters"):
			return jsonResponse(http.StatusOK,
				`[{"id":"c1","mainCluster":false},`+
					`{"id":"c2","mainCluster":true}]`), nil
		default:
			return jsonResponse(http.StatusOK, `{"id":"db-1","status":"running"}`), nil
		}
	}}

	client := NewClient("ORG-1", "tok", WithHTTPDoer(doer))
	db, err := client.ResolveDatabase(context.Background(), "db-1")
	require.NoError(t, err)
	require.Equal(t, "running", db.Status)
	require.Equal(t, "db-1.exasol.test", db.Connection.Host)
	require.Equal(t, 8563, db.Connection.Port)
	require.Equal(t, "jdbc:exa:db-1.exasol.test:8563", db.Connection.JDBC)
}

func TestClientResolveDatabase_NotRunningSkipsConnect(t *testing.T) {
	t.Parallel()

	doer := fakeDoer{fn: func(req *http.Request) (*http.Response, error) {
		require.False(t, strings.HasSuffix(req.URL.Path, "/connect"),
			"connect must not be called for a non-running database")

		return jsonResponse(http.StatusOK, `{"id":"db-1","status":"creating"}`), nil
	}}

	client := NewClient("ORG-1", "tok", WithHTTPDoer(doer))
	db, err := client.ResolveDatabase(context.Background(), "db-1")
	require.NoError(t, err)
	require.Equal(t, "creating", db.Status)
	require.Empty(t, db.Connection.Host)
}

func TestMainClusterID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "c2", mainClusterID([]cluster{{ID: "c1"}, {ID: "c2", MainCluster: true}}))
	require.Equal(t, "c1", mainClusterID([]cluster{{ID: "c1"}, {ID: "c3"}}))
	require.Empty(t, mainClusterID(nil))
}

func TestClientAddAllowedIP_SendsBody(t *testing.T) {
	t.Parallel()

	var method, path string
	var body []byte
	doer := fakeDoer{fn: func(req *http.Request) (*http.Response, error) {
		method = req.Method
		path = req.URL.Path
		body, _ = io.ReadAll(req.Body)

		return jsonResponse(http.StatusCreated, ``), nil
	}}

	client := NewClient("ORG-1", "tok", WithHTTPDoer(doer))
	err := client.AddAllowedIP(
		context.Background(),
		AllowedIP{Name: "src", CidrIp: "203.0.113.7/32"},
	)
	require.NoError(t, err)
	require.Equal(t, http.MethodPost, method)
	require.Equal(t, "/api/v1/accounts/ORG-1/security/allowlist_ip", path)
	require.Contains(t, string(body), `"cidrIp":"203.0.113.7/32"`)
}

func TestClientListAllowedIPs(t *testing.T) {
	t.Parallel()

	var path string
	doer := fakeDoer{fn: func(req *http.Request) (*http.Response, error) {
		path = req.URL.Path
		return jsonResponse(
			http.StatusOK,
			`[{"id":"1","name":"src","cidrIp":"203.0.113.7/32"}]`,
		), nil
	}}

	client := NewClient("ORG-1", "tok", WithHTTPDoer(doer))
	entries, err := client.ListAllowedIPs(context.Background())
	require.NoError(t, err)
	require.Equal(t, "/api/v1/accounts/ORG-1/security/allowlist_ip", path)
	require.Len(t, entries, 1)
	require.Equal(t, "203.0.113.7/32", entries[0].CidrIp)
}
