// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/customslc"
	"github.com/exasol/exasol-personal/internal/directorymutex"
)

func TestValidateCustomAlias(t *testing.T) {
	t.Parallel()

	// Given
	for _, testCase := range []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"mypy3", "MYPY3", false},
		{" my_py3 ", "MY_PY3", false},
		{"", "", true},
		{"has space", "", true},
		{"../evil", "", true},
		{"semi;colon", "", true},
		{"123py", "", true},
		{"_foo", "", true},
		{strings.Repeat("a", 129), "", true},
		{strings.Repeat("a", 128), strings.Repeat("A", 128), false},
	} {
		// When
		got, err := validateCustomAlias(testCase.in)
		// Then
		if testCase.wantErr {
			if err == nil {
				t.Fatalf("validateCustomAlias(%q): expected error", testCase.in)
			}

			continue
		}
		if err != nil {
			t.Fatalf("validateCustomAlias(%q): unexpected error %v", testCase.in, err)
		}
		if got != testCase.want {
			t.Fatalf("validateCustomAlias(%q) = %q, want %q", testCase.in, got, testCase.want)
		}
	}
}

func TestValidateCustomSourceRequiresExactlyOne(t *testing.T) {
	t.Parallel()

	if _, err := validateCustomSource(CustomSLCInstallOpts{}); err == nil {
		t.Fatal("expected an error when neither file nor url is set")
	}
	if _, err := validateCustomSource(
		CustomSLCInstallOpts{File: "a.tar.gz", URL: "https://x"},
	); err == nil {
		t.Fatal("expected an error when both file and url are set")
	}
	if got, err := validateCustomSource(CustomSLCInstallOpts{File: "a.tar.gz"}); err != nil ||
		got != "a.tar.gz" {
		t.Fatalf("expected file source, got %q err %v", got, err)
	}
	if got, err := validateCustomSource(CustomSLCInstallOpts{URL: "https://x"}); err != nil ||
		got != "https://x" {
		t.Fatalf("expected url source, got %q err %v", got, err)
	}
}

func TestValidateCustomSourceEnforcesHTTPS(t *testing.T) {
	t.Parallel()

	if _, err := validateCustomSource(
		CustomSLCInstallOpts{URL: "https://example.com/c.tar.gz"},
	); err != nil {
		t.Fatalf("an https URL should be accepted, got %v", err)
	}
	if _, err := validateCustomSource(
		CustomSLCInstallOpts{URL: "http://example.com/c.tar.gz"},
	); err == nil {
		t.Fatal("a plaintext http URL must be rejected")
	}
	if _, err := validateCustomSource(CustomSLCInstallOpts{URL: "https:foo"}); err == nil {
		t.Fatal("a URL without a host must be rejected")
	}
}

func TestOfficialOwnerMatchesAliasCaseInsensitively(t *testing.T) {
	t.Parallel()

	const flavor = "python-3.12"
	installed := []config.InstalledSLC{
		{Flavor: flavor, Aliases: []string{"PYTHON3", "PYTHON312"}},
	}
	if got := officialOwner(installed, "python3"); got != flavor {
		t.Fatalf("expected %s, got %q", flavor, got)
	}
	if got := officialOwner(installed, "JAVA"); got != "" {
		t.Fatalf("expected no owner for JAVA, got %q", got)
	}
}

// checkOfficialAliasNotHeldByCustom is the WF1 mirror rule: an official install must be
// blocked when a custom SLC already owns one of its aliases.
func TestCheckOfficialAliasNotHeldByCustom(t *testing.T) {
	t.Parallel()

	customs := []config.InstalledCustomSLC{{Alias: "PYTHON3", Language: "python"}}

	err := checkOfficialAliasNotHeldByCustom(customs, []string{"PYTHON3", "PYTHON312"})
	if err == nil {
		t.Fatal("expected a collision error when a custom SLC owns the alias")
	}
	if !strings.Contains(err.Error(), "exasol slc custom remove PYTHON3") {
		t.Fatalf("expected the error to guide removal, got %v", err)
	}

	if err := checkOfficialAliasNotHeldByCustom(customs, []string{"JAVA"}); err != nil {
		t.Fatalf("expected no collision for a disjoint alias, got %v", err)
	}
}

