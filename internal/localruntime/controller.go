// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	localconfig "github.com/exasol/exasol-personal/internal/localruntime/config"
	"github.com/exasol/exasol-personal/internal/localruntime/vm"
)

const (
	controlShareTag        = "exa-control"
	guestControlMountPoint = "/.exanano/control"
	gracefulStopCommand    = "STOP_GRACEFUL"
	controlPollInterval    = 100 * time.Millisecond
)

var (
	ErrRuntimeStateUnavailable = errors.New("local runtime state is unavailable")
	ErrRuntimeStateInvalid     = errors.New("local runtime state is invalid")
)

type ControlPaths struct {
	ShareTag             string
	HostDir              string
	HostSocketPath       string
	HostRuntimeStatePath string
	HostStopRequestPath  string
	HostPIDFilePath      string
	GuestDir             string
	GuestSocketPath      string
	GuestRuntimeState    string
	GuestStopRequestPath string
	GuestPIDFilePath     string
}

type GuestRuntimeState struct {
	SQLPort        int
	UIPort         int
	JupyterEnabled bool
	JupyterPort    int
	VoilaPort      int
	EnabledStacks  []string
	StackPorts     []GuestStackPort
}

type GuestStackPort struct {
	StackName   string
	ServiceName string
	HostPort    int
	GuestPort   int
}

type Controller struct {
	layout localconfig.Layout
	dial   func(ctx context.Context, network string, address string) (net.Conn, error)
}

func NewController(layout localconfig.Layout) Controller {
	return Controller{
		layout: layout,
		dial:   (&net.Dialer{}).DialContext,
	}
}

func (c Controller) Paths() ControlPaths {
	return ControlPaths{
		ShareTag:             controlShareTag,
		HostDir:              c.layout.ControlDir(),
		HostSocketPath:       c.layout.ControlSocketPath(),
		HostRuntimeStatePath: c.layout.RuntimeStatePath(),
		HostStopRequestPath:  c.layout.StopRequestPath(),
		HostPIDFilePath:      c.layout.PIDFilePath(),
		GuestDir:             guestControlMountPoint,
		GuestSocketPath:      path.Join(guestControlMountPoint, "control.sock"),
		GuestRuntimeState:    path.Join(guestControlMountPoint, "runtime.state"),
		GuestStopRequestPath: path.Join(guestControlMountPoint, "stop.request"),
		GuestPIDFilePath:     path.Join(guestControlMountPoint, "exanano.pid"),
	}
}

func (c Controller) Ensure() error {
	paths := c.Paths()
	if err := os.MkdirAll(paths.HostDir, 0o700); err != nil {
		return fmt.Errorf("failed to create local runtime control dir: %w", err)
	}

	return c.ClearStopRequest()
}

func (c Controller) SharedDir() vm.SharedDir {
	paths := c.Paths()

	return vm.SharedDir{
		Tag:         paths.ShareTag,
		Source:      paths.HostDir,
		Destination: paths.GuestDir,
		ReadOnly:    false,
	}
}

func (c Controller) ClearStopRequest() error {
	if err := os.Remove(c.Paths().HostStopRequestPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to clear local runtime stop request: %w", err)
	}

	return nil
}

func (c Controller) RequestStop() error {
	if err := c.Ensure(); err != nil {
		return err
	}

	marker := []byte(time.Now().UTC().Format(time.RFC3339) + "\n")
	if err := os.WriteFile(c.Paths().HostStopRequestPath, marker, 0o600); err != nil {
		return fmt.Errorf("failed to write local runtime stop request: %w", err)
	}

	return nil
}

func (c Controller) RequestGracefulStop(ctx context.Context) error {
	response, err := c.SendCommand(ctx, gracefulStopCommand)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return c.RequestStop()
	}
	if !strings.HasPrefix(response, "OK") {
		return fmt.Errorf("unexpected local runtime stop response: %q", response)
	}

	return nil
}

func (c Controller) SendCommand(ctx context.Context, command string) (string, error) {
	socketPath := c.Paths().HostSocketPath
	dial := c.dial
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	conn, err := dial(ctx, "unix", socketPath)
	if err != nil {
		return "", fmt.Errorf("failed to connect to local runtime control socket %q: %w", socketPath, err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return "", fmt.Errorf("failed to apply control socket deadline: %w", err)
		}
	}

	if _, err := io.WriteString(conn, strings.TrimSpace(command)+"\n"); err != nil {
		return "", fmt.Errorf("failed to send local runtime control command: %w", err)
	}

	response, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("failed to read local runtime control response: %w", err)
	}

	return strings.TrimSpace(response), nil
}

