// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package remote

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type SSHRemote struct {
	options *SSHConnectionOptions
}

var _ Remote = (*SSHRemote)(nil)

func NewSshRemote(options *SSHConnectionOptions) *SSHRemote {
	return &SSHRemote{
		options: options,
	}
}

func (s *SSHRemote) Shell(ctx context.Context, out io.Writer, errOut io.Writer) error {
	session, err := startSSHSession(s.options)
	if err != nil {
		return err
	}
	defer session.Close()

	restore, err := configureSSHSessionPty(session, out, errOut)
	if err != nil {
		return err
	}
	defer restore()

	shell_session_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		<-shell_session_ctx.Done()
		session.Close() // nolint: gosec
	}()

	// Start remote shell
	if err := session.Shell(); err != nil {
		return fmt.Errorf("%w: Failed to start shell", err)
	}

	// Wait for session to end
	err = session.Wait()
	if err != nil {
		// print error type
		if errors.Is(err, &ssh.ExitError{}) {
			// This means that the last command before exiting the remote shell failed or was
			// stopped by a signal. Most likely, that signal was sent by the user. In either
			// case, this failure does not indicate that the interactive shell itself has
			// failed and should be ignored.
			return nil
		}

		return err
	}

	return nil
}

func (s *SSHRemote) RunScript(ctx context.Context, script io.Reader, out, errOut io.Writer) error {
	// Normalize Windows CRLF to Unix LF to prevent Bash errors (remove carriage returns)
	// Ensures scripts run correctly across all platforms
	fBytes, err := io.ReadAll(script)
	if err != nil {
		return err
	}

	fBytes = bytes.ReplaceAll(fBytes, []byte("\r\n"), []byte("\n"))

	session, err := startSSHSession(s.options)
	if err != nil {
		return err
	}
	defer session.Close()

	session.Stdout = out
	session.Stderr = errOut

	session.Stdin = bytes.NewReader(fBytes)

	go func() {
		<-ctx.Done()
		err := session.Signal(ssh.SIGINT)
		if err != nil {
			slog.Error("failed to send SIGINT to remote script", "error", err.Error())
		}
	}()

	return session.Run("/bin/bash")
}

type SSHConnectionOptions struct {
	Host string
	Port string
	User string
	Key  []byte
}

func startSSHSession(options *SSHConnectionOptions) (*ssh.Session, error) {
	signer, err := ssh.ParsePrivateKey(options.Key)
	if err != nil {
		return nil, fmt.Errorf("unable to parse private key: %w", err)
	}

	// SSH config
	config := &ssh.ClientConfig{
		User: options.User,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		// We do not know the key of the server prior to the first connect
		// nolint: gosec
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	slog.Debug("dialing ssh connection",
		"host", options.Host,
		"port", options.Port,
		"user", options.User,
		"keySha256", fmt.Sprintf("%x", sha256.Sum256(options.Key)),
	)

	client, err := ssh.Dial("tcp", net.JoinHostPort(options.Host, options.Port), config)
	if err != nil {
		return nil, ErrFailedToConnect
	}

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create session", err)
	}

	return session, nil
}

func configureSSHSessionPty(
	session *ssh.Session,
	out, errOut io.Writer,
) (configureSSHSessionPtyRestoreFunc, error) {
	// Get current terminal size
	stdinFd := int(os.Stdin.Fd())

	width, height, err := term.GetSize(stdinFd)
	if err != nil {
		width, height = 80, 24 // fallback
	}

	// Terminal modes. 14400 is a common default speed.
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400, // nolint: mnd
		ssh.TTY_OP_OSPEED: 14400, // nolint: mnd
	}

	// Request PTY
	if err := session.RequestPty("xterm-256color", height, width, modes); err != nil {
		return nil, fmt.Errorf("%w: Request for pseudo terminal failed", err)
	}

	// Set terminal to raw mode
	oldState, err := term.MakeRaw(stdinFd)
	if err != nil {
		return nil, fmt.Errorf("%w: Failed to set raw mode", err)
	}

	// Wire up I/O
	session.Stdin = os.Stdin
	session.Stdout = out
	session.Stderr = errOut

	// Handle terminal resize
	complete := handleTerminalResize(session)

	return func() {
		complete()

		err := term.Restore(stdinFd, oldState)
		if err != nil {
			slog.Error("error restoring terminal state", "error", err.Error())
		}
	}, nil
}

type configureSSHSessionPtyRestoreFunc = func()

// Re-exported ssh.ExitError.
type ExitError = ssh.ExitError
