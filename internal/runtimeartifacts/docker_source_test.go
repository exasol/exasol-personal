// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"testing"
)

func TestDockerUrlDownloadTarget(t *testing.T) {
	t.Parallel()

	dockerCases := []struct {
		name   string
		tag    string
		digest string
	}{
		{"example.org/foo", "", ""},
		{"example.org/foo", ":latest", ""},
		{"example.org/foo", ":1.5", ""},
		{
			"example.org/foo",
			"",
			"@sha256:93512673ca38053cb45fa33eaa9ac999fc93c2c8f70c873d054432433c5e81bf",
		},
		{
			"example.org/foo",
			":latest",
			"@sha256:93512673ca38053cb45fa33eaa9ac999fc93c2c8f70c873d054432433c5e81bf",
		},
		{
			"example.org/foo",
			":1.5",
			"@sha256:93512673ca38053cb45fa33eaa9ac999fc93c2c8f70c873d054432433c5e81bf",
		},
	}
	ociCases := []struct {
		name string
		tag  string
	}{
		{"/foo/bar.tar", ""},
		{"/foo/bar.tar", ":latest"},
	}

	for _, tc := range dockerCases {
		inUrl := "docker://" + tc.name + tc.tag + tc.digest
		wantDownload := ("docker://" + tc.name + tc.digest)
		wantTag := (tc.name + tc.tag)
		downloadUrl, targetTag, err := DockerUrlDownloadTarget(inUrl)
		if err != nil || downloadUrl != wantDownload || targetTag != wantTag {
			t.Errorf("DockerUrlDownloadTarget(%q) = (%q, %q), want (%q, %q)",
				inUrl, downloadUrl, targetTag, wantDownload, wantTag)
		}
	}
	for _, tc := range ociCases {
		inUrl := "oci:" + tc.name + tc.tag
		wantDownload := "oci:" + tc.name + tc.tag
		wantTag := tc.name + tc.tag
		downloadUrl, targetTag, err := DockerUrlDownloadTarget(inUrl)
		if err != nil || downloadUrl != wantDownload || targetTag != wantTag {
			t.Errorf("DockerUrlDownloadTarget(%q) = (%q, %q), want (%q, %q)",
				inUrl, downloadUrl, targetTag, wantDownload, wantTag)
		}
	}
	for _, tc := range ociCases {
		inUrl := "oci-archive:" + tc.name + tc.tag
		wantDownload := "oci-archive:" + tc.name + tc.tag
		wantTag := tc.name + tc.tag
		downloadUrl, targetTag, err := DockerUrlDownloadTarget(inUrl)
		if err != nil || downloadUrl != wantDownload || targetTag != wantTag {
			t.Errorf("DockerUrlDownloadTarget(%q) = (%q, %q), want (%q, %q)",
				inUrl, downloadUrl, targetTag, wantDownload, wantTag)
		}
	}
}

// nolint: revive
func TestDockerSource_CanFetch_RemoteURLs(t *testing.T) {
	t.Parallel()

	src := &DockerSource{}
	trueURLs := []string{
		"docker://example.org/foo",
		"docker://example.org/foo:latest",
		"docker://example.org/foo:1.5",
		"docker://example.org/foo@sha256:93512673ca38053cb45fa33eaa9ac999fc93c2c8f70c873d054432433c5e81bf",
		"docker://example.org/foo:latest@sha256:93512673ca38053cb45fa33eaa9ac999fc93c2c8f70c873d054432433c5e81bf",
		"docker://example.org/foo:1.5@sha256:93512673ca38053cb45fa33eaa9ac999fc93c2c8f70c873d054432433c5e81bf",
		"oci:/foo/bar.tar",
		"oci:/foo/bar.tar:latest",
		"oci-archive:/foo/bar.tar",
		"oci-archive:/foo/bar.tar:latest",
	}
	for _, url := range trueURLs {
		if !src.CanFetch(url) {
			t.Errorf("CanFetch(%q) = false, want true", url)
		}
	}

	falseURLs := []string{
		"docker://example.org/foo@sha256:invalid",
		"docker://example.org/foo:latest@sha256:invalid",
		"docker://example.org/foo:1.5@sha256:invalid",
		"oci:/foo/bar.tar@sha256:93512673ca38053cb45fa33eaa9ac999fc93c2c8f70c873d054432433c5e81bf",
		"oci:/foo/bar.tar:latest@sha256:93512673ca38053cb45fa33eaa9ac999fc93c2c8f70c873d054432433c5e81bf",
		"oci-archive:/foo/bar.tar@sha256:93512673ca38053cb45fa33eaa9ac999fc93c2c8f70c873d054432433c5e81bf",
		"oci-archive:/foo/bar.tar:latest@sha256:93512673ca38053cb45fa33eaa9ac999fc93c2c8f70c873d054432433c5e81bf",
		"https://example.com/archive.tar.gz",
		"http://example.com/archive.tar.gz",
		"file:///path/to/dir",
		"",
	}
	for _, url := range falseURLs {
		if src.CanFetch(url) {
			t.Errorf("CanFetch(%q) = true, want false", url)
		}
	}
}