func (c Controller) WaitForControlSocket(ctx context.Context) error {
	ticker := time.NewTicker(controlPollInterval)
	defer ticker.Stop()

	for {
		if ok, err := hasSocket(c.Paths().HostSocketPath); err == nil && ok {
			return nil
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c Controller) ReadRuntimeState() (*GuestRuntimeState, error) {
	data, err := os.ReadFile(c.Paths().HostRuntimeStatePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrRuntimeStateUnavailable, c.Paths().HostRuntimeStatePath)
		}

		return nil, fmt.Errorf("failed to read local runtime state file: %w", err)
	}

	return parseRuntimeState(string(data))
}

func (c Controller) WaitForRuntimeState(ctx context.Context) (*GuestRuntimeState, error) {
	ticker := time.NewTicker(controlPollInterval)
	defer ticker.Stop()

	for {
		state, err := c.ReadRuntimeState()
		if err == nil {
			return state, nil
		}
		if !errors.Is(err, ErrRuntimeStateUnavailable) && !errors.Is(err, ErrRuntimeStateInvalid) {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func parseRuntimeState(data string) (*GuestRuntimeState, error) {
	state := &GuestRuntimeState{}
	scanner := bufio.NewScanner(strings.NewReader(data))

	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("%w: line %d does not contain '='", ErrRuntimeStateInvalid, lineNumber)
		}

		switch key {
		case "sql_port":
			port, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid sql_port value %q", ErrRuntimeStateInvalid, value)
			}
			state.SQLPort = port
		case "ui_port":
			port, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid ui_port value %q", ErrRuntimeStateInvalid, value)
			}
			state.UIPort = port
		case "jupyter_enabled":
			enabled, err := parseBoolFlag(value)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid jupyter_enabled value %q", ErrRuntimeStateInvalid, value)
			}
			state.JupyterEnabled = enabled
		case "jupyter_port":
			port, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid jupyter_port value %q", ErrRuntimeStateInvalid, value)
			}
			state.JupyterPort = port
		case "voila_port":
			port, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid voila_port value %q", ErrRuntimeStateInvalid, value)
			}
			state.VoilaPort = port
		case "stack_enabled":
			state.EnabledStacks = append(state.EnabledStacks, value)
		case "stack_port":
			stackPort, err := parseStackPort(value)
			if err != nil {
				return nil, err
			}
			state.StackPorts = append(state.StackPorts, stackPort)
		default:
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan local runtime state file: %w", err)
	}

	if state.SQLPort == 0 || state.UIPort == 0 {
		return nil, fmt.Errorf("%w: required sql/ui ports are missing", ErrRuntimeStateInvalid)
	}

	return state, nil
}

func parseBoolFlag(value string) (bool, error) {
	switch strings.TrimSpace(value) {
	case "1", "true", "TRUE":
		return true, nil
	case "0", "false", "FALSE":
		return false, nil
	default:
		return false, errors.New("invalid boolean flag")
	}
}

func parseStackPort(value string) (GuestStackPort, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 4 {
		return GuestStackPort{}, fmt.Errorf(
			"%w: invalid stack_port value %q",
			ErrRuntimeStateInvalid,
			value,
		)
	}

	hostPort, err := strconv.Atoi(parts[2])
	if err != nil {
		return GuestStackPort{}, fmt.Errorf(
			"%w: invalid stack_port host port %q",
			ErrRuntimeStateInvalid,
			parts[2],
		)
	}
	guestPort, err := strconv.Atoi(parts[3])
	if err != nil {
		return GuestStackPort{}, fmt.Errorf(
			"%w: invalid stack_port guest port %q",
			ErrRuntimeStateInvalid,
			parts[3],
		)
	}

	return GuestStackPort{
		StackName:   parts[0],
		ServiceName: parts[1],
		HostPort:    hostPort,
		GuestPort:   guestPort,
	}, nil
}

func hasSocket(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	return info.Mode()&os.ModeSocket != 0, nil
}
