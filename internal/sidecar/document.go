// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar

import (
	"errors"
	"fmt"
)

func Parse(data []byte) (Document, error) {
	var document Document
	if err := decode(data, &document); err != nil {
		return Document{}, err
	}
	if document.Version != 1 || document.Containers == nil {
		return Document{}, errors.New("document: version must be 1 and containers must be present")
	}
	names := make(map[string]bool)
	for index, container := range document.Containers {
		location := fmt.Sprintf("containers[%d]", index)
		if names[container.Name] {
			return Document{}, fmt.Errorf("%s.name: duplicate container name", location)
		}
		names[container.Name] = true
		if err := container.Validate(location); err != nil {
			return Document{}, err
		}
	}

	return document, nil
}
