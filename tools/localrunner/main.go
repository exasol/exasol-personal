// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	dirPerm              = 0o700
	filePerm             = 0o600
	executablePerm       = 0o700
	defaultRepo          = "exasol/exasol-local-vm"
	defaultVersion       = "v1.0.0-rc1"
	defaultAsset         = "mac-runner-aarch64.zip"
	defaultResourcePath  = "launcher"
	defaultGeneratedPath = "assets/localruntimebin/generated/darwin/arm64/mac-runner-aarch64"
	defaultDownloadDir   = "assets/localruntimebin/downloads/darwin/arm64"
	placeholderText      = "placeholder for go:embed"
)

type runnerConfig struct {
	repo         string
	version      string
	asset        string
	resourcePath string
	downloadDir  string
	targetPath   string
	runnerPath   string
	baseURL      string
	targetGOOS   string
	targetGOARCH string
	force        bool
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("expected subcommand: placeholder, download, or stage")
	}

	switch args[0] {
	case "placeholder":
		return runPlaceholder(args[1:])
	case "download":
		return runDownload(ctx, args[1:])
	case "stage":
		return runStage(ctx, args[1:])
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func runPlaceholder(args []string) error {
	flags := flag.NewFlagSet("placeholder", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	targetPath := flags.String("target", defaultGeneratedPath, "Placeholder file path")
	text := flags.String("text", placeholderText, "Placeholder file content")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if _, err := os.Stat(*targetPath); err == nil {
		fmt.Fprintf(os.Stdout, "Exasol Local runner placeholder already exists: %s\n", *targetPath)

		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(*targetPath), dirPerm); err != nil {
		return err
	}

	return os.WriteFile(*targetPath, []byte(*text), executablePerm)
}

func runDownload(ctx context.Context, args []string) error {
	config, err := parseRunnerFlags("download", args)
	if err != nil {
		return err
	}

	targetPath := filepath.Join(config.downloadDir, "mac-runner-aarch64")

	return downloadRunner(ctx, config, targetPath)
}

func runStage(ctx context.Context, args []string) error {
	config, err := parseRunnerFlags("stage", args)
	if err != nil {
		return err
	}
	if config.targetGOOS != "darwin" || config.targetGOARCH != "arm64" {
		fmt.Fprintf(
			os.Stdout,
			"Skipping Exasol Local runner staging for %s/%s\n",
			config.targetGOOS,
			config.targetGOARCH,
		)

		return nil
	}

	sourcePath, err := resolveRunnerSource(ctx, config)
	if err != nil {
		return err
	}

	if err := copyExecutable(sourcePath, config.targetPath); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Staged Exasol Local runner: %s -> %s\n", sourcePath, config.targetPath)

	return nil
}

func parseRunnerFlags(name string, args []string) (runnerConfig, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	config := runnerConfig{}
	flags.StringVar(&config.repo, "repo", defaultRepo, "GitHub repository")
	flags.StringVar(&config.version, "version", defaultVersion, "GitHub release version")
	flags.StringVar(&config.asset, "asset", defaultAsset, "GitHub release asset")
	flags.StringVar(&config.resourcePath, "resource", defaultResourcePath, "Path inside zip")
	flags.StringVar(&config.downloadDir, "download-dir", defaultDownloadDir, "Download cache dir")
	flags.StringVar(&config.targetPath, "target", defaultGeneratedPath, "Staged runner path")
	flags.StringVar(&config.runnerPath, "runner-path", "", "Existing runner binary path")
	flags.StringVar(&config.baseURL, "base-url", "", "Release asset base URL override")
	flags.StringVar(&config.targetGOOS, "goos", targetEnv("GOOS", runtime.GOOS), "Target GOOS")
	flags.StringVar(&config.targetGOARCH, "goarch", targetEnv("GOARCH", runtime.GOARCH), "Target GOARCH")
	flags.BoolVar(&config.force, "force", false, "Refresh existing download")
	if err := flags.Parse(args); err != nil {
		return runnerConfig{}, err
	}

	if strings.TrimSpace(config.repo) == "" {
		return runnerConfig{}, errors.New("repo must not be empty")
	}
	if strings.TrimSpace(config.version) == "" {
		return runnerConfig{}, errors.New("version must not be empty")
	}
	if strings.TrimSpace(config.asset) == "" {
		return runnerConfig{}, errors.New("asset must not be empty")
	}

	return config, nil
}

func targetEnv(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}

	return value
}

func resolveRunnerSource(ctx context.Context, config runnerConfig) (string, error) {
	if source := strings.TrimSpace(config.runnerPath); source != "" {
		return requireFile(source)
	}
	if source := validCachedRunner(config); source != "" {
		return source, nil
	}

	targetPath := filepath.Join(config.downloadDir, "mac-runner-aarch64")
	if err := downloadRunner(ctx, config, targetPath); err != nil {
		return "", err
	}

	return targetPath, nil
}

func requireFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("runner source is a directory: %s", path)
	}

	return path, nil
}

