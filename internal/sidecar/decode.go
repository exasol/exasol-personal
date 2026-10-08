// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"go.yaml.in/yaml/v3"
)

func decode(data []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var node yaml.Node
	if err := decoder.Decode(&node); err != nil {
		return fmt.Errorf("document: %w", err)
	}
	if len(node.Content) != 1 {
		return errors.New("document: expected one mapping")
	}
	if err := checkShape(node.Content[0], reflect.TypeOf(target).Elem(), "document"); err != nil {
		return err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("document: expected exactly one YAML document")
	}
	if err := node.Decode(target); err != nil {
		return fmt.Errorf("document: %w", err)
	}

	return nil
}

// Check the syntax tree before decoding to retain paths and prevent scalar coercion.
func checkShape(node *yaml.Node, typ reflect.Type, path string) error {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	//exhaustive:ignore // Only the supported YAML schema types are accepted.
	switch typ.Kind() {
	case reflect.Struct, reflect.Map:
		if node.Kind != yaml.MappingNode {
			return fmt.Errorf("%s: expected a mapping", path)
		}

		return checkMapping(node, typ, path)
	case reflect.Slice:
		if node.Kind != yaml.SequenceNode {
			return fmt.Errorf("%s: expected a sequence", path)
		}
		for index, child := range node.Content {
			if err := checkShape(
				child,
				typ.Elem(),
				fmt.Sprintf("%s[%d]", path, index),
			); err != nil {
				return err
			}
		}
	case reflect.String, reflect.Int, reflect.Bool:
		tags := map[reflect.Kind]string{
			reflect.String: "!!str",
			reflect.Int:    "!!int",
			reflect.Bool:   "!!bool",
		}
		if node.Kind != yaml.ScalarNode || node.Tag != tags[typ.Kind()] {
			return fmt.Errorf("%s: expected %s", path, typ.Kind())
		}
	default:
		return fmt.Errorf("%s: unsupported schema type", path)
	}

	return nil
}

func checkMapping(node *yaml.Node, typ reflect.Type, path string) error {
	fields := make(map[string]reflect.Type)
	if typ.Kind() == reflect.Struct {
		for field := range typ.Fields() {
			fields[strings.Split(field.Tag.Get("yaml"), ",")[0]] = field.Type
		}
	}
	seen := make(map[string]bool)
	for index := 0; index < len(node.Content); index += 2 {
		key, value := node.Content[index], node.Content[index+1]
		location := path + "." + key.Value
		if key.Tag != "!!str" || seen[key.Value] {
			return fmt.Errorf("%s: expected a unique string key", location)
		}
		seen[key.Value] = true
		fieldType, exists := fields[key.Value]
		if typ.Kind() == reflect.Map {
			fieldType, exists = typ.Elem(), true
		}
		if !exists {
			return fmt.Errorf("%s: unsupported field", location)
		}
		if err := checkShape(value, fieldType, location); err != nil {
			return err
		}
	}

	return nil
}
