// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/podmanmachine"
	"github.com/exasol/exasol-personal/internal/prompt"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/exasol/exasol-personal/internal/winget"
)

// rootlessPastaNetworkArgs are the podman-run flags injected for
// rootless podman-for-windows containers. gvproxy's default rootless
// publishing binds only to the Windows IPv6 loopback ([::1]:<port>),
// and TLS handshakes over that IPv6 path fail — the peer aborts the
// connection between ClientHello and ServerHello. Forcing pasta to
// IPv4-only makes gvproxy publish on 127.0.0.1 instead, over which
// TLS handshakes complete cleanly. Empirically verified 2026-08-25;
// see ROOTLESS_PODMAN_REPORT.md for the diagnostic trail.
var rootlessPastaNetworkArgs = []string{
	"--network", "pasta:--ipv4-only",
}

// WindowsHostRuntime provides the Windows implementation of Runtime. It
// delegates container lifecycle to localinstall.PodmanInstall exactly like
// LinuxHostRuntime; the Windows-specific parts are:
//
//   - Prepare installs podman-for-windows via winget when missing and
//     ensures the default podman machine exists and is running.
//   - Start re-checks the machine before delegating (the machine can be
//     stopped between launcher invocations) and, on rootless machines,
//     injects pasta network flags to work around gvproxy's multi-segment
//     TLS handshake abort. See ROOTLESS_PODMAN_REPORT.md.
//   - The Nano-startup durability workaround is a no-op on Windows: Nano's
//     data lives inside the WSL2 VM's ext4 disk, whose VHDX backing on
//     the Windows host provides crash-safe writeback semantics, and there
//     is no reliable POSIX `sync` binary on Windows to invoke.
type WindowsHostRuntime struct {
	deployment  config.DeploymentDir
	paths       runtimePaths
	manager     *runtimeartifacts.Manager
	endpoint    *RuntimeEndpoint
	runtimeExec []string
	dialContext func(context.Context, string, string) (net.Conn, error)
}

// manager may be nil for operations that never need to invoke podman
// (e.g. Destroy on a deployment that was never prepared).
func NewHostWindowsRuntime(
	deployment config.DeploymentDir,
	manager *runtimeartifacts.Manager,
) *WindowsHostRuntime {
	return &WindowsHostRuntime{
		deployment: deployment,
		paths:      newRuntimePaths(deployment),
		manager:    manager,
	}
}

func (runtime *WindowsHostRuntime) Deployment() config.DeploymentDir {
	return runtime.deployment
}

// Prepare ensures podman-for-windows is installed, the default podman
// machine exists and is running, and the local runtime work directory
// exists. If in refers to a terminal the setup steps prompt for consent;
// otherwise they proceed automatically.
//
// The machine is created rootless on first-install. Start compensates
// for rootless containers by injecting pasta network flags so the TLS
// handshake succeeds despite gvproxy's multi-segment forwarding issue.
func (runtime *WindowsHostRuntime) Prepare(
	ctx context.Context,
	in io.Reader,
	out, outErr io.Writer,
) error {
	if err := runtime.ensurePodmanAvailable(ctx, in, out, outErr); err != nil {
		return err
	}
	if _, err := podmanmachine.EnsureMachineRunning(ctx, out, outErr); err != nil {
		return err
	}
	if err := os.MkdirAll(runtime.paths.WorkDir, dirMode); err != nil {
		return fmt.Errorf("failed to create local runtime directory: %w", err)
	}

	return nil
}

func (runtime *WindowsHostRuntime) Start(
	ctx context.Context,
	out, outErr io.Writer,
	runtimeConfig VMConfig,
) error {
	// Re-run the machine preflight: it can be stopped between Prepare
	// and Start (reboot, user action). Otherwise install.Start would
	// fail later with an opaque "podman socket unreachable".
	status, err := podmanmachine.EnsureMachineRunning(ctx, out, outErr)
	if err != nil {
		return err
	}
	startConfig, err := runtime.podmanStartConfig(runtimeConfig.RuntimeConfig)
	if err != nil {
		return err
	}
	if !status.Rootful {
		startConfig.ExtraRunArgs = append(
			startConfig.ExtraRunArgs, rootlessPastaNetworkArgs...,
		)
	}
	if err := runtime.install().Start(ctx, out, outErr, startConfig); err != nil {
		return err
	}
	runtime.endpoint = &RuntimeEndpoint{DBPort: startConfig.ContainerDBPort}

	return nil
}

