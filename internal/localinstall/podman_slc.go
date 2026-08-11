// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	slcImportLabel        = "com.exasol.slc.imported=true"
	officialSLCRepository = "exasol/script-language-container"
	slcStatusPresent      = "present"
	slcStatusPulled       = "pulled"
	slcStatusImported     = "imported"
	slcStatusMissing      = "package-missing"
	slcStatusImportFailed = "import-failed"
	slcStatusDirMode      = 0o750
	slcStatusFileMode     = 0o640
)

type slcImageStatus struct {
	Image string `json:"image"`
	State string `json:"state"`
}

//nolint:tagliatelle // JSON key mirrors the SLC domain abbreviation.
type slcStatusReport struct {
	SLC []slcImageStatus `json:"slc"`
}

func (install *PodmanInstall) materializeSLCs(
	ctx context.Context,
	out, outErr io.Writer,
	containerName string,
	slcs []SLCConfig,
) ([]SLCConfig, error) {
	if slcs == nil {
		if err := os.Remove(install.slcStatusPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to remove stale SLC status report: %w", err)
		}

		return nil, nil
	}
	if strings.TrimSpace(install.slcStatusPath) == "" {
		return nil, errors.New("SLC status path is required for SLC-aware startup")
	}
	if strings.TrimSpace(install.slcStagingDir) == "" {
		return nil, errors.New("SLC staging directory is required for SLC-aware startup")
	}

	available := make([]SLCConfig, 0, len(slcs))
	statuses := make([]slcImageStatus, 0, len(slcs))
	for _, slc := range slcs {
		isAvailable, state, err := install.materializeSLC(
			ctx, out, outErr, containerName, slc,
		)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, slcImageStatus{Image: slc.Image, State: state})
		if isAvailable {
			available = append(available, slc)
		}
	}
	if err := writeSLCStatusAtomically(install.slcStatusPath, statuses); err != nil {
		return nil, err
	}

	return available, nil
}

func (install *PodmanInstall) materializeSLC(
	ctx context.Context,
	out, outErr io.Writer,
	containerName string,
	slc SLCConfig,
) (bool, string, error) {
	exists, err := install.imageExists(ctx, outErr, slc.Image)
	if err != nil {
		return false, "", install.failureWithDiagnostics(ctx, outErr, containerName, err)
	}
	if exists {
		return true, slcStatusPresent, nil
	}

	if slc.Package == "" {
		if err := install.runCmd(ctx, out, outErr, "podman", "pull", slc.Image); err != nil {
			return false, "", install.failureWithDiagnostics(ctx, outErr, containerName,
				fmt.Errorf("failed to pull SLC image %s: %w", slc.Image, err))
		}

		return true, slcStatusPulled, nil
	}

	packagePath, ok := install.slcPackagePath(slc.Package)
	if !ok {
		return false, slcStatusMissing, nil
	}
	if _, err := os.Stat(packagePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, slcStatusMissing, nil
		}

		return false, "", fmt.Errorf(
			"failed to inspect custom SLC package %s: %w",
			packagePath,
			err,
		)
	}
	if err := install.runCmd(
		ctx,
		out,
		outErr,
		"podman",
		"import",
		"--change",
		"LABEL "+slcImportLabel,
		packagePath,
		slc.Image,
	); err != nil {
		install.writePodmanDiagnostics(ctx, outErr, containerName)

		return false, slcStatusImportFailed, nil
	}

	return true, slcStatusImported, nil
}

func (install *PodmanInstall) imageExists(
	ctx context.Context,
	outErr io.Writer,
	image string,
) (bool, error) {
	err := install.runCmd(ctx, nil, outErr, "podman", "image", "exists", image)
	if err == nil {
		return true, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false, nil
	}

	return false, fmt.Errorf("failed to find SLC image %s: %w", image, err)
}

