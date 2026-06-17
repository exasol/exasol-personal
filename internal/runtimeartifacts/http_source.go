// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

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

func (*HttpSource) CanFetch(url string) bool {
	return (strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://"))
}

func (*HttpSource) Fetch(ctx context.Context, url, dstPath string) (string, error) {
	tmpDownload, err := os.CreateTemp(filepath.Dir(dstPath), "download-*")
	if err != nil {
		return "", err
	}
	tmpDownloadPath := tmpDownload.Name()
	if err := tmpDownload.Close(); err != nil {
		_ = os.Remove(tmpDownloadPath)
		return "", err
	}

	if err := downloadFile(ctx, url, tmpDownloadPath); err != nil {
		_ = os.Remove(tmpDownloadPath)
		return "", err
	}

	if err := os.Rename(tmpDownloadPath, dstPath); err != nil {
		_ = os.Remove(tmpDownloadPath)
		return "", err
	}

	return "", nil
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
