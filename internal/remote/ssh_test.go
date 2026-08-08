// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package remote

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// sshExecResult is what a fake session's "exec" handler sends back over the
// wire: stdout/stderr bytes and a final exit-status.
type sshExecResult struct {
	stdout     string
	stderr     string
	exitStatus uint32
}

// startTestSSHServer runs a minimal in-process SSH server that accepts any
// client public key and forwards the first "exec" request on the first
// session channel to onExec, letting tests exercise the real wire protocol
// startSSHSession/RunScript use, without a live host. onSignal, if non-nil,
// is called for every "signal" channel request received while exec runs.
func startTestSSHServer(
	t *testing.T,
	onExec func(command string, stdin []byte) sshExecResult,
	onSignal func(signal string),
) string {
	t.Helper()

	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatalf("failed to build host signer: %v", err)
	}

	config := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, _ ssh.PublicKey) (*ssh.Permissions, error) {
			//nolint:nilnil // ssh's own contract for "accept, no restrictions".
			return nil, nil
		},
	}
	config.AddHostKey(signer)

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		serverConn, chans, reqs, err := ssh.NewServerConn(conn, config)
		if err != nil {
			return
		}
		defer serverConn.Close()
		go ssh.DiscardRequests(reqs)

		for newChannel := range chans {
			if newChannel.ChannelType() != "session" {
				_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")
				continue
			}
			channel, requests, err := newChannel.Accept()
			if err != nil {
				return
			}
			handleTestSSHSession(channel, requests, onExec, onSignal)
		}
	}()

	return listener.Addr().String()
}

