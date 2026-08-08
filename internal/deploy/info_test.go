// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestDisplayHostname_NilDetailsReturnsEmpty(t *testing.T) {
	t.Parallel()

	// Then
	if got := (*ConnectionDetails)(nil).DisplayHostname(); got != "" {
		t.Fatalf("expected an empty hostname for nil details, got %q", got)
	}
}

func TestDisplayHostname_PrefersDisplayHostOverHostnameAndPublicIp(t *testing.T) {
	t.Parallel()

	// Given
	details := &ConnectionDetails{
		DisplayHost: "display.example.test",
		Hostname:    "internal.example.test",
		PublicIp:    "203.0.113.1",
	}

	// Then
	if got := details.DisplayHostname(); got != "display.example.test" {
		t.Fatalf("expected DisplayHost to win, got %q", got)
	}
}

func TestDisplayHostname_FallsBackToHostnameWhenDisplayHostIsBlank(t *testing.T) {
	t.Parallel()

	// Given
	details := &ConnectionDetails{Hostname: "internal.example.test", PublicIp: "203.0.113.1"}

	// Then
	if got := details.DisplayHostname(); got != "internal.example.test" {
		t.Fatalf("expected Hostname to win over PublicIp, got %q", got)
	}
}

func TestDisplayHostname_FallsBackToPublicIpWhenHostnameIsBlank(t *testing.T) {
	t.Parallel()

	// Given
	details := &ConnectionDetails{PublicIp: "203.0.113.1"}

	// Then
	if got := details.DisplayHostname(); got != "203.0.113.1" {
		t.Fatalf("expected PublicIp to be used as a last resort, got %q", got)
	}
}

func TestDisplayHostname_AllFieldsBlankReturnsEmpty(t *testing.T) {
	t.Parallel()

	// Then
	if got := (&ConnectionDetails{}).DisplayHostname(); got != "" {
		t.Fatalf("expected an empty hostname when nothing is set, got %q", got)
	}
}

func TestIsLocalBackend_NilDetailsIsFalse(t *testing.T) {
	t.Parallel()

	// Then
	if (*ConnectionDetails)(nil).IsLocalBackend() {
		t.Fatal("expected nil details to not be a local backend")
	}
}

func TestIsLocalBackend_MatchesLocalBackendType(t *testing.T) {
	t.Parallel()

	// Then
	if !(&ConnectionDetails{Backend: localDeploymentBackend}).IsLocalBackend() {
		t.Fatal("expected the local backend type to be recognized")
	}
	// Then
	if (&ConnectionDetails{Backend: "tofu"}).IsLocalBackend() {
		t.Fatal("expected a non-local backend type to not be recognized as local")
	}
}

func TestHasAdminUI_RequiresNonEmptyURL(t *testing.T) {
	t.Parallel()

	// Then
	if (*ConnectionDetails)(nil).HasAdminUI() {
		t.Fatal("expected nil details to not have an admin UI")
	}
	// Then
	if (&ConnectionDetails{}).HasAdminUI() {
		t.Fatal("expected no admin UI when unset")
	}
	// Then
	if (&ConnectionDetails{AdminUI: &config.DeploymentAdminUI{}}).HasAdminUI() {
		t.Fatal("expected no admin UI when the URL is blank")
	}

	// Given
	details := &ConnectionDetails{AdminUI: &config.DeploymentAdminUI{URL: "https://admin.test"}}

	// Then
	if !details.HasAdminUI() {
		t.Fatal("expected an admin UI when the URL is set")
	}
}

func TestReadConnectionDetails_ResolvesFullConnectionInfo(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:      localDeploymentBackend,
		DeploymentId: "test-deployment",
		Connection: &config.DeploymentConnection{
			Host:   "127.0.0.1",
			DBPort: 8563,
		},
	}); err != nil {
		t.Fatalf("failed to seed deployment info: %v", err)
	}
	if err := config.WriteSecrets(deployment.Root(), &config.Secrets{
		DbPassword:      "x",
		AdminUiPassword: "y",
	}); err != nil {
		t.Fatalf("failed to seed secrets: %v", err)
	}

	// When
	details, err := readConnectionDetails(deployment)
	// Then
	if err != nil {
		t.Fatalf("expected connection details to resolve, got %v", err)
	}
	if details.Backend != localDeploymentBackend {
		t.Fatalf("expected the backend to be carried over, got %q", details.Backend)
	}
	if details.Hostname != "127.0.0.1" || details.DBPort != 8563 {
		t.Fatalf("expected the resolved host/port, got %+v", details)
	}
	if !details.AdminUISecured {
		t.Fatal("expected AdminUISecured to be true when an admin UI password is set")
	}
}

func TestReadConnectionDetails_MissingDeploymentInfoReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())

	// Then
	if _, err := readConnectionDetails(deployment); err == nil {
		t.Fatal("expected an error when no deployment info has been persisted")
	}
}

