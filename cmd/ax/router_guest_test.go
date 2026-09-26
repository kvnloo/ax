// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
)

// stubPortForward replaces portForwardFn for the test, returning port (or
// pfErr) and counting cleanup calls.
func stubPortForward(t *testing.T, port int, pfErr error) *int {
	t.Helper()
	old := portForwardFn
	cleanups := new(int)
	portForwardFn = func(ctx context.Context, kubeContext, namespace, target string, remotePort int) (int, func(), error) {
		if pfErr != nil {
			return 0, nil, pfErr
		}
		return port, func() { *cleanups++ }, nil
	}
	t.Cleanup(func() { portForwardFn = old })
	return cleanups
}

func testTCPPort(t *testing.T, addr string) int {
	t.Helper()
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// acceptAndHold returns a listener that accepts TCP connections and never
// speaks: the half-open "dead router" shape.
func acceptAndHold(t *testing.T) net.Listener {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			c, err := lis.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = lis.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	return lis
}

// TestDialRouterGuestDeadRouter proves the defect: the port-forward is up but
// nothing serves gRPC behind it. dialRouterGuest must fail fast with a clear
// error (and release the port-forward) instead of handing back a client whose
// first Exec RPC blows up.
func TestDialRouterGuestDeadRouter(t *testing.T) {
	lis := acceptAndHold(t)
	cleanups := stubPortForward(t, testTCPPort(t, lis.Addr().String()), nil)

	start := time.Now()
	c, cleanup, err := dialRouterGuest("some-context", "space/actor")
	if err == nil || !strings.Contains(err.Error(), "not serving gRPC") {
		t.Fatalf("dialRouterGuest(dead router) err = %v, want 'not serving gRPC'", err)
	}
	if c != nil || cleanup != nil {
		t.Fatal("want nil client and cleanup on failure")
	}
	if *cleanups != 1 {
		t.Fatalf("port-forward cleanup called %d times, want 1", *cleanups)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("dead router took %v, want fast failure", d)
	}
}

// TestDialRouterGuestLiveRouter: a real gRPC handshake behind the
// port-forward dials cleanly and the returned cleanup releases the forward.
func TestDialRouterGuestLiveRouter(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer() // no services needed: the handshake is the gate
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	cleanups := stubPortForward(t, testTCPPort(t, lis.Addr().String()), nil)

	c, cleanup, err := dialRouterGuest("some-context", "space/actor")
	if err != nil {
		t.Fatalf("dialRouterGuest(live router): %v", err)
	}
	if c == nil || cleanup == nil {
		t.Fatal("want non-nil client and cleanup on success")
	}
	cleanup()
	_ = c.Close()
	if *cleanups != 1 {
		t.Fatalf("port-forward cleanup called %d times, want 1", *cleanups)
	}
}

// TestDialRouterGuestPortForwardError: a failed port-forward propagates
// without touching the guest client.
func TestDialRouterGuestPortForwardError(t *testing.T) {
	stubPortForward(t, 0, errors.New("no cluster"))

	c, cleanup, err := dialRouterGuest("some-context", "space/actor")
	if err == nil || !strings.Contains(err.Error(), "port-forward") {
		t.Fatalf("dialRouterGuest(port-forward error) err = %v, want 'port-forward'", err)
	}
	if c != nil || cleanup != nil {
		t.Fatal("want nil client and cleanup on failure")
	}
}
