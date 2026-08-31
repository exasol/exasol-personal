// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

const deterministicTarMode = 0o600

var deterministicTarTimestamp = time.Unix(0, 0).UTC()

// repackTarDeterministically rewrites srcPath to dstPath while retaining entry
// names, types, links, and contents. Metadata that depends on the host file
// system is replaced with fixed values so the resulting archive is identical
// on every supported platform.
func repackTarDeterministically(srcPath, dstPath string) (retErr error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() {
		retErr = errors.Join(retErr, src.Close())
	}()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	succeeded := false
	dstClosed := false
	defer func() {
		if !dstClosed {
			retErr = errors.Join(retErr, dst.Close())
		}
		if !succeeded {
			_ = os.Remove(dstPath)
		}
	}()

	reader := tar.NewReader(src)
	writer := tar.NewWriter(dst)
	writerClosed := false
	defer func() {
		if !writerClosed {
			retErr = errors.Join(retErr, writer.Close())
		}
	}()

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}

		normalized := deterministicTarHeader(header)
		if err := writer.WriteHeader(normalized); err != nil {
			return fmt.Errorf("write tar header %q: %w", header.Name, err)
		}
		if _, err := io.CopyN(writer, reader, header.Size); err != nil {
			return fmt.Errorf("copy tar entry %q: %w", header.Name, err)
		}
	}

	if err := writer.Close(); err != nil {
		return err
	}
	writerClosed = true
	if err := dst.Close(); err != nil {
		return err
	}
	dstClosed = true
	succeeded = true

	return nil
}

func deterministicTarHeader(source *tar.Header) *tar.Header {
	return &tar.Header{
		Typeflag: source.Typeflag,
		Name:     source.Name,
		Linkname: source.Linkname,
		Size:     source.Size,
		Mode:     deterministicTarMode,
		ModTime:  deterministicTarTimestamp,
		Format:   tar.FormatUSTAR,
	}
}