func validCachedRunner(config runnerConfig) string {
	if config.version == "latest" {
		return ""
	}

	targetPath := filepath.Join(config.downloadDir, "mac-runner-aarch64")
	if _, err := os.Stat(targetPath); err != nil {
		return ""
	}

	data, err := os.ReadFile(versionFilePath(config.downloadDir))
	if err != nil {
		return ""
	}
	if strings.TrimSpace(string(data)) != cacheKey(config) {
		return ""
	}

	return targetPath
}

func downloadRunner(ctx context.Context, config runnerConfig, targetPath string) error {
	if !config.force && validCachedRunner(config) == targetPath {
		fmt.Fprintf(os.Stdout, "Using cached Exasol Local runner: %s\n", targetPath)

		return nil
	}

	if err := os.MkdirAll(config.downloadDir, dirPerm); err != nil {
		return err
	}

	archivePath, cleanup, err := downloadReleaseAsset(ctx, config)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := verifyChecksum(ctx, config, archivePath); err != nil {
		return err
	}
	if err := extractZipResource(archivePath, config.resourcePath, targetPath); err != nil {
		return err
	}
	if err := os.WriteFile(versionFilePath(config.downloadDir), []byte(cacheKey(config)+"\n"), filePerm); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "Downloaded Exasol Local runner to %s\n", targetPath)

	return nil
}

func downloadReleaseAsset(
	ctx context.Context,
	config runnerConfig,
) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "exasol-local-runner-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}

	archivePath := filepath.Join(tmpDir, config.asset)
	if err := downloadFile(ctx, releaseAssetURL(config, config.asset), archivePath); err != nil {
		cleanup()

		return "", func() {}, err
	}

	return archivePath, cleanup, nil
}

func verifyChecksum(ctx context.Context, config runnerConfig, archivePath string) error {
	checksumPath := archivePath + ".sha256"
	found, err := downloadOptionalFile(
		ctx,
		releaseAssetURL(config, config.asset+".sha256"),
		checksumPath,
	)
	if err != nil {
		return err
	}
	if !found {
		fmt.Fprintf(
			os.Stderr,
			"Warning: checksum asset not found for %s; continuing without checksum verification\n",
			config.asset,
		)

		return nil
	}

	expected, err := readExpectedChecksum(checksumPath)
	if err != nil {
		return err
	}
	actual, err := sha256OfFile(archivePath)
	if err != nil {
		return err
	}
	if expected != actual {
		return fmt.Errorf(
			"checksum mismatch for %s: expected %s, got %s",
			config.asset,
			expected,
			actual,
		)
	}

	return nil
}

func releaseAssetURL(config runnerConfig, asset string) string {
	if strings.TrimSpace(config.baseURL) != "" {
		return strings.TrimRight(config.baseURL, "/") + "/" + asset
	}

	if config.version == "latest" {
		return fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", config.repo, asset)
	}

	return fmt.Sprintf(
		"https://github.com/%s/releases/download/%s/%s",
		config.repo,
		config.version,
		asset,
	)
}

func downloadFile(ctx context.Context, url, destPath string) error {
	found, err := downloadURLFile(ctx, url, destPath)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("download %s: not found", url)
	}

	return nil
}

