// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/exasol/exasol-personal/internal/customslc"
	"github.com/exasol/exasol-personal/internal/remote"
	"github.com/pkg/sftp"
)

const vmBucketFSDir = "/var/lib/exa/bucketfs"

func customSLCVMDir(dir string) string {
	return vmBucketFSDir + "/" + customSLCBucketFS + "/" + customSLCBucket + "/" + dir
}

// The caller validates the archive before delivery, so extraction assumes a safe container.
func (b *localBackend) deliverCustomSLC(
	ctx context.Context,
	dir string,
	tarball io.Reader,
) error {
	session, err := b.openSFTPSession()
	if err != nil {
		return err
	}
	defer session.Close()

	// Closing the session unblocks any in-flight SFTP call, so a cancelled context aborts a
	// long extraction promptly.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
		case <-stop:
		}
	}()

	client := session.Client()
	target := customSLCVMDir(dir)
	if err := extractTarOverSFTP(client, tarball, target); err != nil {
		_ = client.RemoveAll(target)

		return fmt.Errorf("failed to unpack the custom SLC into the VM: %w", err)
	}

	return nil
}

func (b *localBackend) removeCustomSLCFiles(_ context.Context, dir string) error {
	session, err := b.openSFTPSession()
	if err != nil {
		return err
	}
	defer session.Close()

	if err := removeAllIfExists(session.Client(), customSLCVMDir(dir)); err != nil {
		return fmt.Errorf("failed to remove the custom SLC files from the VM: %w", err)
	}

	return nil
}

func (b *localBackend) openSFTPSession() (*remote.SFTPSession, error) {
	remoteConn, err := localSSHRemoteUnsafe(b.deployment)
	if err != nil {
		return nil, err
	}
	session, err := remoteConn.OpenSFTP()
	if err != nil {
		return nil, fmt.Errorf("failed to open an sftp session to the VM: %w", err)
	}

	return session, nil
}

// extractTarOverSFTP recreates targetDir first so install and replace are idempotent, and
// re-checks per-entry safety as defense in depth even though the caller validated the archive,
// because this is where the filesystem writes actually happen.
func extractTarOverSFTP(client *sftp.Client, source io.Reader, targetDir string) error {
	if err := removeAllIfExists(client, targetDir); err != nil {
		return err
	}
	if err := client.MkdirAll(targetDir); err != nil {
		return err
	}

	reader, finish, err := customslc.OpenTar(source)
	if err != nil {
		return err
	}
	checker := customslc.NewEntryChecker()
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if err := checker.Check(header); err != nil {
			return err
		}
		if err := writeTarEntry(client, reader, header, targetDir); err != nil {
			return err
		}
	}

	return finish()
}

func writeTarEntry(
	client *sftp.Client,
	archive io.Reader,
	header *tar.Header,
	targetDir string,
) error {
	dest := path.Join(targetDir, path.Clean(header.Name))

	switch header.Typeflag {
	case tar.TypeDir:
		return client.MkdirAll(dest)
	case tar.TypeReg:
		return writeTarFile(client, archive, dest, header.FileInfo().Mode().Perm())
	case tar.TypeSymlink:
		if err := client.MkdirAll(path.Dir(dest)); err != nil {
			return err
		}

		return client.Symlink(header.Linkname, dest)
	case tar.TypeLink:
		if err := client.MkdirAll(path.Dir(dest)); err != nil {
			return err
		}

		return client.Link(path.Join(targetDir, path.Clean(header.Linkname)), dest)
	default:
		// Character/block/fifo entries never appear in an SLC container; skipping them keeps
		// extraction total without materializing device nodes on the remote.
		return nil
	}
}

func writeTarFile(
	client *sftp.Client,
	source io.Reader,
	dest string,
	mode os.FileMode,
) error {
	if err := client.MkdirAll(path.Dir(dest)); err != nil {
		return err
	}
	file, err := client.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, source); err != nil {
		_ = file.Close()

		return err
	}
	if err := file.Close(); err != nil {
		return err
	}

	return client.Chmod(dest, mode)
}

// removeAllIfExists exists because, unlike os.RemoveAll, the SFTP client reports a missing path
// as an error — which would otherwise force callers to special-case first-time installs and
// repeated removes.
func removeAllIfExists(client *sftp.Client, targetDir string) error {
	if _, err := client.Stat(targetDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}

	return client.RemoveAll(targetDir)
}