// handleTestSSHSession dispatches "exec" to its own goroutine so the main
// loop keeps consuming requests (in particular "signal") while the exec
// handler is still running/blocked, matching how a real sshd session behaves.
func handleTestSSHSession(
	channel ssh.Channel,
	requests <-chan *ssh.Request,
	onExec func(command string, stdin []byte) sshExecResult,
	onSignal func(signal string),
) {
	for req := range requests {
		switch req.Type {
		case "exec":
			handleTestSSHExec(channel, req, onExec)
		case "signal":
			handleTestSSHSignal(req, onSignal)
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

// handleTestSSHExec replies to the "exec" request and runs onExec in its own
// goroutine so the session's main loop keeps consuming requests while it runs.
func handleTestSSHExec(
	channel ssh.Channel,
	req *ssh.Request,
	onExec func(command string, stdin []byte) sshExecResult,
) {
	var payload struct{ Command string }
	_ = ssh.Unmarshal(req.Payload, &payload)
	_ = req.Reply(true, nil)

	go runTestSSHExec(channel, payload.Command, onExec)
}

func runTestSSHExec(
	channel ssh.Channel,
	command string,
	onExec func(command string, stdin []byte) sshExecResult,
) {
	defer channel.Close()

	stdin, _ := io.ReadAll(channel)
	result := sshExecResult{}
	if onExec != nil {
		result = onExec(command, stdin)
	}
	if result.stdout != "" {
		_, _ = channel.Write([]byte(result.stdout))
	}
	if result.stderr != "" {
		_, _ = channel.Stderr().Write([]byte(result.stderr))
	}
	_, _ = channel.SendRequest(
		"exit-status",
		false,
		ssh.Marshal(struct{ Status uint32 }{result.exitStatus}),
	)
}

func handleTestSSHSignal(req *ssh.Request, onSignal func(signal string)) {
	var payload struct{ Signal string }
	_ = ssh.Unmarshal(req.Payload, &payload)
	if onSignal != nil {
		onSignal(payload.Signal)
	}
	if req.WantReply {
		_ = req.Reply(true, nil)
	}
}

// newTestSSHConnectionOptions generates a fresh client key pair (the test
// server accepts any public key) and points it at addr.
func newTestSSHConnectionOptions(t *testing.T, addr string) *SSHConnectionOptions {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate client key: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("failed to marshal client key: %v", err)
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("failed to split test server address: %v", err)
	}

	return &SSHConnectionOptions{
		Host: host,
		Port: port,
		User: "exasol",
		Key:  pem.EncodeToMemory(block),
	}
}

func TestStartSSHSessionReturnsWrappedErrorForUnparseablePrivateKey(t *testing.T) {
	t.Parallel()

	_, err := startSSHSession(&SSHConnectionOptions{
		Host: "127.0.0.1",
		Port: "22",
		User: "exasol",
		Key:  []byte("not a private key"),
	})

	if err == nil || !strings.Contains(err.Error(), "unable to parse private key") {
		t.Fatalf("expected private key parse error, got %v", err)
	}
}

func TestStartSSHSessionMapsDialFailureToErrFailedToConnect(t *testing.T) {
	t.Parallel()

	// Bind then immediately close a listener to obtain a port nothing is
	// listening on, so the dial fails fast and deterministically.
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a port: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}

	options := newTestSSHConnectionOptions(t, addr)

	_, err = startSSHSession(options)

	if !errors.Is(err, ErrFailedToConnect) {
		t.Fatalf("expected ErrFailedToConnect, got %v", err)
	}
}

func TestRunScriptNormalizesCRLFAndRunsBashWithScriptOnStdin(t *testing.T) {
	t.Parallel()

	var receivedCommand string
	var receivedStdin []byte
	addr := startTestSSHServer(t, func(command string, stdin []byte) sshExecResult {
		receivedCommand = command
		receivedStdin = stdin

		return sshExecResult{stdout: "hello from remote\n", exitStatus: 0}
	}, nil)

	remoteHost := NewSshRemote(newTestSSHConnectionOptions(t, addr))

	script := strings.NewReader("echo one\r\necho two\r\n")
	var stdout, stderr strings.Builder

	if err := remoteHost.RunScript(context.Background(), script, &stdout, &stderr); err != nil {
		t.Fatalf("expected script run to succeed, got %v", err)
	}

	if receivedCommand != "/bin/bash" {
		t.Fatalf("expected remote command to be /bin/bash, got %q", receivedCommand)
	}
	if string(receivedStdin) != "echo one\necho two\n" {
		t.Fatalf("expected CRLF to be normalized to LF, got %q", string(receivedStdin))
	}
	if stdout.String() != "hello from remote\n" {
		t.Fatalf("expected remote stdout to be forwarded, got %q", stdout.String())
	}
}

func TestRunScriptReturnsErrorWhenRemoteCommandFails(t *testing.T) {
	t.Parallel()

	addr := startTestSSHServer(t, func(_ string, _ []byte) sshExecResult {
		return sshExecResult{stderr: "boom\n", exitStatus: 1}
	}, nil)

	remoteHost := NewSshRemote(newTestSSHConnectionOptions(t, addr))

	var stdout, stderr strings.Builder
	script := strings.NewReader("false\n")
	err := remoteHost.RunScript(context.Background(), script, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected a non-nil error when the remote command exits non-zero")
	}
	var exitErr *ssh.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitStatus() != 1 {
		t.Fatalf("expected an ssh.ExitError with status 1, got %v", err)
	}
	if stderr.String() != "boom\n" {
		t.Fatalf("expected remote stderr to be forwarded, got %q", stderr.String())
	}
}

// TestShell_ReturnsErrorWhenPtyRequestRejected exercises Shell's real
// startSSHSession + configureSSHSessionPty wiring against the in-process test
// server. The server only understands "exec"/"signal" (see
// handleTestSSHSession) and rejects every other channel request by default,
// so the client-side session.RequestPty call fails deterministically at the
// SSH protocol level -- this needs no real TTY and is stable on any platform.
func TestShell_ReturnsErrorWhenPtyRequestRejected(t *testing.T) {
	t.Parallel()

	addr := startTestSSHServer(t, nil, nil)
	remoteHost := NewSshRemote(newTestSSHConnectionOptions(t, addr))

	var stdout, stderr strings.Builder
	err := remoteHost.Shell(context.Background(), &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "pseudo terminal") {
		t.Fatalf("expected a pty request failure, got %v", err)
	}
}

func TestRunInteractiveCommand_ReturnsErrorWhenPtyRequestRejected(t *testing.T) {
	t.Parallel()

	addr := startTestSSHServer(t, nil, nil)
	remoteHost := NewSshRemote(newTestSSHConnectionOptions(t, addr))

	var stdout, stderr strings.Builder
	err := remoteHost.RunInteractiveCommand(context.Background(), "echo hi", &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "pseudo terminal") {
		t.Fatalf("expected a pty request failure, got %v", err)
	}
}

func TestRunScriptSendsSIGINTToRemoteWhenContextIsCancelled(t *testing.T) {
	t.Parallel()

	received := make(chan string, 1)
	unblock := make(chan struct{})
	addr := startTestSSHServer(t, func(_ string, _ []byte) sshExecResult {
		// Block until the client's cancellation-triggered signal arrives (or
		// time out, so a failure to signal doesn't hang the test forever).
		select {
		case <-unblock:
		case <-time.After(2 * time.Second):
		}

		return sshExecResult{exitStatus: 0}
	}, func(signal string) {
		received <- signal
		close(unblock)
	})

	remoteHost := NewSshRemote(newTestSSHConnectionOptions(t, addr))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Already cancelled: RunScript's watcher goroutine should signal immediately.

	var stdout, stderr strings.Builder
	script := strings.NewReader("sleep 100\n")
	if err := remoteHost.RunScript(ctx, script, &stdout, &stderr); err != nil {
		t.Fatalf("expected script run to complete cleanly after signalling, got %v", err)
	}

	select {
	case signal := <-received:
		if signal != string(ssh.SIGINT) {
			t.Fatalf("expected SIGINT, got signal %q", signal)
		}
	default:
		t.Fatal("expected the remote command to receive a signal, got none")
	}
}
