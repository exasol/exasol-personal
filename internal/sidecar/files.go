// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar

import (
	"fmt"
	"maps"
	"slices"
)

// File is a volume item resolved to its place in the container.
type File struct {
	// MountPath is the directory of the mount the file belongs to.
	MountPath string
	Path      string
	Content   string
	Sensitive bool
}

// ResolveFiles resolves the secret volumes mounted into the definition's
// container, following Kubernetes: without items every key becomes a file
// named after it, and an optional source drops what it cannot supply.
func ResolveFiles(definition Definition, secrets Secrets) ([]File, error) {
	name := definition.Container.Name
	volumes := make(map[string]Volume, len(definition.Volumes))
	for _, volume := range definition.Volumes {
		volumes[volume.Name] = volume
	}
	var files []File
	for _, mount := range definition.Container.VolumeMounts {
		volume, declared := volumes[mount.Name]
		if !declared || volume.Secret == nil {
			return nil, fmt.Errorf("sidecar %s: volume mount %s names no secret volume",
				name, mount.Name)
		}
		source := volume.Secret
		values, available := secrets[source.SecretName]
		if !available {
			if source.Optional {
				continue
			}

			return nil, fmt.Errorf("sidecar %s: required secret %s is unavailable",
				name, source.SecretName)
		}
		items := source.Items
		if len(items) == 0 {
			for _, key := range slices.Sorted(maps.Keys(values)) {
				items = append(items, KeyToPath{Key: key, Path: key})
			}
		}
		for _, item := range items {
			value, available := values[item.Key]
			if !available {
				if source.Optional {
					continue
				}

				return nil, fmt.Errorf(
					"sidecar %s: required secret %s key %s is unavailable",
					name, source.SecretName, item.Key,
				)
			}
			files = append(files, File{
				MountPath: mount.MountPath, Path: mount.MountPath + "/" + item.Path,
				Content: value.Value, Sensitive: value.Sensitive,
			})
		}
	}

	return files, nil
}
