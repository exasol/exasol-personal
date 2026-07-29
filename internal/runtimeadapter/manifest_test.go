// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

//nolint:forcetypeassert,revive // The test asserts the renderer's known YAML shape.
func TestRenderKubeManifestPreservesNanoRuntimeInvariants(t *testing.T) {
	t.Parallel()

	// Given
	spec := testWorkloadSpec()

	// When
	data, err := RenderKubeManifest(spec)
	// Then
	if err != nil {
		t.Fatalf("expected manifest rendering to succeed, got %v", err)
	}
	var manifest map[string]any
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("expected valid YAML, got %v", err)
	}
	metadata := manifest["metadata"].(map[string]any)
	annotations := metadata["annotations"].(map[string]any)
	if annotations["io.podman.annotations.pids-limit/nano"] != "-1" {
		t.Fatalf("expected unlimited PID annotation, got %#v", annotations)
	}
	specMap := manifest["spec"].(map[string]any)
	if specMap["restartPolicy"] != "Always" {
		t.Fatalf("expected Always restart policy, got %#v", specMap["restartPolicy"])
	}
	container := specMap["containers"].([]any)[0].(map[string]any)
	if container["image"] != spec.ImageReference || container["imagePullPolicy"] != "Never" {
		t.Fatalf("unexpected image contract: %#v", container)
	}
	security := container["securityContext"].(map[string]any)
	if security["procMount"] != "Unmasked" {
		t.Fatalf("expected unmasked proc mount, got %#v", security)
	}
	ports := container["ports"].([]any)
	port := ports[0].(map[string]any)
	if port["containerPort"] != NanoContainerPort || port["hostPort"] != spec.DBHostPort {
		t.Fatalf("unexpected database port: %#v", port)
	}
	volumes := specMap["volumes"].([]any)
	assertVolume(t, volumes, "exa-data", func(volume map[string]any) bool {
		hostPath := volume["hostPath"].(map[string]any)
		return hostPath["path"] == spec.DataPath && hostPath["type"] == "DirectoryOrCreate"
	})
	assertVolume(t, volumes, "shared-memory", func(volume map[string]any) bool {
		emptyDir := volume["emptyDir"].(map[string]any)
		return emptyDir["medium"] == "Memory" && emptyDir["sizeLimit"] == "512Mi"
	})
	assertVolume(t, volumes, "slc-0", func(volume map[string]any) bool {
		image := volume["image"].(map[string]any)
		return image["reference"] == spec.SLCMounts[0].Image && image["pullPolicy"] == "Never"
	})
	arguments := container["args"].([]any)
	joined := make([]string, len(arguments))
	for index, argument := range arguments {
		joined[index] = argument.(string)
	}
	for _, expected := range []string{
		"init",
		"params=maxConnectionsLicenseLimit=20",
		"VERSION_CHECK_ENABLED=1",
		"VERSION_CHECK_ENDPOINT=https://metrics.example.test/v1/version-check",
		"VERSION_CHECK_OPERATING_SYSTEM=MacOS",
	} {
		if !contains(joined, expected) {
			t.Fatalf("expected argument %q in %#v", expected, joined)
		}
	}
}

func TestRenderKubeManifestRequiresDigestAndSecurityInvariants(t *testing.T) {
	t.Parallel()

	// Given
	tests := []struct {
		name   string
		mutate func(*WorkloadSpec)
	}{
		{
			name: "mutable image",
			mutate: func(spec *WorkloadSpec) {
				spec.ImageReference = "docker.io/exasol/nano:latest"
			},
		},
		{
			name:   "bounded pids",
			mutate: func(spec *WorkloadSpec) { spec.Security.PIDsLimit = 100 },
		},
		{
			name:   "masked proc",
			mutate: func(spec *WorkloadSpec) { spec.Security.UnmaskAll = false },
		},
		{
			name:   "wrong shm",
			mutate: func(spec *WorkloadSpec) { spec.Security.ShmMiB = 256 },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			spec := testWorkloadSpec()
			test.mutate(&spec)

			// When
			_, err := RenderKubeManifest(spec)

			// Then
			if err == nil {
				t.Fatal("expected invalid invariant to be rejected")
			}
		})
	}
}

func TestRenderWorkloadHelperUsesOnlyDeclarativeKubeLifecycle(t *testing.T) {
	t.Parallel()

	// Given
	spec := testWorkloadSpec()

	// When
	data, err := RenderWorkloadHelper(
		spec,
		"/mnt/control/workload.yaml",
		"/mnt/control/nano.tar.gz",
	)
	// Then
	if err != nil {
		t.Fatalf("expected helper rendering to succeed, got %v", err)
	}
	script := string(data)
	for _, expected := range []string{
		"podman kube play --replace",
		"podman kube down",
		"podman pod inspect",
		"podman logs",
		"podman load --input",
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("expected helper to contain %q:\n%s", expected, script)
		}
	}
	if strings.Contains(script, "podman run") {
		t.Fatalf("helper must not fall back to podman run:\n%s", script)
	}
}

func TestWorkloadNameIsDeterministicAndIsolated(t *testing.T) {
	t.Parallel()

	// Given / When
	first := WorkloadName("2A0A5901-1C27-43D7-991B-922D04902713")
	repeated := WorkloadName("2A0A5901-1C27-43D7-991B-922D04902713")
	second := WorkloadName("5fd46e6c-9267-45f7-8822-36a4da26f787")

	// Then
	if first != repeated {
		t.Fatalf("expected stable name, got %q and %q", first, repeated)
	}
	if first == second {
		t.Fatalf("expected isolated names, got %q", first)
	}
	if len(first) > kubeNameMaxLength {
		t.Fatalf("name is too long: %q", first)
	}
}

func testWorkloadSpec() WorkloadSpec {
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	return WorkloadSpec{
		DeploymentID:   "2a0a5901-1c27-43d7-991b-922d04902713",
		ImageReference: "docker.io/exasol/nano@" + digest,
		ImageDigest:    digest,
		DataPath:       "/mnt/exa",
		DBHostPort:     8563,
		CPUs:           2,
		MemoryMiB:      8192,
		NanoArguments:  []string{"params=maxConnectionsLicenseLimit=20"},
		VersionCheck: VersionCheckSettings{
			Enabled:              true,
			URL:                  "https://metrics.example.test/v1/version-check",
			Identity:             "exasol-personal;deployment;small;default",
			OperatingSystem:      "MacOS",
			IntervalSeconds:      86400,
			RetryIntervalSeconds: 86400,
		},
		SLCMounts: []SLCMount{{
			Image:  "docker.io/exasol/script-language-container:content-addressed-tag",
			Target: "/exa/slc/python312",
		}},
		Security: DefaultContainerSecurity(),
		LoadImageArchive: func() ([]byte, error) {
			return []byte("test image"), nil
		},
	}
}

//nolint:forcetypeassert,revive // The test asserts the renderer's known YAML shape.
func assertVolume(
	t *testing.T,
	volumes []any,
	name string,
	matches func(map[string]any) bool,
) {
	t.Helper()
	for _, candidate := range volumes {
		volume := candidate.(map[string]any)
		if volume["name"] == name {
			if !matches(volume) {
				t.Fatalf("volume %q does not match: %#v", name, volume)
			}

			return
		}
	}
	t.Fatalf("volume %q not found in %#v", name, volumes)
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}

	return false
}
