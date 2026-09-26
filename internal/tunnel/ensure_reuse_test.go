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

package tunnel

import (
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

// A stale state file whose recorded process is gone must not be adopted when
// an unrelated local service answers 200 on /healthz at the recycled port.
// EnsureServerURL must reject it and try a fresh tunnel instead of handing
// the caller a URL that points at the foreign service.
func TestEnsureServerURLRejectsForeignServiceOnRecycledPort(t *testing.T) {
	// Foreign service answering 200 on /healthz at a real bound port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})}
	go srv.Serve(ln) //nolint:errcheck
	defer srv.Close()

	withTempAXHome(t)

	// Stale state file: recorded PID is long gone, port now serves the
	// foreign service.
	stale := &TunnelInfo{
		Context:   "reusetest",
		Namespace: "ax-system",
		Service:   "ax-server",
		Port:      port,
		PID:       999999999,
		CreatedAt: time.Now().Add(-time.Hour),
	}
	if err := SaveTunnel(stale); err != nil {
		t.Fatal(err)
	}

	url, err := EnsureServerURL(Options{Context: "reusetest"})
	foreign := fmt.Sprintf("http://127.0.0.1:%d", port)
	if url == foreign {
		t.Fatalf("adopted foreign service from stale state file: %s", url)
	}
	// The state file must have been reaped as part of the rejection.
	if _, serr := GetTunnel("reusetest"); serr == nil {
		t.Fatalf("stale state file was not reaped")
	}
	if err == nil {
		t.Logf("note: a fresh tunnel was started instead (url=%s)", url)
	}
}