func (runtime *WindowsHostRuntime) Stop(ctx context.Context, out, outErr io.Writer) error {
	return runtime.install().Stop(ctx, out, outErr)
}

func (runtime *WindowsHostRuntime) Status(ctx context.Context) (*RuntimeStatus, error) {
	status, err := runtime.install().Status(ctx, nil, nil)
	if err != nil {
		return nil, err
	}

	return &RuntimeStatus{Running: status.Running}, nil
}

func (runtime *WindowsHostRuntime) Destroy(ctx context.Context, out, outErr io.Writer) error {
	if err := runtime.install().Destroy(ctx, out, outErr); err != nil {
		return fmt.Errorf("failed to remove deployment: %w", err)
	}
	if err := os.RemoveAll(runtime.paths.Root); err != nil {
		return fmt.Errorf("failed to remove local runtime files %s: %w", runtime.paths.Root, err)
	}

	return nil
}

// WorkaroundNanoStartupDurability is a no-op on Windows. See type doc.
func (*WindowsHostRuntime) WorkaroundNanoStartupDurability(
	context.Context, io.Writer, io.Writer,
) error {
	return nil
}

func (runtime *WindowsHostRuntime) ReadEndpoints() (*VMRuntimeEndpoint, error) {
	if runtime.endpoint == nil {
		info, err := config.ReadDeploymentInfo(runtime.Deployment())
		if err != nil {
			return nil, fmt.Errorf("windows host runtime endpoint is unavailable: %w", err)
		}
		if info.Connection == nil || info.Connection.DBPort <= 0 || info.Connection.DBPort > 65535 {
			return nil, errors.New(
				"windows host runtime deployment information has no valid database endpoint",
			)
		}
		runtime.endpoint = &RuntimeEndpoint{DBPort: info.Connection.DBPort}
	}

	return &VMRuntimeEndpoint{RuntimeEndpoint: *runtime.endpoint}, nil
}

func (runtime *WindowsHostRuntime) HealthCheck(ctx context.Context) (*HealthCheckResult, error) {
	endpoint, err := runtime.ReadEndpoints()
	if err != nil {
		return nil, err
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(endpoint.DBPort))
	dialContext := runtime.dialContext
	if dialContext == nil {
		dialContext = (&net.Dialer{}).DialContext
	}
	connection, err := dialContext(ctx, "tcp", address)
	state := classifyHostPortHealth(err)
	if connection != nil {
		_ = connection.Close()
	}

	return &HealthCheckResult{Ports: map[string]PortHealth{
		"db": {State: state},
	}}, nil
}

func (*WindowsHostRuntime) OpenHostShell(
	context.Context, io.Reader, io.Writer, io.Writer,
) error {
	return ErrHostShellUnsupported
}

func (*WindowsHostRuntime) OpenContainerShell(
	context.Context, io.Reader, io.Writer, io.Writer,
) error {
	return ErrContainerShellUnsupported
}

func (runtime *WindowsHostRuntime) install() *localinstall.PodmanInstall {
	// Sync-disabled execution environment: Windows has no POSIX `sync`
	// binary, and container data lives inside the WSL2 VM's ext4 disk
	// (crash-safe via VHDX writeback) so host-side sync has no target.
	environment := &windowsHostExecutionEnvironment{
		DirectExecutionEnvironment: localinstall.NewDirectExecutionEnvironment(
			runtime.runtimeExec,
		),
	}
	resolveImage := func(ctx context.Context) (localinstall.RuntimePath, error) {
		path, err := localinstall.ResolveNanoImage(ctx, runtime.manager)
		if err != nil {
			return localinstall.RuntimePath{}, err
		}

		return localinstall.IdentityRuntimePath(path), nil
	}

	return localinstall.NewPodmanInstallWithEnvironment(
		runtime.Deployment(),
		environment,
		resolveImage,
		localinstall.IdentityRuntimePath(SLCStagingDir(runtime.Deployment())),
		localinstall.IdentityRuntimePath(SLCStatusPath(runtime.Deployment())),
	)
}

