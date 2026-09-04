// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
)

const (
	// RustSLCAlias is the fixed alias `exasol slc install rust` installs under.
	RustSLCAlias = "RUST"

	rustSLCLanguage = "rust"

	rustSLCRepo = "exasol-labs/language-container-rs"

	rustSLCAssetPrefix = "lc-rust-"

	rustSLCLatestReleaseURL = "https://api.github.com/repos/" + rustSLCRepo + "/releases/latest"

	rustSLCReleaseTimeout = 10 * time.Second
)

// RustSLCInstallOpts is the option surface for `exasol slc install rust`. Alias and language are
// fixed (RustSLCAlias, rustSLCLanguage), so unlike CustomSLCInstallOpts there is nothing here to
// validate beyond Source, which the shared custom-SLC path already validates.
type RustSLCInstallOpts struct {
	Source string
}

// InstallRustSLC installs the Rust script language container under the fixed alias RustSLCAlias,
// defaulting Source to the latest matching release of language-container-rs when it is empty, and
// otherwise delegating to the existing custom-SLC install path unchanged.
//
//nolint:revive // restart is a user-controlled flag (--no-restart), not internal control coupling.
func InstallRustSLC(
	ctx context.Context,
	deployment config.DeploymentDir,
	opts RustSLCInstallOpts,
	verbose bool,
	restart bool,
	confirm CustomSLCConfirm,
) (*CustomSLCInstallResult, error) {
	source, err := resolveRustSLCSource(ctx, opts.Source)
	if err != nil {
		return nil, err
	}

	return InstallCustomSLC(
		ctx,
		deployment,
		CustomSLCInstallOpts{Alias: RustSLCAlias, Language: rustSLCLanguage, Source: source},
		verbose,
		restart,
		confirm,
	)
}

// resolveRustSLCSource returns an explicit source unchanged, so an explicit --source never
// triggers a network call to resolve a release.
func resolveRustSLCSource(ctx context.Context, source string) (string, error) {
	if strings.TrimSpace(source) != "" {
		return source, nil
	}

	return latestRustSLCAssetURL(ctx, rustSLCLatestReleaseURL, runtime.GOARCH)
}

// rustSLCArchSuffix maps a Go architecture to the suffix language-container-rs uses in its
// release asset names: no suffix for x86_64, "-aarch64" for arm64.
func rustSLCArchSuffix(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "", nil
	case "arm64":
		return "-aarch64", nil
	default:
		return "", fmt.Errorf(
			"the Rust SLC publishes no release for architecture %q; "+
				"supported architectures are amd64 (x86_64) and arm64 (aarch64)",
			goarch,
		)
	}
}

// latestRustSLCAssetURL resolves the latest release tag of language-container-rs and builds the
// download URL for the asset matching goarch. releaseURL is a parameter, not a package constant,
// so tests can point it at a local server without an env var or a build tag. The architecture is
// checked before any network call, so an unsupported one fails without contacting releaseURL.
func latestRustSLCAssetURL(ctx context.Context, releaseURL, goarch string) (string, error) {
	suffix, err := rustSLCArchSuffix(goarch)
	if err != nil {
		return "", err
	}

	tag, err := latestRustSLCTag(ctx, releaseURL)
	if err != nil {
		return "", err
	}

	version := strings.TrimPrefix(tag, "v")

	return fmt.Sprintf(
		"https://github.com/%s/releases/download/%s/%s%s%s.tar.gz",
		rustSLCRepo, tag, rustSLCAssetPrefix, version, suffix,
	), nil
}

type rustSLCRelease struct {
	//nolint:tagliatelle // JSON key mirrors the GitHub releases API field name.
	TagName string `json:"tag_name"`
}

// latestRustSLCTag reads the current release tag from the GitHub releases API. The release
// assets embed the version in their filename (lc-rust-0.23.0.tar.gz), so GitHub's fixed-filename
// /releases/latest/download/<name> shortcut cannot resolve "latest" on its own; the tag has to be
// read first.
func latestRustSLCTag(ctx context.Context, releaseURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseURL, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: rustSLCReleaseTimeout, CheckRedirect: rejectNonHTTPSRedirect}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not reach %s: %w", rustSLCRepo, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"could not resolve the latest release of %s: %s returned %s",
			rustSLCRepo, releaseURL, resp.Status,
		)
	}

	var release rustSLCRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("could not parse the latest release of %s: %w", rustSLCRepo, err)
	}

	tag := strings.TrimSpace(release.TagName)
	if tag == "" {
		return "", fmt.Errorf("the latest release of %s carries no tag", rustSLCRepo)
	}

	return tag, nil
}
