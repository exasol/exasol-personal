// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	MetadataFileName            = ".exasolLocalRuntime.json"
	DefaultKind                 = "podman"
	DefaultImage                = "exasol/nano:latest"
	DefaultHost                 = "127.0.0.1"
	DefaultSQLPort              = 8563
	DefaultUIPort               = 8443
	DefaultShmSize              = "1g"
	DefaultReadinessTimeoutSecs = 90
)

type Config struct {
	Kind                    string
	Image                   string
	Host                    string
	SQLPort                 int
	UIPort                  int
	ShmSize                 string
	ReadinessTimeoutSeconds int
}

type InstallOptions struct {
	DeploymentDir string
	DeploymentID  string
	Config        Config
}

type InstallResult struct {
	Metadata Metadata
}

type DestroyOptions struct {
	DeploymentDir string
}

type Metadata struct {
	Kind          string    `json:"kind"`
	Image         string    `json:"image"`
	ContainerName string    `json:"containerName"`
	ContainerID   string    `json:"containerId"`
	Host          string    `json:"host"`
	SQLPort       int       `json:"sqlPort"`
	UIPort        int       `json:"uiPort"`
	Ephemeral     bool      `json:"ephemeral"`
	Runtime       string    `json:"runtime"`
	StartedAt     time.Time `json:"startedAt"`
}

type Backend interface {
	Install(ctx context.Context, opts InstallOptions) (*InstallResult, error)
	Destroy(ctx context.Context, opts DestroyOptions) error
}

type Executor interface {
	Run(ctx context.Context, command []string, workDir string) ([]byte, error)
}

type LocalExecutor struct{}

func (*LocalExecutor) Run(ctx context.Context, command []string, workDir string) ([]byte, error) {
	if len(command) == 0 {
		return nil, errors.New("no command provided")
	}

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = workDir

	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return out, err
		}

		return out, fmt.Errorf("%w: %s", err, msg)
	}

	return out, nil
}

type podmanBackend struct {
	executor Executor
}

func NewBackend(cfg Config, executor Executor) (Backend, error) {
	if executor == nil {
		executor = &LocalExecutor{}
	}

	kind := strings.TrimSpace(cfg.Kind)
	if kind == "" {
		kind = DefaultKind
	}

	switch kind {
	case DefaultKind:
		return &podmanBackend{executor: executor}, nil
	default:
		return nil, fmt.Errorf("unsupported local runtime kind %q", kind)
	}
}

