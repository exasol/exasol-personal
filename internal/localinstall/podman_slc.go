// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func (install *NanoInstall) materializeSLCs(
	ctx context.Context,
	out, outErr io.Writer,
	containerName string,
	slcs []SLCConfig,
) ([]SLCConfig, error) {
	if slcs == nil {
		if err := install.environment.RemoveFile(
			ctx, install.slcStatusPath.RuntimePath,
		); err != nil {
			return nil, fmt.Errorf("failed to remove stale SLC status report: %w", err)
		}

		return nil, nil
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
	if err := install.writeSLCStatusAtomically(
		ctx, install.slcStatusPath.RuntimePath, statuses,
	); err != nil {
		return nil, err
	}

	return available, nil
}

func (install *NanoInstall) materializeSLC(
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
		if err := install.client.PullImage(ctx, slc.Image, out, outErr); err != nil {
			return false, "", install.failureWithDiagnostics(ctx, outErr, containerName,
				fmt.Errorf("failed to pull SLC image %s: %w", slc.Image, err))
		}

		return true, slcStatusPulled, nil
	}

	packagePath, ok := install.slcPackagePath(slc.Package)
	if !ok {
		return false, slcStatusMissing, nil
	}
	packageExists, err := install.environment.PathExists(ctx, packagePath.RuntimePath)
	if err != nil {
		return false, "", fmt.Errorf(
			"failed to inspect custom SLC package %s: %w", packagePath.RuntimePath, err,
		)
	}
	if !packageExists {
		return false, slcStatusMissing, nil
	}
	if err := install.client.ImportImage(
		ctx,
		packagePath.RuntimePath,
		slc.Image,
		[]string{slcImportLabel},
		out,
		outErr,
	); err != nil {
		install.writePodmanDiagnostics(ctx, outErr, containerName)

		//nolint:nilerr // Custom SLC imports are unavailable but do not block Nano startup.
		return false, slcStatusImportFailed, nil
	}

	return true, slcStatusImported, nil
}

func (install *NanoInstall) imageExists(
	ctx context.Context,
	outErr io.Writer,
	image string,
) (bool, error) {
	exists, err := install.client.ImageExists(ctx, image, outErr)
	if err != nil {
		return false, fmt.Errorf("failed to find SLC image %s: %w", image, err)
	}

	return exists, nil
}

func (install *NanoInstall) slcPackagePath(packageName string) (RuntimePath, bool) {
	if packageName == "" || packageName != filepath.Base(packageName) || packageName == "." {
		return RuntimePath{}, false
	}

	return RuntimePath{
		HostPath:    filepath.Join(install.slcStagingDir.HostPath, packageName),
		RuntimePath: filepath.Join(install.slcStagingDir.RuntimePath, packageName),
	}, true
}

func (install *NanoInstall) writeSLCStatusAtomically(
	ctx context.Context,
	path string,
	statuses []slcImageStatus,
) error {
	data, err := json.Marshal(slcStatusReport{SLC: statuses})
	if err != nil {
		return fmt.Errorf("failed to encode SLC status report: %w", err)
	}
	data = append(data, '\n')
	if err := install.environment.WriteFileAtomically(
		ctx, path, data, slcStatusDirMode, slcStatusFileMode,
	); err != nil {
		return fmt.Errorf("failed to replace SLC status report: %w", err)
	}

	return nil
}

func (install *NanoInstall) pruneUnreferencedSLCImages(
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
	allImages, err := install.client.Images(ctx, outErr)
	if err != nil {
		writeSLCPruneWarning(outErr, "failed to list container images", err)
	} else {
		for _, image := range allImages {
			if isUntaggedImage(image) || slcImageRepository(image) != officialSLCRepository {
				continue
			}
			candidates[image] = struct{}{}
		}
	}

	importedImages, err := install.client.Images(ctx, outErr, slcImportLabel)
	if err != nil {
		writeSLCPruneWarning(outErr, "failed to list imported SLC images", err)
	} else {
		for _, image := range importedImages {
			if !isUntaggedImage(image) {
				candidates[image] = struct{}{}
			}
		}
	}

	for image := range candidates {
		if _, referenced := desired[normalizeSLCImageRef(image)]; referenced {
			continue
		}
		if err := install.client.RemoveImage(ctx, image, nil, outErr); err != nil {
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
