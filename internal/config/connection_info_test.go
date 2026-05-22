// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import "testing"

const (
	testNodeTLSCertPEM = `-----BEGIN CERTIFICATE-----
MIIDFzCCAf+gAwIBAgIUX7bPqKmldD5pRIXsmMVaqy07D3MwDQYJKoZIhvcNAQEL
BQAwGzEZMBcGA1UEAwwQZXhhY2x1c3Rlci5sb2NhbDAeFw0yNjA1MTgxNjUxNTFa
Fw0yNjA1MTkxNjUxNTFaMBsxGTAXBgNVBAMMEGV4YWNsdXN0ZXIubG9jYWwwggEi
MA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQCobbqt5Z1WFsnQuhN3UGGGc2xL
iR4S7e16/H9edtiFs03vpBevGFaZ1E/80t4fNyK3Zh3+a0QBrMOq4lL8Az3hpT/t
d7ejE6SXic+gFelelrEKExhoUBW+QvaPMKgOs/I3CqpUANy3i9eBufEEO7wD5zNY
3NpSGkjO24FA132M2ZHbA4vT/LDrQgqmCFmLfVK/AVHwJHP1FPa7cdpkXSfoNMS8
mP/ic62xKa/3eN8eKErSd46IBD6Se5H5NaTdTd5Px5QlHTjiGUYFf5sSPGytjHdO
10EZQtAINJDr8G+7xtv7/4VIOr5InTsTZ7YEoPOsLjR5JbbX6dANqetjU7g3AgMB
AAGjUzBRMB0GA1UdDgQWBBRbFgxPkDn3OVTIxkhGipwGmLWYrzAfBgNVHSMEGDAW
gBRbFgxPkDn3OVTIxkhGipwGmLWYrzAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3
DQEBCwUAA4IBAQAkminqClEKemWcz2qnFm7S+0JGvm/afImPLeI86cMm+K3FlCAk
gt0+Z+ievDe6Qlzx2hd/5ZNb06doWROAQW0Wa48YpxxYzDJYsMg2cYXcNHyfpLRY
Maod08st83Omazh8q4gdMXoPHewrvqWw8rQ8aFCYcKv2tb6ydLybfFRgQJcPGY5A
weHW+6xccsl/oQKBumqaAPic0Vogx7l73MvfI8HpwgMJz2PpeTf0aCG3Z3MJO2rw
R+38EnmWEQ6v9DLI3rNssbU6Dz6UMW+W68SitvJfsyjh9bCXt++yox1hfZOHIMGp
lKXb6y8Al2UTdJ3ED+yONdAc7OHg4e3vwJ8R
-----END CERTIFICATE-----`

	testCertSHA256Fingerprint = "CDEC9FB892603FDC23D7CFCC86A533434B5E16E69E7744E057506F2006BD732E"
)