func TestCustomSLCDirIsContentAddressed(t *testing.T) {
	t.Parallel()

	const shaA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const shaB = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

	dirA := customSLCDir("MyPy3", shaA)
	if dirA != "mypy3-0123456789abcdef" {
		t.Fatalf("expected lower alias with 16-char digest, got %q", dirA)
	}
	if dirA == customSLCDir("MyPy3", shaB) {
		t.Fatalf("expected distinct digests to yield distinct dirs, both were %q", dirA)
	}
}

func TestCustomSLCOperationsRequireDeploymentLock(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	mutex, err := directorymutex.New(deployment.Root())
	if err != nil {
		t.Fatal(err)
	}

	opts := CustomSLCInstallOpts{Alias: "MYPY3", Language: "python", File: "container.tar.gz"}
	ops := []struct {
		name string
		call func(context.Context) error
	}{
		{"install", func(ctx context.Context) error {
			_, err := InstallCustomSLC(ctx, deployment, opts, nil)

			return err
		}},
		{"update", func(ctx context.Context) error {
			_, err := UpdateCustomSLC(ctx, deployment, opts)

			return err
		}},
		{"remove", func(ctx context.Context) error {
			_, err := RemoveCustomSLC(ctx, deployment, "MYPY3")

			return err
		}},
	}

	for _, operation := range ops {
		held := mutex.WithExclusive(context.Background(), nil, func(any) error {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			// When
			err := operation.call(ctx)
			// Then
			if !errors.Is(err, ErrDeploymentDirectoryLocked) {
				t.Fatalf("%s: expected ErrDeploymentDirectoryLocked, got %v", operation.name, err)
			}

			return nil
		})
		if held != nil {
			t.Fatalf("%s: holding the lock failed: %v", operation.name, held)
		}
	}
}

func TestAliasResolvesTo(t *testing.T) {
	t.Parallel()

	entries := customslc.ParseScriptLanguages(
		"MYPY3=localzmq+protobuf:///a?lang=python#a PYTHON3=builtin_python3",
	)

	if !aliasResolvesTo(entries, "mypy3", "localzmq+protobuf:///a?lang=python#a") {
		t.Fatal("the exact recorded URI must match")
	}
	if aliasResolvesTo(entries, "mypy3", "localzmq+protobuf:///b?lang=python#b") {
		t.Fatal("a stale URI must not be treated as a match")
	}
	if aliasResolvesTo(entries, "absent", "x") {
		t.Fatal("an absent alias must not match")
	}
}

func TestCustomSLCUnchanged(t *testing.T) {
	t.Parallel()

	recorded := config.InstalledCustomSLC{Sha256: "abc", Language: "python"}

	if !customSLCUnchanged(recorded, "abc", customslc.LanguagePython) {
		t.Fatal("same digest and language must be a no-op")
	}
	if customSLCUnchanged(recorded, "abc", customslc.LanguageJava) {
		t.Fatal("a language-only change must not be treated as unchanged")
	}
	if customSLCUnchanged(recorded, "def", customslc.LanguagePython) {
		t.Fatal("a content change must not be treated as unchanged")
	}
}

func TestCarriedDisplacedURI(t *testing.T) {
	t.Parallel()

	if got := carriedDisplacedURI(nil, "builtin_python3"); got != "builtin_python3" {
		t.Fatalf("first install: got %q, want builtin_python3", got)
	}

	previous := &config.InstalledCustomSLC{Alias: "PYTHON3", DisplacedURI: "builtin_python3"}
	if got := carriedDisplacedURI(previous, "localzmq+protobuf:///x"); got != "builtin_python3" {
		t.Fatalf("update: got %q, want the preserved builtin_python3", got)
	}

	if got := carriedDisplacedURI(nil, ""); got != "" {
		t.Fatalf("fresh alias: got %q, want empty", got)
	}
}