func downloadOptionalFile(ctx context.Context, url, destPath string) (bool, error) {
	return downloadURLFile(ctx, url, destPath)
}

func downloadURLFile(ctx context.Context, rawURL, destPath string) (bool, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false, err
	}

	switch parsed.Scheme {
	case "http", "https":
		return downloadHTTPFile(ctx, rawURL, destPath)
	case "file":
		return copyURLFile(parsed, destPath)
	default:
		return false, fmt.Errorf("unsupported download URL scheme in %q", rawURL)
	}
}

func downloadHTTPFile(ctx context.Context, url, destPath string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("download %s: %s", url, resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return false, err
	}

	return true, nil
}

func copyURLFile(parsed *url.URL, destPath string) (bool, error) {
	if parsed.Host != "" {
		return false, fmt.Errorf("file URL %q must not contain a host", parsed.String())
	}

	source, err := os.Open(fileURLLocalPath(parsed))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}
	defer func() {
		_ = source.Close()
	}()

	dest, err := os.Create(destPath)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = dest.Close()
	}()

	if _, err := io.Copy(dest, source); err != nil {
		return false, err
	}

	return true, nil
}

func fileURLLocalPath(parsed *url.URL) string {
	localPath := parsed.Path
	if runtime.GOOS == "windows" && len(localPath) >= 3 &&
		localPath[0] == '/' && localPath[2] == ':' {
		localPath = localPath[1:]
	}

	return filepath.FromSlash(localPath)
}

func extractZipResource(archivePath, resourcePath, targetPath string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = reader.Close()
	}()

	resourcePath = path.Clean(resourcePath)
	if resourcePath == "." || strings.HasPrefix(resourcePath, "../") || path.IsAbs(resourcePath) {
		return fmt.Errorf("invalid zip resource path %q", resourcePath)
	}

	for _, entry := range reader.File {
		entryName := path.Clean(filepath.ToSlash(entry.Name))
		if entryName != resourcePath {
			continue
		}
		if entry.FileInfo().IsDir() {
			return fmt.Errorf("zip resource %q is a directory", resourcePath)
		}

		return extractZipEntry(entry, targetPath)
	}

	return fmt.Errorf("downloaded runner archive does not contain %s", resourcePath)
}

func extractZipEntry(entry *zip.File, targetPath string) error {
	input, err := entry.Open()
	if err != nil {
		return err
	}
	defer func() {
		_ = input.Close()
	}()

	if err := os.MkdirAll(filepath.Dir(targetPath), dirPerm); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(targetPath), "runner-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	_, copyErr := io.Copy(tmpFile, input)
	closeErr := tmpFile.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)

		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)

		return closeErr
	}
	if err := os.Chmod(tmpPath, executablePerm); err != nil {
		_ = os.Remove(tmpPath)

		return err
	}

	_ = os.Remove(targetPath)

	return os.Rename(tmpPath, targetPath)
}

func copyExecutable(sourcePath, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = source.Close()
	}()

	if err := os.MkdirAll(filepath.Dir(targetPath), dirPerm); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(targetPath), "runner-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	_, copyErr := io.Copy(tmpFile, source)
	closeErr := tmpFile.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)

		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)

		return closeErr
	}
	if err := os.Chmod(tmpPath, executablePerm); err != nil {
		_ = os.Remove(tmpPath)

		return err
	}

	_ = os.Remove(targetPath)

	return os.Rename(tmpPath, targetPath)
}

func readExpectedChecksum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	for _, field := range strings.Fields(string(data)) {
		candidate := strings.Trim(field, "*()=")
		if len(candidate) != sha256.Size*2 {
			continue
		}
		if _, err := hex.DecodeString(candidate); err == nil {
			return strings.ToLower(candidate), nil
		}
	}

	return "", fmt.Errorf("checksum file %s does not contain a SHA-256 digest", path)
}

func sha256OfFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func cacheKey(config runnerConfig) string {
	return config.repo + "@" + config.version + "/" + config.asset
}

func versionFilePath(downloadDir string) string {
	return filepath.Join(downloadDir, ".version")
}
