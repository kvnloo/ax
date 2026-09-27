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
	"testing"

	"github.com/google/ax/internal/guest"
)

type fakePortForward struct {
	cleanedUp     bool
	gotKubeCtx    string
	gotNamespace  string
	gotResource   string
	gotRemotePort int
	failErr       error
}

func (f *fakePortForward) run(ctx context.Context, kubeContext, namespace, targetResource string, remotePort int) (int, func(), error) {
	f.gotKubeCtx = kubeContext
	f.gotNamespace = namespace
	f.gotResource = targetResource
	f.gotRemotePort = remotePort
	if f.failErr != nil {
		return 0, nil, f.failErr
	}
	return 41234, func() { f.cleanedUp = true }, nil
}

// A failed dial must release the port-forward: the kubectl child runs in its
// own process group and would otherwise be orphaned, holding the local port.
func TestDialGuestViaRouterCleansUpOnDialFailure(t *testing.T) {
	pf := &fakePortForward{}
	dialErr := errors.New("connection refused")
	dial := func(target, targetActor string) (*guest.Client, error) {
		if target != "127.0.0.1:41234" {
			t.Errorf("dial target = %q, want 127.0.0.1:41234", target)
		}
		if targetActor != "default/my-actor" {
			t.Errorf("dial targetActor = %q, want default/my-actor", targetActor)
		}
		return nil, dialErr
	}

	client, cleanup, err := dialGuestViaRouter("my-ctx", "default/my-actor", pf.run, dial)
	if !errors.Is(err, dialErr) {
		t.Fatalf("err = %v, want wrapped %v", err, dialErr)
	}
	if client != nil {
		t.Fatalf("client = %v, want nil on dial failure", client)
	}
	if cleanup != nil {
		t.Fatal("cleanup is non-nil on dial failure")
	}
	if !pf.cleanedUp {
		t.Fatal("port-forward was not released after dial failure; the kubectl child is orphaned")
	}
}

// The happy path wires the router port-forward and returns its cleanup.
func TestDialGuestViaRouterSuccess(t *testing.T) {
	pf := &fakePortForward{}
	wantClient := &guest.Client{}
	dial := func(target, targetActor string) (*guest.Client, error) {
		return wantClient, nil
	}

	client, cleanup, err := dialGuestViaRouter("my-ctx", "default/my-actor", pf.run, dial)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if client != wantClient {
		t.Fatalf("client = %v, want the dialed client", client)
	}
	if cleanup == nil {
		t.Fatal("cleanup is nil")
	}
	if pf.gotKubeCtx != "my-ctx" || pf.gotNamespace != "ate-system" ||
		pf.gotResource != "svc/atenet-router" || pf.gotRemotePort != 80 {
		t.Fatalf("port-forward args = %q %q %q %d", pf.gotKubeCtx, pf.gotNamespace, pf.gotResource, pf.gotRemotePort)
	}
	cleanup()
	if !pf.cleanedUp {
		t.Fatal("returned cleanup did not release the port-forward")
	}
}

// A port-forward failure propagates without attempting a dial.
func TestDialGuestViaRouterPortForwardFailure(t *testing.T) {
	pfErr := errors.New("kubectl not found")
	pf := &fakePortForward{failErr: pfErr}
	dialed := false
	dial := func(target, targetActor string) (*guest.Client, error) {
		dialed = true
		return &guest.Client{}, nil
	}

	_, _, err := dialGuestViaRouter("my-ctx", "default/my-actor", pf.run, dial)
	if !errors.Is(err, pfErr) {
		t.Fatalf("err = %v, want wrapped %v", err, pfErr)
	}
	if dialed {
		t.Fatal("dialed despite port-forward failure")
	}
}