func TestCustomSLCDirOfPrefersBucketPath(t *testing.T) {
	t.Parallel()

	fromPath := customSLCDirOf(config.InstalledCustomSLC{
		Alias:      "MYPY3",
		BucketPath: "bfsdefault/default/mypy3-0123456789abcdef",
	})
	if fromPath != "mypy3-0123456789abcdef" {
		t.Fatalf("expected the BucketPath base, got %q", fromPath)
	}

	fallback := customSLCDirOf(config.InstalledCustomSLC{Alias: "MYPY3"})
	if fallback != "mypy3" {
		t.Fatalf("expected the lower alias fallback, got %q", fallback)
	}
}

func TestUpsertInstalledCustomSLCReplacesAndSorts(t *testing.T) {
	t.Parallel()

	existing := []config.InstalledCustomSLC{{Alias: "MYPY3", Sha256: "old"}}
	withR := upsertInstalledCustomSLC(
		existing,
		config.InstalledCustomSLC{Alias: "MYR", Sha256: "r"},
	)
	if len(withR) != 2 || withR[0].Alias != "MYPY3" || withR[1].Alias != "MYR" {
		t.Fatalf("expected sorted [MYPY3 MYR], got %+v", withR)
	}

	replaced := upsertInstalledCustomSLC(
		withR,
		config.InstalledCustomSLC{Alias: "MYPY3", Sha256: "new"},
	)
	if len(replaced) != 2 {
		t.Fatalf("expected replace to keep length 2, got %d", len(replaced))
	}
	if idx := findInstalledCustomSLC(replaced, "mypy3"); idx < 0 || replaced[idx].Sha256 != "new" {
		t.Fatalf("expected MYPY3 replaced with new digest, got %+v", replaced)
	}
}

// The start path re-applies image mounts from InstalledSLCs only; a custom SLC in the
// separate list must never leak into the runner's --slc flags.
func TestLocalRunnerSlcArgsIgnoresCustomSLCs(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{
		DeploymentVersion: "0.0.0",
		InstalledSLCs: []config.InstalledSLC{
			{Flavor: "python-3.12", Image: "img:py", Target: "/exa/slc/py"},
		},
		InstalledCustomSLCs: []config.InstalledCustomSLC{
			{Alias: "MYPY3", Language: "python", BucketPath: "bfsdefault/default/mypy3"},
		},
	}
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		t.Fatalf("failed to write state: %v", err)
	}

	// When
	args, err := localRunnerSlcArgs(deployment)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"--slc", "img:py=/exa/slc/py"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("expected only the official mount arg, got %v", args)
	}
	for _, arg := range args {
		if strings.Contains(arg, "mypy3") || strings.Contains(arg, "bfsdefault") {
			t.Fatalf("custom SLC leaked into runner args: %v", args)
		}
	}
}

func TestConfirmOfficialAliasReuseBlocksInstalledOfficial(t *testing.T) {
	t.Parallel()

	// Given
	state := &config.ExasolPersonalState{
		InstalledSLCs: []config.InstalledSLC{
			{Flavor: "python-3.12", Aliases: []string{"PYTHON3", "PYTHON312"}},
		},
	}
	confirm := func(string) (bool, error) {
		t.Fatal("confirm must not be called when an official SLC owns the alias")

		return false, nil
	}

	// When
	err := confirmOfficialAliasReuse(config.NewDeploymentDir(""), state, nil, "PYTHON3", confirm)
	// Then
	if err == nil || !strings.Contains(err.Error(), "python-3.12") {
		t.Fatalf("expected a block naming the official flavor, got %v", err)
	}
}

