// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package winget wraps the Windows Package Manager CLI to install a
// container engine on behalf of the local runtime, plus the registry PATH
// refresh needed to make the just-installed engine findable in the running
// launcher process.
//
// The command shape and flag choices are documented on Install.
// The rationale is intentionally captured in code comments rather than
// external docs because each flag decision reflects a specific failure
// mode observed in practice (see PR.md).
package winget

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Package is a winget package identifier.
type Package string

const (
	// PodmanPackage is RedHat's official podman-for-windows distribution.
	PodmanPackage Package = "RedHat.Podman"
	// DockerDesktopPackage is Docker Inc.'s official Docker Desktop.
	DockerDesktopPackage Package = "Docker.DockerDesktop"
)

// binary is the winget executable name. Overridable so tests can stage a
// shim on PATH under a stable name.
var binary = "winget"

// ErrWingetNotFound is returned when the winget binary is not on PATH.
// Callers surface a specific "install the App Installer / update Windows"
// message rather than the generic exec failure.
var ErrWingetNotFound = errors.New("winget (Windows Package Manager) is not available on PATH")

// pathAdditionsOverrideEnv is the environment variable a test can
// set to bypass the real Windows registry read in RefreshPath.
// Production callers do not set it.
//
// Value semantics: an OS-native PATH string (';'-separated on Windows,
// ':'-separated elsewhere) whose entries RefreshPath prepends to the
// current process's PATH so a subsequent LookPath resolves the
// newly-installed binary.
const pathAdditionsOverrideEnv = "EXASOL_WINGET_TEST_PATH_ADDITIONS"

// InstallArgs returns the argv (without the leading "winget" executable
// name) that Install invokes. Kept exposed so callers can render the exact
// command they will run before invoking it.
func InstallArgs(pkg Package) []string {
	return []string{
		"install",
		"--exact", "--id", string(pkg),
		"--source", "winget",
		"--accept-source-agreements",
		"--accept-package-agreements",
	}
}

// LookupWinget returns ErrWingetNotFound if winget is not on PATH, and
// nil otherwise. Callers use it to give a specific error before falling
// through into the generic Install failure path.
func LookupWinget() error {
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf("%w: %w", ErrWingetNotFound, err)
	}

	return nil
}

// Install runs "winget install --exact --id <package>" with the flag set
// documented below and streams winget's own output to out
// and outErr so the user can see download progress. Blocks until winget
// exits.
//
// Flag choices, each motivated by a specific failure mode:
//
//   - --exact --id <package>: exact-match on the official vendor
//     package rather than substring search, so a compromised or
//     name-shadowing package in the source cannot be substituted.
//
//   - --source winget: pins the search to the official community source.
//     Without this pin, winget falls back to the "msstore" source when
//     its winget-source refresh fails or is stale, and msstore then
//     blocks on an interactive geographic-region prompt that even
//     --accept-source-agreements does not silence.
//
//   - --accept-source-agreements, --accept-package-agreements: agrees
//     to the source and package licence prompts. The user consents by
//     invoking a launcher command that triggers install.
//
//   - Scope is intentionally NOT set. RedHat.Podman declares only a
//     machine-scope installer; passing --scope user filters the
//     installer list to zero candidates and winget aborts with
//     APPINSTALLER_CLI_ERROR_NO_APPLICABLE_INSTALLER (exit 0x8A150010).
//     Without --scope, winget picks based on process elevation: an
//     elevated shell installs at machine scope silently; an
//     unelevated shell triggers a UAC prompt.
//
//   - --silent / --disable-interactivity intentionally NOT passed:
//     winget's progress output is the user's only signal that a
//     multi-minute download is proceeding, and UAC must remain
//     available to end users who did not launch from an elevated shell.
func Install(ctx context.Context, pkg Package, out, outErr io.Writer) error {
	if err := LookupWinget(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, binary, InstallArgs(pkg)...)
	cmd.Stdout = out
	cmd.Stderr = outErr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("winget install %s failed: %w", pkg, err)
	}

	return nil
}

// RefreshPath refreshes the current process's PATH with the entries the
// Windows registry lists after a winget install, so a subsequent
// exec.LookPath in the same launcher process resolves the newly-installed
// binary without waiting for the user to open a new shell.
//
// Windows sets a process's PATH once at start-up and does not re-read
// the registry when installers add entries to it. Callers should invoke
// this both BEFORE deciding to install (to recover from an earlier
// install whose PATH the current process missed) and AFTER installing.
//
// Entries already on the current process's PATH are preserved and not
// duplicated; new entries are prepended so the installed binary resolves before any
// conflicting shim earlier in PATH.
func RefreshPath(ctx context.Context) error {
	additions, err := pathAdditions(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(additions) == "" {
		return nil
	}
	merged := mergePATH(os.Getenv("PATH"), additions)

	return os.Setenv("PATH", merged)
}

// pathAdditions returns the PATH entries to add to the current process's
// PATH so newly installed binaries become findable. On Windows this reads the
// merged machine- + user-scope PATH from the registry via PowerShell.
// On non-Windows the override env is required (test-only).
func pathAdditions(ctx context.Context) (string, error) {
	if override, ok := os.LookupEnv(pathAdditionsOverrideEnv); ok {
		return override, nil
	}
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf(
			"winget.RefreshPath: non-windows platform requires %s to be set (test-only)",
			pathAdditionsOverrideEnv,
		)
	}

	return readWindowsRegistryPATH(ctx)
}

// readWindowsRegistryPATH concatenates the machine- and user-scope Path
// values from the Windows registry into a single OS-PATH string using
// PowerShell. If a scope is empty it is skipped; if both are empty the
// result is empty and RefreshPath no-ops.
func readWindowsRegistryPATH(ctx context.Context) (string, error) {
	// GetEnvironmentVariable expands REG_EXPAND_SZ automatically.
	const script = `$m = [Environment]::GetEnvironmentVariable("Path", "Machine")
$u = [Environment]::GetEnvironmentVariable("Path", "User")
if ($m -and $u) { "$m;$u" } elseif ($m) { $m } elseif ($u) { $u } else { "" }`
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile",
		"-ExecutionPolicy", "Bypass", "-Command", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf(
			"powershell read of registry PATH failed (%w): %s",
			err, strings.TrimSpace(stderr.String()),
		)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// mergePATH returns currentPATH with any entries in additional that are
// not already present prepended, using the OS path separator. Comparison
// is case-insensitive so Windows entries differing only in case do not
// produce duplicates. Empty and whitespace-only entries are dropped.
func mergePATH(currentPATH, additional string) string {
	sep := string(os.PathListSeparator)
	have := map[string]struct{}{}
	for entry := range strings.SplitSeq(currentPATH, sep) {
		if trimmed := strings.TrimSpace(entry); trimmed != "" {
			have[strings.ToLower(trimmed)] = struct{}{}
		}
	}
	additionalEntries := strings.Split(additional, sep)
	toAdd := make([]string, 0, len(additionalEntries))
	for _, entry := range additionalEntries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, dup := have[key]; dup {
			continue
		}
		have[key] = struct{}{}
		toAdd = append(toAdd, trimmed)
	}
	if len(toAdd) == 0 {
		return currentPATH
	}
	if currentPATH == "" {
		return strings.Join(toAdd, sep)
	}

	return strings.Join(toAdd, sep) + sep + currentPATH
}
