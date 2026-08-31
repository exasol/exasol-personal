// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"context"
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

// Probe states no identity: a download is identified by its declared checksum,
// and without one it is re-fetched on every request.
func (*HttpSource) Probe(_ context.Context, _ Locator) (Probe, error) {
	return Probe{}, nil
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