func TestConfirmOfficialAliasReuseConfirmsBuiltinOverride(t *testing.T) {
	t.Parallel()

	entries := []customslc.AliasEntry{{Alias: "PYTHON3", URI: "builtin_python3"}}

	declined := confirmOfficialAliasReuse(
		config.NewDeploymentDir(""), &config.ExasolPersonalState{}, entries, "PYTHON3",
		func(string) (bool, error) { return false, nil },
	)
	if !errors.Is(declined, ErrSLCOperationCancelled) {
		t.Fatalf("expected cancellation when the user declines, got %v", declined)
	}

	accepted := confirmOfficialAliasReuse(
		config.NewDeploymentDir(""), &config.ExasolPersonalState{}, entries, "PYTHON3",
		func(string) (bool, error) { return true, nil },
	)
	if accepted != nil {
		t.Fatalf("expected success when the user confirms, got %v", accepted)
	}
}

func TestConfirmOfficialAliasReuseAllowsFreeAlias(t *testing.T) {
	t.Parallel()

	// When
	err := confirmOfficialAliasReuse(
		config.NewDeploymentDir(""), &config.ExasolPersonalState{}, nil, "MYPY3",
		func(string) (bool, error) {
			t.Fatal("confirm must not be called for a non-official alias")

			return false, nil
		},
	)
	// Then
	if err != nil {
		t.Fatalf("expected nil for a free alias, got %v", err)
	}
}

func TestHashFileMatchesSHA256(t *testing.T) {
	t.Parallel()

	// Given
	path := filepath.Join(t.TempDir(), "container.bin")
	content := []byte("hello slc")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	// When
	got, err := hashFile(path)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("%x", sha256.Sum256(content)); got != want {
		t.Fatalf("hashFile = %s, want %s", got, want)
	}
}

func TestRejectNonHTTPSRedirect(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	httpsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := rejectNonHTTPSRedirect(httpsReq, nil); err != nil {
		t.Fatalf("an https redirect should be allowed: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := rejectNonHTTPSRedirect(httpReq, nil); err == nil {
		t.Fatal("a redirect to a non-https URL must be rejected")
	}
}

func TestAcquireCustomTarballFromURLEvictsAfterCleanup(t *testing.T) {
	t.Parallel()

	body := []byte("tarball-bytes")
	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write(body)
		}),
	)
	defer server.Close()

	tarball, err := acquireCustomTarball(
		context.Background(),
		CustomSLCInstallOpts{URL: server.URL},
	)
	if err != nil {
		t.Fatal(err)
	}

	if want := fmt.Sprintf("%x", sha256.Sum256(body)); tarball.sha256 != want {
		t.Fatalf("digest = %s, want %s", tarball.sha256, want)
	}
	got, err := os.ReadFile(tarball.path)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("downloaded content mismatch: %q err %v", got, err)
	}

	tarball.cleanup()
	if _, err := os.Stat(tarball.path); !os.IsNotExist(err) {
		t.Fatalf("expected the downloaded temp file removed after cleanup, got %v", err)
	}
}

func TestAcquireCustomTarballURLFailsOnNon200(t *testing.T) {
	t.Parallel()

	// Given
	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusNotFound)
		}),
	)
	defer server.Close()

	// When
	_, err := acquireCustomTarball(
		context.Background(), CustomSLCInstallOpts{URL: server.URL},
	)
	// Then
	if err == nil {
		t.Fatal("expected an error on a non-200 response")
	}
}

func TestAcquireCustomTarballFromFileIsNeverDeleted(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "c.tar.gz")
	content := []byte("file-bytes")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	tarball, err := acquireCustomTarball(context.Background(), CustomSLCInstallOpts{File: path})
	if err != nil {
		t.Fatal(err)
	}
	if tarball.path != path {
		t.Fatalf("expected the file used in place, got %s", tarball.path)
	}
	if want := fmt.Sprintf("%x", sha256.Sum256(content)); tarball.sha256 != want {
		t.Fatalf("digest = %s, want %s", tarball.sha256, want)
	}

	tarball.cleanup()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cleanup must not delete a user --file: %v", err)
	}
}