func (install *PodmanInstall) slcPackagePath(packageName string) (string, bool) {
	if packageName == "" || packageName != filepath.Base(packageName) || packageName == "." {
		return "", false
	}

	return filepath.Join(install.slcStagingDir, packageName), true
}

func writeSLCStatusAtomically(path string, statuses []slcImageStatus) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, slcStatusDirMode); err != nil {
		return fmt.Errorf("failed to create SLC status directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".slc-status-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary SLC status report: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	encoder := json.NewEncoder(temporary)
	if err := encoder.Encode(slcStatusReport{SLC: statuses}); err != nil {
		_ = temporary.Close()

		return fmt.Errorf("failed to encode SLC status report: %w", err)
	}
	if err := temporary.Chmod(slcStatusFileMode); err != nil {
		_ = temporary.Close()

		return fmt.Errorf("failed to set SLC status report permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("failed to close SLC status report: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("failed to replace SLC status report: %w", err)
	}

	return nil
}

func (install *PodmanInstall) pruneUnreferencedSLCImages(
	ctx context.Context,
	outErr io.Writer,
	desiredSLCs []SLCConfig,
) {
	if desiredSLCs == nil {
		return
	}

	desired := make(map[string]struct{}, len(desiredSLCs))
	for _, slc := range desiredSLCs {
		desired[normalizeSLCImageRef(slc.Image)] = struct{}{}
	}

	candidates := make(map[string]struct{})
	allImages, err := install.runCmdOutput(
		ctx, nil, outErr, "podman", "images", "--format", "{{.Repository}}:{{.Tag}}",
	)
	if err != nil {
		writeSLCPruneWarning(outErr, "failed to list Podman images", err)
	} else {
		for image := range strings.FieldsSeq(allImages) {
			if isUntaggedImage(image) || slcImageRepository(image) != officialSLCRepository {
				continue
			}
			candidates[image] = struct{}{}
		}
	}

	importedImages, err := install.runCmdOutput(
		ctx,
		nil,
		outErr,
		"podman",
		"images",
		"--filter",
		"label="+slcImportLabel,
		"--format",
		"{{.Repository}}:{{.Tag}}",
	)
	if err != nil {
		writeSLCPruneWarning(outErr, "failed to list imported SLC images", err)
	} else {
		for image := range strings.FieldsSeq(importedImages) {
			if !isUntaggedImage(image) {
				candidates[image] = struct{}{}
			}
		}
	}

	for image := range candidates {
		if _, referenced := desired[normalizeSLCImageRef(image)]; referenced {
			continue
		}
		if err := install.runCmd(ctx, nil, outErr, "podman", "rmi", image); err != nil {
			writeSLCPruneWarning(outErr, "failed to remove unreferenced SLC image "+image, err)
		}
	}
}

func normalizeSLCImageRef(reference string) string {
	normalized := strings.TrimSpace(reference)
	normalized = strings.TrimPrefix(normalized, "docker.io/")
	normalized = strings.TrimPrefix(normalized, "localhost/")
	lastSlash := strings.LastIndex(normalized, "/")
	lastColon := strings.LastIndex(normalized, ":")
	if lastColon <= lastSlash {
		normalized += ":latest"
	}

	return normalized
}

func slcImageRepository(reference string) string {
	normalized := normalizeSLCImageRef(reference)
	lastSlash := strings.LastIndex(normalized, "/")
	lastColon := strings.LastIndex(normalized, ":")
	if lastColon > lastSlash {
		return normalized[:lastColon]
	}

	return normalized
}

func isUntaggedImage(reference string) bool {
	normalized := strings.TrimSpace(reference)
	return normalized == "" || strings.HasSuffix(normalized, ":<none>") ||
		strings.HasPrefix(normalized, "<none>:")
}

func writeSLCPruneWarning(outErr io.Writer, message string, err error) {
	if outErr != nil {
		_, _ = fmt.Fprintf(outErr, "Warning: %s: %v\n", message, err)
	}
}