func (b *podmanBackend) Install(ctx context.Context, opts InstallOptions) (*InstallResult, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, fmt.Errorf("local Podman runtime is only supported on macOS and Linux, not %s", runtime.GOOS)
	}

	cfg := withDefaults(opts.Config)
	if cfg.SQLPort == cfg.UIPort {
		return nil, errors.New("sql and ui ports must be different")
	}

	if err := ensureLoopbackPortAvailable(cfg.SQLPort); err != nil {
		return nil, fmt.Errorf("sql port %d is unavailable: %w", cfg.SQLPort, err)
	}
	if err := ensureLoopbackPortAvailable(cfg.UIPort); err != nil {
		return nil, fmt.Errorf("ui port %d is unavailable: %w", cfg.UIPort, err)
	}

	if _, err := b.executor.Run(ctx, []string{"podman", "--version"}, opts.DeploymentDir); err != nil {
		return nil, fmt.Errorf("podman is required for local installs: %w", err)
	}

	containerName := buildContainerName(opts.DeploymentID)
	command := []string{
		"podman", "run", "--rm", "-d",
		"--name", containerName,
		"--shm-size", cfg.ShmSize,
		"-p", fmt.Sprintf("%d:%d", cfg.SQLPort, DefaultSQLPort),
		"-p", fmt.Sprintf("%d:%d", cfg.UIPort, DefaultUIPort),
		cfg.Image,
	}

	out, err := b.executor.Run(ctx, command, opts.DeploymentDir)
	if err != nil {
		return nil, fmt.Errorf("failed to start Exasol Nano container: %w", err)
	}

	containerID := strings.TrimSpace(string(out))
	if containerID == "" {
		return nil, errors.New("podman did not return a container id")
	}

	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.ReadinessTimeoutSeconds)*time.Second)
	defer cancel()
	if err := waitForTCP(waitCtx, cfg.Host, cfg.SQLPort); err != nil {
		_, _ = b.executor.Run(context.Background(),
			[]string{"podman", "stop", "-t", "10", containerName},
			opts.DeploymentDir)

		return nil, fmt.Errorf("exasol nano did not become reachable on %s:%d: %w", cfg.Host, cfg.SQLPort, err)
	}

	metadata := Metadata{
		Kind:          DefaultKind,
		Image:         cfg.Image,
		ContainerName: containerName,
		ContainerID:   containerID,
		Host:          cfg.Host,
		SQLPort:       cfg.SQLPort,
		UIPort:        cfg.UIPort,
		Ephemeral:     true,
		Runtime:       "podman",
		StartedAt:     time.Now().UTC(),
	}
	if err := WriteMetadata(opts.DeploymentDir, &metadata); err != nil {
		return nil, err
	}

	// The UI port may take longer to come up than the SQL port; wait for it
	// before attempting the TLS handshake to capture the certificate.
	if err := waitForTCP(waitCtx, cfg.Host, cfg.UIPort); err != nil {
		_, _ = b.executor.Run(context.Background(),
			[]string{"podman", "stop", "-t", "10", containerName},
			opts.DeploymentDir)

		return nil, fmt.Errorf("exasol nano UI did not become reachable on %s:%d: %w", cfg.Host, cfg.UIPort, err)
	}

	tlsCertPEM, err := fetchServerCert(waitCtx, cfg.Host, cfg.UIPort)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch TLS certificate from Exasol Nano: %w", err)
	}

	if err := writeDeploymentContract(opts.DeploymentDir, opts.DeploymentID, &metadata, tlsCertPEM); err != nil {
		return nil, err
	}

	return &InstallResult{Metadata: metadata}, nil
}

func (b *podmanBackend) Destroy(ctx context.Context, opts DestroyOptions) error {
	metadata, err := ReadMetadata(opts.DeploymentDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("failed to read local runtime metadata: %w", err)
	}

	_, err = b.executor.Run(ctx,
		[]string{"podman", "stop", "-t", "10", metadata.ContainerName},
		opts.DeploymentDir,
	)
	if err != nil && !strings.Contains(err.Error(), "no such container") {
		return fmt.Errorf("failed to stop local runtime container %q: %w", metadata.ContainerName, err)
	}

	for _, f := range []string{
		metadataPath(opts.DeploymentDir),
		filepath.Join(opts.DeploymentDir, "deployment.json"),
		filepath.Join(opts.DeploymentDir, "secrets.json"),
	} {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove %s: %w", filepath.Base(f), err)
		}
	}

	return nil
}

// writeDeploymentContract writes deployment.json and secrets.json in the same
// schema that cloud Tofu presets produce. This allows all downstream code
// (connection instructions, readiness checks, connect) to work uniformly
// regardless of whether the deployment is local or cloud-based.
func writeDeploymentContract(deploymentDir string, deploymentID string, metadata *Metadata, tlsCertPEM string) error {
	uiPort := fmt.Sprintf("%d", metadata.UIPort)

	nodeDetails := map[string]any{
		"deploymentId": deploymentID,
		"clusterSize":  1,
		"clusterState": "running",
		"nodes": map[string]any{
			"n11": map[string]any{
				"dnsName":  metadata.Host,
				"publicIp": metadata.Host,
				"tlsCert":  tlsCertPEM,
				"database": map[string]any{
					"dbPort": fmt.Sprintf("%d", metadata.SQLPort),
					"uiPort": uiPort,
					"url":    "https://" + net.JoinHostPort(metadata.Host, uiPort),
				},
			},
		},
	}

	secrets := map[string]any{
		"dbPassword":      "exasol",
		"adminUiPassword": "admin",
	}

	if err := writeJSONFile(filepath.Join(deploymentDir, "deployment.json"), nodeDetails); err != nil {
		return fmt.Errorf("failed to write deployment contract (node details): %w", err)
	}

	if err := writeJSONFile(filepath.Join(deploymentDir, "secrets.json"), secrets); err != nil {
		return fmt.Errorf("failed to write deployment contract (secrets): %w", err)
	}

	return nil
}

