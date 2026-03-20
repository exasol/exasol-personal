// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exoscale

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// apiClient wraps direct HTTP calls to Exoscale API v2
type apiClient struct {
	apiKey    string
	apiSecret string
	endpoint  string
	client    *http.Client
}

// newAPIClient creates a client for direct API calls
func newAPIClient(zone string) (*apiClient, error) {
	apiKey := os.Getenv("EXOSCALE_API_KEY")
	apiSecret := os.Getenv("EXOSCALE_API_SECRET")
	
	if apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("EXOSCALE_API_KEY and EXOSCALE_API_SECRET environment variables are required")
	}

	return &apiClient{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		endpoint:  fmt.Sprintf("https://api-%s.exoscale.com/v2", zone),
		client:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// signRequest signs an API request using EXO2-HMAC-SHA256
// Based on github.com/exoscale/egoscale/v2/api/security.go
func (c *apiClient) signRequest(method, path string, body []byte, queryParams url.Values) (string, int64, string) {
	expires := time.Now().UTC().Add(10 * time.Minute).Unix()
	
	// Build message to sign
	var sigParts []string
	
	// Request method and URL path
	sigParts = append(sigParts, method+" "+path)
	
	// Request body (always include, even if empty)
	if len(body) > 0 {
		sigParts = append(sigParts, string(body))
	} else {
		sigParts = append(sigParts, "")
	}
	
	// Query parameters (sorted keys, concatenated values)
	// Only include parameters with exactly 1 value
	var paramNames []string
	for param, values := range queryParams {
		if len(values) == 1 {
			paramNames = append(paramNames, param)
		}
	}
	sort.Strings(paramNames)
	
	var paramValues string
	for _, param := range paramNames {
		paramValues += queryParams.Get(param)
	}
	sigParts = append(sigParts, paramValues)
	
	// Request headers (none currently)
	sigParts = append(sigParts, "")
	
	// Expiration timestamp
	sigParts = append(sigParts, strconv.FormatInt(expires, 10))
	
	message := strings.Join(sigParts, "\n")
	
	// Compute HMAC-SHA256
	mac := hmac.New(sha256.New, []byte(c.apiSecret))
	mac.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	
	// Build signed-query-args pragma
	signedQueryArgs := ""
	if len(paramNames) > 0 {
		signedQueryArgs = strings.Join(paramNames, ";")
	}
	
	return signature, expires, signedQueryArgs
}

// doRequest performs a signed HTTP request
func (c *apiClient) doRequest(ctx context.Context, method, path string, body []byte, queryParams url.Values) ([]byte, error) {
	// Parse the full URL to get proper escaping
	fullURL := c.endpoint + path
	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}
	
	// Add query parameters
	if len(queryParams) > 0 {
		parsedURL.RawQuery = queryParams.Encode()
	}
	
	// Use the escaped path for signing (including leading slash)
	escapedPath := parsedURL.EscapedPath()
	
	signature, expires, signedQueryArgs := c.signRequest(method, escapedPath, body, queryParams)
	
	req, err := http.NewRequestWithContext(ctx, method, parsedURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set authorization header with optional signed-query-args
	authHeader := fmt.Sprintf("EXO2-HMAC-SHA256 credential=%s,expires=%d,signature=%s",
		c.apiKey, expires, signature)
	if signedQueryArgs != "" {
		authHeader += ",signed-query-args=" + signedQueryArgs
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}
	
	return respBody, nil
}

// BlockStorageVolume represents a block storage volume from the API
type BlockStorageVolume struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	State     string            `json:"state"`
	Size      int64             `json:"size"`
	CreatedAt string            `json:"created-at"`
	Labels    map[string]string `json:"labels"`
	Instance  *struct {
		ID string `json:"id"`
	} `json:"instance"`
}

// listBlockStorageVolumes lists all block storage volumes in the zone
func (c *apiClient) listBlockStorageVolumes(ctx context.Context) ([]BlockStorageVolume, error) {
	respBody, err := c.doRequest(ctx, "GET", "/block-storage", nil, nil)
	if err != nil {
		return nil, err
	}
	
	var response struct {
		BlockStorageVolumes []BlockStorageVolume `json:"block-storage-volumes"`
	}
	
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	return response.BlockStorageVolumes, nil
}

// IAMAPIKey represents an IAM API key from the API
type IAMAPIKey struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	RoleID string `json:"role-id"`
}

// listIAMAPIKeys lists all IAM API keys
func (c *apiClient) listIAMAPIKeys(ctx context.Context) ([]IAMAPIKey, error) {
	respBody, err := c.doRequest(ctx, "GET", "/api-key", nil, nil)
	if err != nil {
		return nil, err
	}
	
	var response struct {
		APIKeys []IAMAPIKey `json:"api-keys"`
	}
	
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	return response.APIKeys, nil
}
