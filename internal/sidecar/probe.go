// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	HTTP  = "HTTP"
	HTTPS = "HTTPS"

	defaultPeriodSeconds    = 10
	defaultTimeoutSeconds   = 1
	defaultFailureThreshold = 3
)

type Probe struct {
	Exec                *ExecAction      `yaml:"exec,omitempty"`
	HTTPGet             *HTTPGetAction   `yaml:"httpGet,omitempty"`
	TCPSocket           *TCPSocketAction `yaml:"tcpSocket,omitempty"`
	InitialDelaySeconds int              `yaml:"initialDelaySeconds,omitempty"`
	PeriodSeconds       int              `yaml:"periodSeconds,omitempty"`
	TimeoutSeconds      int              `yaml:"timeoutSeconds,omitempty"`
	FailureThreshold    int              `yaml:"failureThreshold,omitempty"`
}

type ExecAction struct {
	Command []string `yaml:"command"`
}

type HTTPGetAction struct {
	Path   string        `yaml:"path,omitempty"`
	Port   PortReference `yaml:"port"`
	Scheme string        `yaml:"scheme,omitempty"`
}

type TCPSocketAction struct {
	Port PortReference `yaml:"port"`
}

// PortReference is the Kubernetes int-or-string port: a container port number
// or the name of a declared port.
type PortReference struct {
	Number int
	Name   string
}

func (reference *PortReference) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: expected a port number or name", node.Line)
	}
	if node.Tag == "!!int" {
		number, err := strconv.Atoi(node.Value)
		if err != nil {
			return fmt.Errorf("line %d: %w", node.Line, err)
		}
		*reference = PortReference{Number: number}

		return nil
	}
	*reference = PortReference{Name: node.Value}

	return nil
}

func (reference PortReference) MarshalYAML() (any, error) {
	if reference.Name != "" {
		return reference.Name, nil
	}

	return reference.Number, nil
}

func (reference PortReference) String() string {
	if reference.Name != "" {
		return reference.Name
	}

	return strconv.Itoa(reference.Number)
}

// Port finds the declared container port the reference names.
func (reference PortReference) Port(ports []Port) (Port, bool) {
	index := slices.IndexFunc(ports, func(port Port) bool {
		if reference.Name != "" {
			return port.Name == reference.Name
		}

		return port.ContainerPort == reference.Number
	})
	if index < 0 {
		return Port{}, false
	}

	return ports[index], true
}

// Window is how long after the container starts a failing probe still counts
// as starting.
func (probe *Probe) Window() int {
	return probe.InitialDelaySeconds + probe.PeriodSeconds*probe.FailureThreshold
}

func (probe *Probe) clone() *Probe {
	if probe == nil {
		return nil
	}
	cloned := *probe
	if probe.Exec != nil {
		cloned.Exec = &ExecAction{Command: slices.Clone(probe.Exec.Command)}
	}
	if probe.HTTPGet != nil {
		action := *probe.HTTPGet
		cloned.HTTPGet = &action
	}
	if probe.TCPSocket != nil {
		action := *probe.TCPSocket
		cloned.TCPSocket = &action
	}

	return &cloned
}

func (probe *Probe) defaults() {
	if probe == nil {
		return
	}
	if probe.PeriodSeconds == 0 {
		probe.PeriodSeconds = defaultPeriodSeconds
	}
	if probe.TimeoutSeconds == 0 {
		probe.TimeoutSeconds = defaultTimeoutSeconds
	}
	if probe.FailureThreshold == 0 {
		probe.FailureThreshold = defaultFailureThreshold
	}
	if probe.HTTPGet != nil {
		if probe.HTTPGet.Path == "" {
			probe.HTTPGet.Path = "/"
		}
		if probe.HTTPGet.Scheme == "" {
			probe.HTTPGet.Scheme = HTTP
		}
	}
}

func (probe *Probe) validate(ports []Port, path string) error {
	if probe == nil {
		return nil
	}
	handlers := 0
	for _, declared := range []bool{
		probe.Exec != nil, probe.HTTPGet != nil, probe.TCPSocket != nil,
	} {
		if declared {
			handlers++
		}
	}
	if handlers != 1 {
		return fmt.Errorf("%s: expected exactly one of exec, httpGet, or tcpSocket", path)
	}
	if err := probe.validateTiming(path); err != nil {
		return err
	}
	switch {
	case probe.Exec != nil:
		return validateExec(probe.Exec, path+".exec")
	case probe.HTTPGet != nil:
		return validateHTTPGet(probe.HTTPGet, ports, path+".httpGet")
	default:
		return validateProbePort(probe.TCPSocket.Port, ports, path+".tcpSocket.port")
	}
}

func (probe *Probe) validateTiming(path string) error {
	// Zero selects the default, so only a negative value is out of range.
	for _, field := range []struct {
		name  string
		value int
	}{
		{"initialDelaySeconds", probe.InitialDelaySeconds},
		{"periodSeconds", probe.PeriodSeconds},
		{"timeoutSeconds", probe.TimeoutSeconds},
		{"failureThreshold", probe.FailureThreshold},
	} {
		if field.value < 0 {
			return fmt.Errorf("%s.%s: expected a non-negative integer", path, field.name)
		}
	}

	return nil
}

func validateExec(action *ExecAction, path string) error {
	if len(action.Command) == 0 {
		return fmt.Errorf("%s.command: expected a command", path)
	}
	for index, value := range action.Command {
		if strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("%s.command[%d]: contains a NUL character", path, index)
		}
	}

	return nil
}

func validateHTTPGet(action *HTTPGetAction, ports []Port, path string) error {
	if action.Path != "" && !strings.HasPrefix(action.Path, "/") {
		return fmt.Errorf("%s.path: expected a path beginning with /", path)
	}
	if !slices.Contains([]string{"", HTTP, HTTPS}, action.Scheme) {
		return fmt.Errorf("%s.scheme: expected HTTP or HTTPS", path)
	}

	return validateProbePort(action.Port, ports, path+".port")
}

// The launcher probes a network handler from the host, so it needs the
// published address.
func validateProbePort(reference PortReference, ports []Port, path string) error {
	port, declared := reference.Port(ports)
	if !declared || port.HostPort == 0 {
		return fmt.Errorf("%s: expected a container port with a hostPort", path)
	}

	return nil
}
