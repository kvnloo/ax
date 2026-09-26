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

package guest

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
)

// TestWaitReadyRealServer proves the direct-path gate accepts a genuine gRPC
// endpoint: WaitReady returns nil once the connection is READY.
func TestWaitReadyRealServer(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	defer srv.Stop()
	go srv.Serve(lis)

	c, err := Dial(lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady on a live gRPC server: %v", err)
	}
}

// TestWaitReadyHalfOpenPath proves the defect the direct-path gate fixes: a
// listener that accepts TCP but never speaks gRPC (stale worker IP,
// intercepting middlebox) passes the old TCP probe, yet WaitReady refuses to
// declare it usable, so runSSH falls back to the atenet-router instead of
// failing at Exec with no fallback left.
func TestWaitReadyHalfOpenPath(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			// Hold the connection open without speaking: the half-open case.
			go func() { <-make(chan struct{}); _ = conn }()
		}
	}()

	c, err := Dial(lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := c.WaitReady(ctx); err == nil {
		t.Fatal("WaitReady declared a half-open (non-gRPC) TCP path ready")
	}
}

// TestWaitReadyNilConn guards the zero-value Client.
func TestWaitReadyNilConn(t *testing.T) {
	c := &Client{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.WaitReady(ctx); err == nil {
		t.Fatal("WaitReady on a nil connection: want error")
	}
}