func (runtime *WindowsHostRuntime) podmanStartConfig(
	runtimeConfig RuntimeConfig,
) (localinstall.StartConfig, error) {
	hostDBPort, err := resolveHostPodmanDBPort(runtimeConfig.Ports)
	if err != nil {
		return localinstall.StartConfig{}, err
	}

	return localinstall.StartConfig{
		ContainerDBPort: hostDBPort,
		DataDir:         filepath.Join(runtime.paths.WorkDir, nanoDataDirName),
		InitParams:      append([]string(nil), nanoInitParams...),
		VersionCheck:    runtimeConfig.VersionCheck,
		SLCs:            runtimeConfig.SLCs,
	}, nil
}

// windowsHostExecutionEnvironment embeds DirectExecutionEnvironment and
// overrides Sync to be a no-op. See install() for the rationale.
type windowsHostExecutionEnvironment struct {
	*localinstall.DirectExecutionEnvironment
}

func (*windowsHostExecutionEnvironment) Sync(context.Context, io.Writer, io.Writer) error {
	return nil
}

func (runtime *WindowsHostRuntime) ensurePodmanAvailable(
	ctx context.Context,
	in io.Reader,
	out, outErr io.Writer,
) error {
	out, outErr = discardIfNil(out), discardIfNil(outErr)
	if _, err := exec.LookPath("podman"); err == nil {
		return nil
	}
	// Stale-PATH recovery: podman may already be installed by a prior
	// winget run whose PATH updates this process never saw.
	if err := winget.EnsurePodmanOnPath(); err == nil {
		if _, err := exec.LookPath("podman"); err == nil {
			return nil
		}
	}
	if err := winget.LookupWinget(); err != nil {
		fmt.Fprintln(outErr,
			"podman-for-windows is not installed and winget is unavailable to install it.")
		fmt.Fprintln(outErr,
			"Install App Installer from the Microsoft Store, or install podman-for-windows")
		fmt.Fprintln(outErr, "manually from https://podman.io/, then re-run this command.")

		return err
	}
	fmt.Fprintln(out, "podman-for-windows is not installed on this system.")
	fmt.Fprintln(out, "The launcher will install it now by running:")
	fmt.Fprintln(out, "  "+winget.PodmanInstallCommand())
	fmt.Fprintln(out,
		"This may prompt for administrator (UAC) approval and take a few minutes.")
	consent, err := prompt.YesNo(in, out,
		"Install podman-for-windows now?", true)
	if err != nil {
		return fmt.Errorf("could not read install-consent prompt: %w", err)
	}
	if !consent {
		return errors.New(
			"cannot proceed without podman-for-windows; " +
				"install it manually from https://podman.io/ and re-run this command")
	}
	if err := winget.InstallPodman(ctx, out, outErr); err != nil {
		fmt.Fprintln(outErr, "")
		fmt.Fprintln(outErr,
			"Winget was unable to install podman-for-windows automatically.")
		fmt.Fprintln(outErr,
			"Please install podman-for-windows manually from https://podman.io/,")
		fmt.Fprintln(outErr, "then re-run this command.")

		return err
	}
	if err := winget.EnsurePodmanOnPath(); err != nil {
		return fmt.Errorf("failed to refresh PATH after winget install: %w", err)
	}
	if _, err := exec.LookPath("podman"); err != nil {
		return fmt.Errorf(
			"winget install completed but podman is still not on PATH: %w", err)
	}
	fmt.Fprintln(out, "podman is installed.")

	return nil
}

// discardIfNil replaces a nil io.Writer with io.Discard so subsequent
// fmt.Fprintln calls do not nil-deref. The deploy pipeline passes nil
// writers into Runtime.Prepare by convention.
func discardIfNil(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}

	return w
}

// Compile-time interface assertion.
var _ Runtime = (*WindowsHostRuntime)(nil)
