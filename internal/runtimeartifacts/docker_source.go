// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type DockerSource struct{}

func (*DockerSource) CanFetch(url string) bool {
	_, _, err := DockerUrlDownloadTarget(url)
	return err == nil
}

func (*DockerSource) Fetch(ctx context.Context, url, dstPath string) (string, error) {
	tmpDownloadDir, err := os.MkdirTemp(filepath.Dir(dstPath), "download-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDownloadDir)

	downloadUrl, targetTag, err := DockerUrlDownloadTarget(url)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx,
		"skopeo",
		"copy",
		"--preserve-digests",
		downloadUrl,
		// Specify the target tag here so it's saved as part of the image
		// and will be loaded by podman.
		"oci:"+tmpDownloadDir+":"+targetTag,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	if err := reproducibleTar(tmpDownloadDir, dstPath); err != nil {
		// Ignore further errors
		_ = os.Remove(dstPath)
		return "", err
	}

	return "", nil
}

// Returns the url to copy from using skopeo and the target tag to encode in the
// oci archive.
// nolint: revive
func DockerUrlDownloadTarget(url string) (string, string, error) {
	if strings.HasPrefix(url, "docker://") {
		re := regexp.MustCompile(`^(docker://)([^:@]+)(|:[^@]+)(|@sha256:[a-zA-Z0-9]{64})$`)
		matches := re.FindStringSubmatch(url)
		if matches != nil {
			scheme, name, tag, digest := matches[1], matches[2], matches[3], matches[4]
			return scheme + name + digest, name + tag, nil
		}
	} else if strings.HasPrefix(url, "oci:") || strings.HasPrefix(url, "oci-archive:") {
		re := regexp.MustCompile(`^(oci:|oci-archive:)([^:@]+)(|:[^@]+)$`)
		matches := re.FindStringSubmatch(url)
		if matches != nil {
			// digest isn't supported for oci/oci-archive
			scheme, name, tag := matches[1], matches[2], matches[3]
			return scheme + name + tag, name + tag, nil
		}
	}

	return "", "", fmt.Errorf("invalid url: %s", url)
}

// Based on tar.Writer.AddFS(fs.FS) but sets all metadata to 0 for deterministic
// output. fs.WalkDir uses lexicographical order which is also deterministic.
func reproducibleTar(dir, dst string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	fsys := root.FS()

	outFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer outFile.Close()

	tarWriter := tar.NewWriter(outFile)
	defer tarWriter.Close()

	return fs.WalkDir(fsys, ".", func(name string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		info, err := dirEntry.Info()
		if err != nil {
			return err
		}
		typ := dirEntry.Type()
		if !typ.IsRegular() && !typ.IsDir() {
			return errors.New("tar: cannot add non-regular file")
		}
		const fixedPerms = int64(0o0600)
		header := &tar.Header{
			Name:    name,
			Mode:    fixedPerms,
			Size:    info.Size(),
			ModTime: time.Unix(0, 0),
		}
		if dirEntry.IsDir() {
			header.Name += "/"
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if !typ.IsRegular() {
			return nil
		}
		f, err := fsys.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tarWriter, f)

		return err
	})
}
