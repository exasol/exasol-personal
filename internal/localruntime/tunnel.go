// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
)

const localLoopbackHost = "127.0.0.1"

const proxyCopyDirectionCount = 2

type LoopbackForwarder struct {
	listener   net.Listener
	guestHost  string
	guestPort  int
	closeOnce  sync.Once
	closeError error
	wg         sync.WaitGroup
}

func StartLoopbackForwarder(
	ctx context.Context,
	hostPort int,
	guestHost string,
	guestPort int,
) (*LoopbackForwarder, error) {
	listener, err := (&net.ListenConfig{}).Listen(
		ctx,
		"tcp",
		net.JoinHostPort(localLoopbackHost, strconv.Itoa(hostPort)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s:%d: %w", localLoopbackHost, hostPort, err)
	}

	forwarder := &LoopbackForwarder{
		listener:  listener,
		guestHost: guestHost,
		guestPort: guestPort,
	}
	forwarder.wg.Add(1)
	go forwarder.acceptLoop(ctx)

	return forwarder, nil
}

func (f *LoopbackForwarder) Close() error {
	f.closeOnce.Do(func() {
		f.closeError = f.listener.Close()
		f.wg.Wait()
	})

	if f.closeError != nil && !errors.Is(f.closeError, net.ErrClosed) {
		return f.closeError
	}

	return nil
}

func (f *LoopbackForwarder) acceptLoop(ctx context.Context) {
	defer f.wg.Done()

	for {
		clientConn, err := f.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}

			continue
		}

		f.wg.Add(1)
		go f.proxyConnection(ctx, clientConn)
	}
}

func (f *LoopbackForwarder) proxyConnection(ctx context.Context, clientConn net.Conn) {
	defer f.wg.Done()
	defer clientConn.Close()

	guestConn, err := (&net.Dialer{}).DialContext(
		ctx,
		"tcp",
		net.JoinHostPort(f.guestHost, strconv.Itoa(f.guestPort)),
	)
	if err != nil {
		return
	}
	defer guestConn.Close()

	var copyWG sync.WaitGroup
	copyWG.Add(proxyCopyDirectionCount)

	go func() {
		defer copyWG.Done()
		_, _ = io.Copy(guestConn, clientConn)
		_ = guestConn.Close()
	}()

	go func() {
		defer copyWG.Done()
		_, _ = io.Copy(clientConn, guestConn)
		_ = clientConn.Close()
	}()

	copyWG.Wait()
}