func TestReadConnectionDetails_MissingConnectionReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		DeploymentId: "test-deployment",
	}); err != nil {
		t.Fatalf("failed to seed deployment info: %v", err)
	}

	// Then
	if _, err := readConnectionDetails(deployment); err == nil {
		t.Fatal("expected an error when the deployment has no connection details")
	}
}

func TestGetDeploymentInfoReportInitializedIncludesOverviewAndPresets(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	initDeploymentForInfoTest(t, deployment)

	// When
	report, err := GetDeploymentInfoReport(context.Background(), deployment)
	// Then
	if err != nil {
		t.Fatalf("expected initialized deployment info report, got error: %v", err)
	}
	if report.DeploymentDir != deployment.Root() {
		t.Fatalf("expected deployment dir %q, got %q", deployment.Root(), report.DeploymentDir)
	}
	if report.DeploymentID == "" {
		t.Fatal("expected deployment ID")
	}
	if report.DeploymentState != StatusInitialized {
		t.Fatalf("expected state %q, got %q", StatusInitialized, report.DeploymentState)
	}
	if report.Presets == nil {
		t.Fatal("expected preset summary")
	}
}

func TestDeploymentInfoReportOperationInProgressOmitsPartialDeploymentDetails(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	initDeploymentForInfoTest(t, deployment)
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		DeploymentId:    "test-deployment",
		ClusterSize:     1,
		ClusterState:    StatusRunning,
		DeploymentState: StatusRunning,
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	// When
	report, err := deploymentInfoReportFromState(
		deployment,
		StatusOperationInProgress,
	)
	// Then
	if err != nil {
		t.Fatalf("expected operation in progress report, got error: %v", err)
	}
	if report.DeploymentState != StatusOperationInProgress {
		t.Fatalf("expected state %q, got %q", StatusOperationInProgress, report.DeploymentState)
	}
	if report.Presets == nil {
		t.Fatal("expected stable preset summary")
	}
	if report.Deployment != nil {
		t.Fatalf("expected partial deployment attributes to be omitted, got %#v", report.Deployment)
	}
	if report.Connection != nil {
		t.Fatalf("expected connection details to be omitted, got %#v", report.Connection)
	}
}

func TestDeploymentInfoReportOperationInProgressToleratesMissingIdentity(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir() + "/missing")

	// When
	report, err := deploymentInfoReportFromState(
		deployment,
		StatusOperationInProgress,
	)
	// Then
	if err != nil {
		t.Fatalf("expected operation in progress report without identity files, got error: %v", err)
	}
	if report.DeploymentDir != deployment.Root() {
		t.Fatalf("expected deployment dir %q, got %q", deployment.Root(), report.DeploymentDir)
	}
	if report.DeploymentState != StatusOperationInProgress {
		t.Fatalf("expected state %q, got %q", StatusOperationInProgress, report.DeploymentState)
	}
	if report.Presets != nil {
		t.Fatalf("expected presets to remain optional, got %#v", report.Presets)
	}
	if report.Deployment != nil {
		t.Fatalf("expected deployment attributes to remain optional, got %#v", report.Deployment)
	}
	if report.Connection != nil {
		t.Fatalf("expected connection details to remain optional, got %#v", report.Connection)
	}
}

func TestGetDeploymentInfoReportNotInitializedIncludesStructuredState(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())

	// When
	report, err := GetDeploymentInfoReport(context.Background(), deployment)
	// Then
	if err != nil {
		t.Fatalf("expected missing deployment info to resolve without error, got: %v", err)
	}
	if report.DeploymentState != StatusNotInitialized {
		t.Fatalf("expected state %q, got %q", StatusNotInitialized, report.DeploymentState)
	}
	if report.DeploymentDir != deployment.Root() {
		t.Fatalf(
			"expected deployment dir %q, got %q",
			deployment.Root(),
			report.DeploymentDir,
		)
	}
	if report.Presets != nil {
		t.Fatalf("expected no presets for not-initialized deployment, got %#v", report.Presets)
	}
	if report.Deployment != nil {
		t.Fatalf("expected no deployment attributes, got %#v", report.Deployment)
	}
	if report.Connection != nil {
		t.Fatalf("expected no connection details, got %#v", report.Connection)
	}
}

func TestGetDeploymentInfoReportNotInitializedHandlesMissingDirectory(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir() + "/missing")

	// When
	report, err := GetDeploymentInfoReport(context.Background(), deployment)
	// Then
	if err != nil {
		t.Fatalf("expected missing deployment directory to resolve without error, got: %v", err)
	}
	if report.DeploymentState != StatusNotInitialized {
		t.Fatalf("expected state %q, got %q", StatusNotInitialized, report.DeploymentState)
	}
	if report.DeploymentDir != deployment.Root() {
		t.Fatalf(
			"expected deployment dir %q, got %q",
			deployment.Root(),
			report.DeploymentDir,
		)
	}
}
