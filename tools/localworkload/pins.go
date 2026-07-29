package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
)

const nanoImageLatest = "docker.io/exasol/nano"

type nanoImagePin struct {
	Reference         string
	ContainerPlatform ociPlatform
	HostPlatforms     []ociPlatform
}

// Maps host platform to container platform
func localImageMap() map[ociPlatform]ociPlatform {
	return map[ociPlatform]ociPlatform{
		{
			OS:           "darwin",
			Architecture: "arm64",
		}: {
			OS:           "linux",
			Architecture: "arm64",
		},
		{
			OS:           "linux",
			Architecture: "arm64",
		}: {
			OS:           "linux",
			Architecture: "arm64",
		},
		{
			OS:           "linux",
			Architecture: "amd64",
		}: {
			OS:           "linux",
			Architecture: "amd64",
		},
		{
			OS:           "windows",
			Architecture: "amd64",
		}: {
			OS:           "linux",
			Architecture: "amd64",
		},
	}
}

func localImagePinsFile() (string, error) {
	var dir string
	_, filename, _, ok := runtime.Caller(1)
	if ok {
		dir = filepath.Dir(filename)
	} else {
		return "", fmt.Errorf("Could not find pinned image list")
	}

	return filepath.Join(dir, "pins.json"), nil
}

func localImagePins(targets []ociPlatform) (map[string]nanoImagePin, error) {
	pinnedReferences := map[string]nanoImagePin{}
	pinFile, err := localImagePinsFile()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(pinFile)
	if err != nil {
		return nil, fmt.Errorf("Could not read pinned image list: %w", err)
	}

	if err := json.Unmarshal(data, &pinnedReferences); err != nil {
		return nil, fmt.Errorf("Could not parse pinned image list: %w", err)
	}

	imageMap := localImageMap()
	if targets == nil {
		targets = slices.Collect(maps.Keys(imageMap))
	}

	outReferences := map[string]nanoImagePin{}
	for pinName, pin := range pinnedReferences {
		pinHosts := []ociPlatform{}
		for _, target := range targets {
			if slices.Contains(pin.HostPlatforms, target) {
				pinHosts = append(pinHosts, target)
			}
		}
		if len(pinHosts) > 0 {
			outReferences[pinName] = nanoImagePin{
				Reference:         pin.Reference,
				ContainerPlatform: pin.ContainerPlatform,
				HostPlatforms:     pinHosts,
			}
		}
	}

	return outReferences, nil
}

func getPinnedDigest(imageRef string, platform ociPlatform) (string, error) {
	index := ociIndex{}

	out, err := exec.Command(
		"skopeo",
		"inspect",
		"--raw",
		"docker://"+imageRef,
	).Output()
	if err != nil {
		return "", fmt.Errorf("Failed to inspect image %s: %w", imageRef, err)
	}

	err = json.Unmarshal(out, &index)
	if err != nil {
		return "", fmt.Errorf("Failed to parse image index %s: %w", imageRef, err)
	}

	if index.SchemaVersion != 2 {
		return "", fmt.Errorf("Unknown index schema version %s: %d", imageRef, index.SchemaVersion)
	}

	for _, manifest := range index.Manifests {
		if *manifest.Platform == platform {
			return (imageRef + "@" + manifest.Digest), nil
		}
	}

	return "", fmt.Errorf("Cannot find platform %s/%s in index %s", platform.OS, platform.Architecture, imageRef)
}

func updateLocalImagePins() error {
	pinnedReferences := map[string]nanoImagePin{}

	pinFile, err := localImagePinsFile()
	if err != nil {
		return err
	}

	for hPlatform, cPlatform := range localImageMap() {
		cPlatformstr := cPlatform.OS + "_" + cPlatform.Architecture
		if _, exists := pinnedReferences[cPlatformstr]; exists {
			pin := pinnedReferences[cPlatformstr]
			pinnedReferences[cPlatformstr] = nanoImagePin{
				Reference:         pin.Reference,
				ContainerPlatform: pin.ContainerPlatform,
				HostPlatforms: append(
					pin.HostPlatforms,
					hPlatform,
				),
			}
		} else {
			pinnedDigest, err := getPinnedDigest(nanoImageLatest, cPlatform)
			if err != nil {
				return err
			}
			pinnedReferences[cPlatformstr] = nanoImagePin{
				Reference:         pinnedDigest,
				ContainerPlatform: cPlatform,
				HostPlatforms:     []ociPlatform{hPlatform},
			}
		}
	}

	pinOut, err := json.MarshalIndent(pinnedReferences, "", "  ")
	if err != nil {
		return fmt.Errorf("Could not serialize pinned references %w", err)
	}

	err = os.WriteFile(pinFile, pinOut, 0644)
	if err != nil {
		return fmt.Errorf("Could not write pin file: %w", err)
	}

	return nil
}
