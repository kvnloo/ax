// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on a "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tunnel

import (
	"fmt"
	"net"
	"net/http"
	"testing"
)

// serveHealth starts a test HTTP server whose /healthz returns the given
// status code and body, and reports the bound local port.
func serveHealth(t *testing.T, code int, body string) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		_ = http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			_, _ = fmt.Fprint(w, body)
		}))
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestIsTunnelHealthyVerifiesServerIdentity(t *testing.T) {
	// The ax server answers /healthz with 200 and body "ok". A different
	// process may squat the recorded port and answer 200 for its own
	// /healthz; accepting it would route ax traffic to the wrong server.
	if p := serveHealth(t, http.StatusOK, "ok\n"); !IsTunnelHealthy(p) {
		t.Errorf("IsTunnelHealthy(%d) = false for the ax health payload; want true", p)
	}
	if p := serveHealth(t, http.StatusOK, `{"status":"ok"}`); IsTunnelHealthy(p) {
		t.Errorf("IsTunnelHealthy(%d) = true for a foreign 200 payload; want false", p)
	}
	if p := serveHealth(t, http.StatusInternalServerError, "ok\n"); IsTunnelHealthy(p) {
		t.Errorf("IsTunnelHealthy(%d) = true on 500; want false", p)
	}
	if IsTunnelHealthy(0) {
		t.Errorf("IsTunnelHealthy(0) = true; want false")
	}
}