func writeJSONFile(path string, data any) error {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, content, 0o600)
}

// fetchServerCert performs a TLS handshake against the given host:port and
// returns the server's leaf certificate as a PEM-encoded string.
// It retries because the HTTPS server inside the container may not be ready
// even after the TCP port is accepting connections.
func fetchServerCert(ctx context.Context, host string, port int) (string, error) {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastErr error
	for {
		conn, err := tls.DialWithDialer(
			&net.Dialer{Timeout: 5 * time.Second},
			"tcp",
			addr,
			&tls.Config{InsecureSkipVerify: true}, //nolint:gosec // need to accept self-signed cert to read it
		)
		if err == nil {
			defer conn.Close()

			certs := conn.ConnectionState().PeerCertificates
			if len(certs) == 0 {
				return "", errors.New("server returned no TLS certificates")
			}

			pemBytes := pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: certs[0].Raw,
			})

			return string(pemBytes), nil
		}

		lastErr = err

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("TLS handshake did not succeed on %s before timeout: %w", addr, lastErr)
		case <-ticker.C:
		}
	}
}

func withDefaults(cfg Config) Config {
	if strings.TrimSpace(cfg.Kind) == "" {
		cfg.Kind = DefaultKind
	}
	if strings.TrimSpace(cfg.Image) == "" {
		cfg.Image = DefaultImage
	}
	if strings.TrimSpace(cfg.Host) == "" {
		cfg.Host = DefaultHost
	}
	if cfg.SQLPort == 0 {
		cfg.SQLPort = DefaultSQLPort
	}
	if cfg.UIPort == 0 {
		cfg.UIPort = DefaultUIPort
	}
	if strings.TrimSpace(cfg.ShmSize) == "" {
		cfg.ShmSize = DefaultShmSize
	}
	if cfg.ReadinessTimeoutSeconds <= 0 {
		cfg.ReadinessTimeoutSeconds = DefaultReadinessTimeoutSecs
	}

	return cfg
}

func buildContainerName(deploymentID string) string {
	name := strings.TrimSpace(deploymentID)
	if name == "" {
		name = "local"
	}

	name = strings.ToLower(name)
	name = regexp.MustCompile(`[^a-z0-9_.-]+`).ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		name = "local"
	}

	return "exasol-nano-" + name
}

func ensureLoopbackPortAvailable(port int) error {
	address := fmt.Sprintf("%s:%d", DefaultHost, port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	return listener.Close()
}

func waitForTCP(ctx context.Context, host string, port int) error {
	address := fmt.Sprintf("%s:%d", host, port)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func metadataPath(deploymentDir string) string {
	return filepath.Join(deploymentDir, MetadataFileName)
}

func WriteMetadata(deploymentDir string, metadata *Metadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to encode local runtime metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath(deploymentDir), data, 0o600); err != nil {
		return fmt.Errorf("failed to write local runtime metadata: %w", err)
	}

	return nil
}

func ReadMetadata(deploymentDir string) (*Metadata, error) {
	data, err := os.ReadFile(metadataPath(deploymentDir))
	if err != nil {
		return nil, err
	}

	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to decode local runtime metadata: %w", err)
	}

	return &metadata, nil
}

func MetadataExists(deploymentDir string) bool {
	_, err := os.Stat(metadataPath(deploymentDir))
	return err == nil
}
