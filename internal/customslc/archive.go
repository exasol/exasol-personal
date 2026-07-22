// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package customslc

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

var gzipMagic = []byte{0x1f, 0x8b}

func OpenTar(reader io.Reader) (*tar.Reader, func() error, error) {
	buffered := bufio.NewReader(reader)
	magic, err := buffered.Peek(len(gzipMagic))
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, nil, err
	}
	if errors.Is(err, io.EOF) || !bytes.Equal(magic, gzipMagic) {
		return tar.NewReader(buffered), func() error { return nil }, nil
	}

	gzipReader, err := gzip.NewReader(buffered)
	if err != nil {
		return nil, nil, err
	}
	finish := func() error {
		// Draining to EOF is what makes gzip verify its CRC; the size is unbounded by design
		// (a single-tenant, operator-supplied container of unpredictable size).
		//nolint:gosec // G110: unbounded decompression is intentional (see above).
		if _, err := io.Copy(io.Discard, gzipReader); err != nil {
			return err
		}

		return gzipReader.Close()
	}

	return tar.NewReader(gzipReader), finish, nil
}

// Symlink targets are intentionally not restricted — real SLC containers ship absolute symlinks
// (e.g. etc/resolv.conf -> /conf/resolv.conf), so symlinks are created verbatim, like tar does.
func CheckEntry(header *tar.Header) error {
	if !withinRoot(header.Name) {
		return fmt.Errorf("archive entry %q escapes the container root", header.Name)
	}
	if header.Typeflag == tar.TypeLink && !withinRoot(header.Linkname) {
		return fmt.Errorf(
			"archive hardlink %q points outside the container (%q)",
			header.Name, header.Linkname,
		)
	}

	return nil
}

// EntryChecker adds a stateful guard on top of CheckEntry: it rejects an entry that reuses a path
// already created as a symlink — the write-through escape (symlink to an outside path, then a
// write at or under it) that a per-entry check cannot catch.
type EntryChecker struct {
	symlinks map[string]bool
}

func NewEntryChecker() *EntryChecker {
	return &EntryChecker{symlinks: make(map[string]bool)}
}

func (c *EntryChecker) Check(header *tar.Header) error {
	if err := CheckEntry(header); err != nil {
		return err
	}

	name := path.Clean(header.Name)
	if c.traversesSymlink(name) {
		return fmt.Errorf(
			"archive entry %q is written through a symlink and escapes the container root",
			header.Name,
		)
	}
	if header.Typeflag == tar.TypeLink {
		if target := path.Clean(header.Linkname); c.traversesSymlink(target) {
			return fmt.Errorf(
				"archive hardlink %q resolves through a symlink (%q)",
				header.Name, header.Linkname,
			)
		}
	}
	if header.Typeflag == tar.TypeSymlink {
		c.symlinks[name] = true
	}

	return nil
}

func (c *EntryChecker) traversesSymlink(name string) bool {
	for current := name; ; {
		if c.symlinks[current] {
			return true
		}
		parent := path.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

func ValidateArchive(reader io.Reader) error {
	tarReader, finish, err := OpenTar(reader)
	if err != nil {
		return err
	}

	checker := NewEntryChecker()
	clientFound := false
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if err := checker.Check(header); err != nil {
			return err
		}
		if isClientExecutable(header) {
			clientFound = true
		}
	}

	if err := finish(); err != nil {
		return fmt.Errorf("container archive is corrupt: %w", err)
	}
	if !clientFound {
		return fmt.Errorf(
			"%s was not found as an executable in the container; Exasol Personal supports "+
				"standard SLCs built with exaslct",
			clientRelPath,
		)
	}

	return nil
}

func isClientExecutable(header *tar.Header) bool {
	if header.Typeflag != tar.TypeReg {
		return false
	}
	if path.Clean(header.Name) != clientRelPath {
		return false
	}

	return header.FileInfo().Mode().Perm()&0o111 != 0
}

func withinRoot(name string) bool {
	clean := path.Clean(name)

	return !path.IsAbs(clean) && clean != ".." && !strings.HasPrefix(clean, "../")
}
