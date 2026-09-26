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
	"strings"
	"testing"
)

func TestTunnelNotFoundErrNamesContext(t *testing.T) {
	err := tunnelNotFoundErr("prod", false, nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), `"prod"`) {
		t.Fatalf("expected error to name the context, got %q", err)
	}
	if strings.Contains(err.Error(), ".json") {
		t.Fatalf("error must not leak the tunnel state-file path: %q", err)
	}
}

func TestTunnelNotFoundErrListsKnownTunnels(t *testing.T) {
	err := tunnelNotFoundErr("prod", false, []string{"prod-us", "staging"})
	msg := err.Error()
	for _, want := range []string{`"prod-us"`, `"staging"`, "ax tunnel list"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected %q in error %q", want, msg)
		}
	}
}

func TestTunnelNotFoundErrCurrentContext(t *testing.T) {
	err := tunnelNotFoundErr("dev", true, []string{"prod"})
	msg := err.Error()
	if !strings.Contains(msg, "current context") {
		t.Fatalf("expected 'current context' in %q", msg)
	}
	if !strings.Contains(msg, `"dev"`) {
		t.Fatalf("expected context name in %q", msg)
	}
}
