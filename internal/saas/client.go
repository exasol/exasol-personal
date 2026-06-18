// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package saas implements the launcher-driven migration of a local Exasol
// deployment into an Exasol SaaS database: SaaS account access (token
// management, allowed-IP list), connectivity testing, and the migration of
// schema, data, and database objects.
package saas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

// apiPath is the REST API path segment every SaaS endpoint lives under.
const apiPath = "/api/v1"

// DefaultBaseURL is the production SaaS REST API base.
const DefaultBaseURL = "https://cloud.exasol.com" + apiPath

// BaseURLEnv overrides the SaaS API base for non-production (testing)
// environments. The value may be just the host (e.g.
// "https://test.cloud-dev.exasol.com") — the "/api/v1" path is appended when
// missing. When unset, DefaultBaseURL is used.
const BaseURLEnv = "EXASOL_SAAS_API_URL"

// resolveBaseURL returns the base URL from the environment override (normalized
// to include the API path), or the production default when the override is unset.
func resolveBaseURL() string {
	if override := strings.TrimSpace(os.Getenv(BaseURLEnv)); override != "" {
		return normalizeBaseURL(override)
	}

	return DefaultBaseURL
}

// normalizeBaseURL trims a trailing slash and ensures the API path is present,
// so callers can supply just the host.
func normalizeBaseURL(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if !strings.HasSuffix(trimmed, apiPath) {
		trimmed += apiPath
	}

	return trimmed
}

var (
	// ErrUnauthorized indicates the SaaS API did not accept the token (401).
	ErrUnauthorized = errors.New("saas token rejected by the api")
	// ErrForbidden indicates the token is valid but not authorized for the
	// requested account/resource (403) — usually a wrong account id.
	ErrForbidden = errors.New("saas access forbidden")
	// ErrNotFound indicates the requested SaaS resource does not exist.
	ErrNotFound = errors.New("saas resource not found")
)

// Account identifies a SaaS account. The SaaS API has no account-info endpoint,
// so this is populated from the account id the client was built with.
type Account struct {
	ID string `json:"id"`
}

// DatabaseConnection holds the resolved connection details of a SaaS database.
type DatabaseConnection struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	JDBC string `json:"jdbc"`
}

// Database is the subset of a SaaS database the launcher consumes.
type Database struct {
	ID         string             `json:"id"`
	Status     string             `json:"status"`
	Connection DatabaseConnection `json:"connection"`
}

// AllowedIP is one entry of the account's allowed-IP list
// (`/accounts/{accountId}/security/allowlist_ip`).
type AllowedIP struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name"`
	CidrIp string `json:"cidrIp"`
}

// API abstracts the SaaS REST calls the launcher depends on so callers (and
// tests) can substitute the transport.
type API interface {
	ValidateToken(ctx context.Context) (*Account, error)
	ResolveDatabase(ctx context.Context, dbUUID string) (*Database, error)
	ListAllowedIPs(ctx context.Context) ([]AllowedIP, error)
	AddAllowedIP(ctx context.Context, entry AllowedIP) error
	DetectEgressIP(ctx context.Context) (string, error)
}

// httpDoer is the minimal HTTP surface Client needs; *http.Client satisfies it.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client is the HTTP implementation of API.
type Client struct {
	baseURL   string
	token     string
	accountID string
	http      httpDoer
}

// ClientOption customizes a Client.
type ClientOption func(*Client)

