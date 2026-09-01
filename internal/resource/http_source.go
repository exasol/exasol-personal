// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type HttpSource struct{}

func (*HttpSource) Handles(loc Locator) bool {
	return (strings.HasPrefix(loc.URL, "http://") || strings.HasPrefix(loc.URL, "https://")) &&
		!IsGitSourceURL(loc.URL)
}

// Probe: only strong ETags identify exact bytes; failed probes defer errors to Fetch.
func (*HttpSource) Probe(ctx context.Context, loc Locator) (Probe, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, loc.URL, nil)
	if err != nil {
		return Probe{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Probe{}, nil //nolint:nilerr
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Probe{}, nil
	}

	etag := strings.TrimSpace(resp.Header.Get("ETag"))
	if etag == "" || strings.HasPrefix(etag, "W/") {
		return Probe{}, nil
	}

	addressHash := sha256.Sum256([]byte(loc.URL))

	return Probe{
		Identity: "http-etag:" + hex.EncodeToString(addressHash[:]) + ":" + etag,
	}, nil
}

func (*HttpSource) Fetch(ctx context.Context, loc Locator, dstPath string) error {
	tmpDownload, err := os.CreateTemp(filepath.Dir(dstPath), "download-*")
	if err != nil {
		return err
	}
	tmpDownloadPath := tmpDownload.Name()
	if err := tmpDownload.Close(); err != nil {
		_ = os.Remove(tmpDownloadPath)
		return err
	}

	if err := downloadFile(ctx, loc.URL, tmpDownloadPath); err != nil {
		_ = os.Remove(tmpDownloadPath)
		return err
	}

	if err := os.Rename(tmpDownloadPath, dstPath); err != nil {
		_ = os.Remove(tmpDownloadPath)
		return err
	}

	return nil
}

func downloadFile(ctx context.Context, url, dstPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch %s (%s)", url, resp.Status)
	}

	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	_, err = io.Copy(out, resp.Body)

	return err
}
