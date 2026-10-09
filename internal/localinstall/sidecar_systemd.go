// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/sidecar"
)

const (
	podmanPath         = "/usr/bin/podman"
	systemdStopTimeout = "10"
)

// Every remote operation runs as the deployment user, whose service manager
// owns both the deployment service and its rootless containers.
const systemdUserScript = `set -eu
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
unit_dir="$HOME/.config/systemd/user"
`

var (
	secretIndexPattern = regexp.MustCompile(`^[0-9]+$`)

	systemdRestartPolicies = map[string]string{
		sidecar.Always:    nanoRestartPolicy,
		sidecar.OnFailure: "on-failure",
		sidecar.Never:     "no",
	}
)

// SystemdSidecarRuntime runs sidecars as systemd units of the deployment
// service, so the node's own service manager starts them with the database,
// stops them with it, and applies their restart policy. Resolved values reach
// the container as container secrets, keeping them out of the unit file and
// out of the command line.
type SystemdSidecarRuntime struct {
	*SidecarRuntime

	service string
}

// NewSystemdSidecarRuntime binds sidecars to the named deployment service.
func NewSystemdSidecarRuntime(
	environment CommandRunner, owner, service string,
) (*SystemdSidecarRuntime, error) {
	containers, err := NewSidecarRuntime(environment, owner)
	if err != nil {
		return nil, err
	}
	if !runtimeNamePattern.MatchString(service) {
		return nil, errors.New("sidecar runtime requires a valid deployment service name")
	}

	return &SystemdSidecarRuntime{SidecarRuntime: containers, service: service}, nil
}

func (runtime *SystemdSidecarRuntime) Ensure(
	ctx context.Context, definition sidecar.Container, sources sidecar.Sources,
) error {
	index := 0
	plan, err := runtime.plan(definition, sources, containerCreation{
		// The service manager owns process restart, so Podman leaves it alone.
		restart: podmanRestartPolicies[sidecar.Never],
		env: func(name string) []string {
			reference := runtime.secretName(definition.Name, index) +
				",type=env,target=" + name
			index++

			return []string{"--secret", reference}
		},
	})
	if err != nil {
		return err
	}
	applied, err := runtime.applied(ctx, definition.Name, plan.fingerprint)
	if err != nil || applied {
		return err
	}
	if err := runtime.removeService(ctx, definition.Name); err != nil {
		return err
	}
	if _, err := runtime.converge(ctx, definition, plan.fingerprint); err != nil {
		return err
	}
	if err := runtime.writeSecrets(ctx, definition, plan); err != nil {
		return err
	}

	return runtime.installUnit(ctx, definition, plan)
}

func (runtime *SystemdSidecarRuntime) Remove(ctx context.Context, name string) error {
	if err := runtime.removeService(ctx, name); err != nil {
		return err
	}

	return runtime.SidecarRuntime.Remove(ctx, name)
}

func (runtime *SystemdSidecarRuntime) unitName(name string) string {
	return runtime.ContainerName(name) + ".service"
}

func (runtime *SystemdSidecarRuntime) secretName(name string, index int) string {
	return runtime.ContainerName(name) + "-env-" + strconv.Itoa(index)
}

func (runtime *SystemdSidecarRuntime) installUnit(
	ctx context.Context, definition sidecar.Container, plan sidecarPlan,
) error {
	unit := runtime.unitFile(definition, plan)
	var diagnostic bytes.Buffer
	err := runtime.environment.Run(ctx, nil, strings.NewReader(unit), io.Discard, &diagnostic,
		userCommand(`mkdir -p "$unit_dir"
umask 077
cat > "$unit_dir/$1"
systemctl --user daemon-reload
systemctl --user enable --now "$1"
`, runtime.unitName(definition.Name))...)
	if err != nil {
		return plan.startupError(definition.Name, err, diagnostic.String())
	}

	return nil
}

func (runtime *SystemdSidecarRuntime) removeService(ctx context.Context, name string) error {
	var diagnostic bytes.Buffer
	err := runtime.environment.Run(ctx, nil, nil, io.Discard, &diagnostic, userCommand(
		`systemctl --user disable --now "$1" >/dev/null 2>&1 || true
rm -f "$unit_dir/$1"
systemctl --user daemon-reload
`, runtime.unitName(name))...)
	if err != nil {
		return fmt.Errorf("sidecar %s: %w: %s", name, err, diagnostic.String())
	}

	return runtime.removeSecrets(ctx, name)
}

func (runtime *SystemdSidecarRuntime) writeSecrets(
	ctx context.Context, definition sidecar.Container, plan sidecarPlan,
) error {
	if err := runtime.removeSecrets(ctx, definition.Name); err != nil {
		return err
	}
	for index, variable := range plan.resolved.Container.Env {
		var diagnostic bytes.Buffer
		if err := runtime.environment.Run(ctx, nil,
			strings.NewReader(plan.env[variable.Name]), io.Discard, &diagnostic,
			podmanCommand, "secret", "create", "--replace",
			runtime.secretName(definition.Name, index), "-",
		); err != nil {
			return plan.startupError(definition.Name, err, diagnostic.String())
		}
	}

	return nil
}

func (runtime *SystemdSidecarRuntime) removeSecrets(ctx context.Context, name string) error {
	output, err := runtime.command(ctx, "secret", "ls", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	prefix := runtime.ContainerName(name) + "-env-"
	for secret := range strings.FieldsSeq(string(output)) {
		suffix, owned := strings.CutPrefix(secret, prefix)
		if !owned || !secretIndexPattern.MatchString(suffix) {
			continue
		}
		if _, err := runtime.command(ctx, "secret", "rm", "--ignore", secret); err != nil {
			return err
		}
	}

	return nil
}

func userCommand(script string, args ...string) []string {
	return append([]string{"sh", "-c", systemdUserScript + script, "sh"}, args...)
}

func (runtime *SystemdSidecarRuntime) unitFile(
	definition sidecar.Container, plan sidecarPlan,
) string {
	command := append([]string{podmanPath}, plan.args[1:]...)
	arguments := make([]string, 0, len(command))
	for _, argument := range command {
		arguments = append(arguments, systemdArgument(argument))
	}

	return fmt.Sprintf(`[Unit]
Description=Exasol Personal sidecar %s
PartOf=%s
After=%s

[Service]
Type=simple
Restart=%s
RestartSec=5
ExecStart=%s
ExecStop=%s stop --ignore --time %s %s

[Install]
WantedBy=%s
`,
		definition.Name, runtime.service, runtime.service,
		systemdRestartPolicies[definition.Defaults().RestartPolicy],
		strings.Join(arguments, " "),
		podmanPath, systemdStopTimeout, runtime.ContainerName(definition.Name),
		runtime.service,
	)
}

// systemdArgument quotes one command argument for a unit file, where unquoted
// whitespace separates arguments and `$` and `%` introduce substitutions.
func systemdArgument(value string) string {
	escaped := strings.NewReplacer(
		`\`, `\\`, `"`, `\"`, "$", "$$", "%", "%%",
	).Replace(value)

	return `"` + escaped + `"`
}