// WithBaseURL overrides the SaaS API base URL.
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) {
		if strings.TrimSpace(baseURL) != "" {
			c.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

// WithHTTPDoer overrides the HTTP transport (used in tests).
func WithHTTPDoer(doer httpDoer) ClientOption {
	return func(c *Client) {
		if doer != nil {
			c.http = doer
		}
	}
}

// NewClient builds a SaaS API client for the given account and token.
func NewClient(accountID, token string, opts ...ClientOption) *Client {
	client := &Client{
		baseURL:   resolveBaseURL(),
		token:     token,
		accountID: accountID,
		http:      http.DefaultClient,
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// ValidateToken confirms the token and account by listing the account's
// databases (the SaaS API has no dedicated token/account endpoint). A 2xx JSON
// response means the token and account are valid.
func (c *Client) ValidateToken(ctx context.Context) (*Account, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, c.accountPath("/databases"), nil, &raw); err != nil {
		return nil, err
	}

	return &Account{ID: c.accountID}, nil
}

// cluster is one entry of a database's cluster list.
type cluster struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MainCluster bool   `json:"mainCluster"`
}

// clusterConnection is the connect-info subset the launcher consumes.
type clusterConnection struct {
	DNS  string `json:"dns"`
	Port int    `json:"port"`
	JDBC string `json:"jdbc"`
}

// ResolveDatabase fetches the database and, when it is running, resolves its
// connection host/port from the main cluster's connect endpoint (the database
// object itself carries only cluster counts, not connection details).
func (c *Client) ResolveDatabase(ctx context.Context, dbUUID string) (*Database, error) {
	var database Database
	dbPath := c.accountPath("/databases/" + dbUUID)
	if err := c.do(ctx, http.MethodGet, dbPath, nil, &database); err != nil {
		return nil, err
	}

	if !isRunning(database.Status) {
		return &database, nil
	}

	conn, err := c.resolveClusterConnection(ctx, dbUUID)
	if err != nil {
		return nil, err
	}
	database.Connection = DatabaseConnection{Host: conn.DNS, Port: conn.Port, JDBC: conn.JDBC}

	return &database, nil
}

// mainClusterID returns the main cluster's id, falling back to the first cluster.
func mainClusterID(clusters []cluster) string {
	for _, cl := range clusters {
		if cl.MainCluster {
			return cl.ID
		}
	}
	if len(clusters) > 0 {
		return clusters[0].ID
	}

	return ""
}

func (c *Client) ListAllowedIPs(ctx context.Context) ([]AllowedIP, error) {
	var entries []AllowedIP
	if err := c.do(
		ctx,
		http.MethodGet,
		c.accountPath("/security/allowlist_ip"),
		nil,
		&entries,
	); err != nil {
		return nil, err
	}

	return entries, nil
}

func (c *Client) AddAllowedIP(ctx context.Context, entry AllowedIP) error {
	return c.do(ctx, http.MethodPost, c.accountPath("/security/allowlist_ip"), entry, nil)
}

func (c *Client) resolveClusterConnection(
	ctx context.Context,
	dbUUID string,
) (clusterConnection, error) {
	var clusters []cluster
	clustersPath := c.accountPath("/databases/" + dbUUID + "/clusters")
	if err := c.do(ctx, http.MethodGet, clustersPath, nil, &clusters); err != nil {
		return clusterConnection{}, fmt.Errorf("listing clusters: %w", err)
	}

	clusterID := mainClusterID(clusters)
	if clusterID == "" {
		return clusterConnection{}, errors.New("no cluster found for database")
	}

	var conn clusterConnection
	connectPath := c.accountPath("/databases/" + dbUUID + "/clusters/" + clusterID + "/connect")
	if err := c.do(ctx, http.MethodGet, connectPath, nil, &conn); err != nil {
		return clusterConnection{}, fmt.Errorf("getting cluster connection: %w", err)
	}

	return conn, nil
}

func (c *Client) accountPath(suffix string) string {
	return "/accounts/" + c.accountID + suffix
}

// do issues a request, decoding a JSON body into out (when non-nil) and mapping
// well-known status codes to sentinel errors.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	slog.Debug("calling saas api", "method", method, "path", path)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("calling saas api: %w", err)
	}
	defer resp.Body.Close()

	if err := statusError(resp); err != nil {
		return err
	}

	if out == nil {
		return nil
	}

	// A non-JSON 2xx body means the request reached a web page (e.g. the SaaS
	// SPA) rather than the API — usually a wrong base URL, account id, or path.
	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "json") {
		return fmt.Errorf(
			"unexpected non-JSON response from %s (status %d, content-type %q); "+
				"check the SaaS API URL (%s) and account id",
			path, resp.StatusCode, contentType, c.baseURL,
		)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding saas api response: %w", err)
	}

	return nil
}

func statusError(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512)) //nolint:mnd
	detail := strings.TrimSpace(string(snippet))

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w (status 401): %s", ErrUnauthorized, detail)
	case http.StatusForbidden:
		return fmt.Errorf(
			"%w (status 403): token is valid but not authorized for this account "+
				"— verify the --account id is correct. %s",
			ErrForbidden, detail,
		)
	case http.StatusNotFound:
		return fmt.Errorf("%w (status 404): %s", ErrNotFound, detail)
	default:
		return fmt.Errorf("saas api returned status %d: %s", resp.StatusCode, detail)
	}
}
