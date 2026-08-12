// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/docker/reference"
	ociarchive "go.podman.io/image/v5/oci/archive"
	ocilayout "go.podman.io/image/v5/oci/layout"
	"go.podman.io/image/v5/signature"
	"go.podman.io/image/v5/transports"
	"go.podman.io/image/v5/types"
)

type DockerSource struct{}

func (*DockerSource) CanFetch(url string) bool {
	return strings.HasPrefix(url, "docker://") || strings.HasPrefix(url, "oci:") ||
		strings.HasPrefix(url, "oci-archive:")
}

func (*DockerSource) Fetch(ctx context.Context, url, dstPath string) (string, error) {
	parsedRef, err := parseImageReference(url)
	if err != nil {
		return "", err
	}
	tmpDir, err := os.MkdirTemp(filepath.Dir(dstPath), "oci-archive-*")
	if err != nil {
		return "", fmt.Errorf("create temporary OCI archive directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	rawArchivePath := filepath.Join(tmpDir, "image.tar")
	destRef, err := ociarchive.NewReference(rawArchivePath, parsedRef.destinationImage)
	if err != nil {
		return "", fmt.Errorf("create OCI archive destination: %w", err)
	}
	policyContext, err := newInsecurePolicyContext()
	if err != nil {
		return "", err
	}

	timestamp := time.Unix(0, 0).UTC()
	_, copyErr := copy.Image(ctx, policyContext, destRef, parsedRef.source, &copy.Options{
		DestinationTimestamp: &timestamp,
		PreserveDigests:      true,
	})
	destroyErr := policyContext.Destroy()
	if err := errors.Join(copyErr, destroyErr); err != nil {
		_ = os.Remove(dstPath)

		return "", fmt.Errorf("copy image to OCI archive: %w", err)
	}
	if err := repackTarDeterministically(rawArchivePath, dstPath); err != nil {
		_ = os.Remove(dstPath)

		return "", fmt.Errorf("repack OCI archive deterministically: %w", err)
	}

	return "", nil
}

func newInsecurePolicyContext() (*signature.PolicyContext, error) {
	policy := &signature.Policy{
		Default: signature.PolicyRequirements{signature.NewPRInsecureAcceptAnything()},
	}
	policyContext, err := signature.NewPolicyContext(policy)
	if err != nil {
		return nil, fmt.Errorf("create image policy context: %w", err)
	}

	return policyContext, nil
}

type parsedImageReference struct {
	source           types.ImageReference
	destinationImage string
}

func parseImageReference(imageName string) (parsedImageReference, error) {
	transportName, referenceName, found := strings.Cut(imageName, ":")
	if !found {
		return parsedImageReference{}, fmt.Errorf(
			"image reference %q is missing a transport", imageName,
		)
	}
	transport := transports.Get(transportName)
	if transport == nil {
		return parsedImageReference{}, fmt.Errorf("unsupported image transport in %q", imageName)
	}

	switch transport.Name() {
	case docker.Transport.Name():
		return parseDockerImageReference(imageName)
	case ocilayout.Transport.Name(), ociarchive.Transport.Name():
		parsedReference, err := transport.ParseReference(referenceName)
		if err != nil {
			return parsedImageReference{}, fmt.Errorf(
				"parse image reference %q: %w", imageName, err,
			)
		}
		destName, err := archiveImageName(parsedReference)
		if err != nil {
			return parsedImageReference{}, err
		}

		return parsedImageReference{
			source:           parsedReference,
			destinationImage: destName,
		}, nil
	default:
		return parsedImageReference{}, fmt.Errorf(
			"unsupported image transport %q", transport.Name(),
		)
	}
}

func parseDockerImageReference(imageName string) (parsedImageReference, error) {
	referenceName, found := strings.CutPrefix(imageName, docker.Transport.Name()+"://")
	if !found {
		return parsedImageReference{}, fmt.Errorf("invalid Docker image reference %q", imageName)
	}
	originalReference, err := reference.ParseNormalizedNamed(referenceName)
	if err != nil {
		return parsedImageReference{}, fmt.Errorf(
			"parse Docker image reference %q: %w", imageName, err,
		)
	}
	sourceNamed, err := reference.ParseDockerRef(referenceName)
	if err != nil {
		return parsedImageReference{}, fmt.Errorf(
			"parse Docker source reference %q: %w", imageName, err,
		)
	}
	source, err := docker.NewReference(sourceNamed)
	if err != nil {
		return parsedImageReference{}, fmt.Errorf(
			"create Docker source reference %q: %w", imageName, err,
		)
	}

	destinationNamed := reference.TrimNamed(originalReference)
	if tagged, ok := originalReference.(reference.NamedTagged); ok {
		destinationTagged, err := reference.WithTag(destinationNamed, tagged.Tag())
		if err != nil {
			return parsedImageReference{}, fmt.Errorf(
				"create Docker destination reference %q: %w", imageName, err,
			)
		}
		destinationNamed = destinationTagged
	}

	return parsedImageReference{
		source:           source,
		destinationImage: destinationNamed.String(),
	}, nil
}

func archiveImageName(source types.ImageReference) (string, error) {
	var (
		descriptor imgspecv1.Descriptor
		err        error
	)
	switch source.Transport().Name() {
	case ocilayout.Transport.Name():
		descriptor, err = ocilayout.LoadManifestDescriptor(source)
	case ociarchive.Transport.Name():
		descriptor, err = ociarchive.LoadManifestDescriptorWithContext(nil, source)
	default:
		return "", fmt.Errorf(
			"cannot determine destination image for transport %q",
			source.Transport().Name(),
		)
	}
	if err != nil {
		return "", fmt.Errorf("load source image descriptor: %w", err)
	}

	return descriptor.Annotations[imgspecv1.AnnotationRefName], nil
}