func TestResolveConnectionInfo_UsesNormalizedDeploymentConnection(t *testing.T) {
	t.Parallel()

	// Given
	deployment := NewDeploymentDir(t.TempDir())
	if err := WriteSecrets(deployment.Root(), &Secrets{DbPassword: "secret"}); err != nil {
		t.Fatalf("failed to write secrets: %v", err)
	}
	if err := WriteDeploymentInfo(deployment.Root(), &DeploymentInfo{
		DeploymentId:    "dep-1",
		DeploymentState: "running",
		ClusterSize:     1,
		ClusterState:    "running",
		Connection: &DeploymentConnection{
			Host:           "example.local",
			DBPort:         8563,
			UIPort:         8443,
			PublicIp:       "203.0.113.10",
			SSHCommand:     "ssh ubuntu@example.local -p 22",
			SSHPort:        "22",
			ShellSupported: true,
		},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	// When
	info, err := ResolveConnectionInfo(deployment)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.Host != "example.local" {
		t.Fatalf("expected host %q, got %q", "example.local", info.Host)
	}
	if info.DisplayHost != "example.local" {
		t.Fatalf("expected display host %q, got %q", "example.local", info.DisplayHost)
	}
	if info.Username != "sys" {
		t.Fatalf("expected username %q, got %q", "sys", info.Username)
	}
	if info.AdminUI == nil {
		t.Fatal("expected legacy UI port to derive Admin UI metadata")
	}
	if info.AdminUI.URL != "https://example.local:8443" {
		t.Fatalf("expected Admin UI URL %q, got %q", "https://example.local:8443", info.AdminUI.URL)
	}
	if info.AdminUI.Username != "admin" {
		t.Fatalf("expected Admin UI username %q, got %q", "admin", info.AdminUI.Username)
	}
	if !info.ShellSupported {
		t.Fatal("expected shell support to be preserved")
	}
	if info.SecretsFilePath != deployment.Resolve(secretsFileName) {
		t.Fatalf(
			"expected secrets path %q, got %q",
			deployment.Resolve(secretsFileName),
			info.SecretsFilePath,
		)
	}
}

func TestResolveConnectionInfo_UsesExplicitAdminUIMetadata(t *testing.T) {
	t.Parallel()

	// Given
	deployment := NewDeploymentDir(t.TempDir())
	writeConnectionInfoTestFiles(t, deployment, &DeploymentInfo{
		DeploymentId: "dep-1",
		Connection: &DeploymentConnection{
			Host:   "example.local",
			DBPort: 8563,
			AdminUI: &DeploymentAdminUI{
				URL:                        " https://admin.example.local/ui ",
				Username:                   " admin-user ",
				CertFingerprint:            " admin-fingerprint ",
				InsecureSkipCertValidation: true,
			},
		},
	})

	// When
	info, err := ResolveConnectionInfo(deployment)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.UIPort != 0 {
		t.Fatalf("expected legacy UI port to be optional, got %d", info.UIPort)
	}
	if info.AdminUI == nil {
		t.Fatal("expected Admin UI metadata")
	}
	if info.AdminUI.URL != "https://admin.example.local/ui" {
		t.Fatalf("expected explicit Admin UI URL, got %q", info.AdminUI.URL)
	}
	if info.AdminUI.Username != "admin-user" {
		t.Fatalf("expected explicit Admin UI username, got %q", info.AdminUI.Username)
	}
	if info.AdminUI.CertFingerprint != "admin-fingerprint" {
		t.Fatalf("expected explicit Admin UI fingerprint, got %q", info.AdminUI.CertFingerprint)
	}
	if !info.AdminUI.InsecureSkipCertValidation {
		t.Fatal("expected Admin UI certificate validation metadata to be preserved")
	}
}

func TestResolveConnectionInfo_AllowsMissingAdminUIMetadata(t *testing.T) {
	t.Parallel()

	// Given
	deployment := NewDeploymentDir(t.TempDir())
	writeConnectionInfoTestFiles(t, deployment, &DeploymentInfo{
		DeploymentId: "dep-1",
		Connection: &DeploymentConnection{
			Host:   "example.local",
			DBPort: 8563,
		},
	})

	// When
	info, err := ResolveConnectionInfo(deployment)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.AdminUI != nil {
		t.Fatalf("expected no Admin UI metadata, got %#v", info.AdminUI)
	}
}

func TestResolveConnectionInfo_UsesExplicitConnectionCertFingerprint(t *testing.T) {
	t.Parallel()

	// Given
	deployment := NewDeploymentDir(t.TempDir())
	writeConnectionInfoTestFiles(t, deployment, &DeploymentInfo{
		DeploymentId: "dep-1",
		Connection: &DeploymentConnection{
			Host:            "example.local",
			DBPort:          8563,
			UIPort:          8443,
			CertFingerprint: " explicit-fingerprint ",
		},
		Nodes: map[string]DeploymentNode{
			primaryNodeName: {TlsCert: testNodeTLSCertPEM},
		},
	})

	// When
	info, err := ResolveConnectionInfo(deployment)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.CertFingerprint != "explicit-fingerprint" {
		t.Fatalf("expected explicit fingerprint, got %q", info.CertFingerprint)
	}
}

func TestResolveConnectionInfo_UsesLegacyNodeTLSCertFingerprintFallback(t *testing.T) {
	t.Parallel()

	// Given
	deployment := NewDeploymentDir(t.TempDir())
	writeConnectionInfoTestFiles(t, deployment, &DeploymentInfo{
		DeploymentId: "dep-1",
		Connection: &DeploymentConnection{
			Host:   "example.local",
			DBPort: 8563,
			UIPort: 8443,
		},
		Nodes: map[string]DeploymentNode{
			primaryNodeName: {TlsCert: testNodeTLSCertPEM},
		},
	})

	// When
	info, err := ResolveConnectionInfo(deployment)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.CertFingerprint != testCertSHA256Fingerprint {
		t.Fatalf(
			"expected legacy node fingerprint %q, got %q",
			testCertSHA256Fingerprint,
			info.CertFingerprint,
		)
	}
}

func writeConnectionInfoTestFiles(
	t *testing.T,
	deployment DeploymentDir,
	deploymentInfo *DeploymentInfo,
) {
	t.Helper()

	if err := WriteSecrets(deployment.Root(), &Secrets{DbPassword: "secret"}); err != nil {
		t.Fatalf("failed to write secrets: %v", err)
	}
	if err := WriteDeploymentInfo(deployment.Root(), deploymentInfo); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}
}
