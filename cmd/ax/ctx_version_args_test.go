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

func TestValidateNoArgs(t *testing.T) {
	if err := validateNoArgs(nil, "ctx"); err != nil {
		t.Fatalf("nil args: got %v", err)
	}
	if err := validateNoArgs([]string{}, "version"); err != nil {
		t.Fatalf("empty args: got %v", err)
	}
	if err := validateNoArgs([]string{"foo"}, "ctx"); err == nil {
		t.Fatal("extra positional accepted, want usage error")
	} else if !strings.Contains(err.Error(), "usage: ax ctx") {
		t.Fatalf("error %q does not carry usage", err)
	}
	if err := validateNoArgs([]string{"foo", "bar"}, "version"); err == nil {
		t.Fatal("extra positionals accepted, want usage error")
	}
	// Flags that survive the top-level loop are extra args too.
	if err := validateNoArgs([]string{"-h"}, "ctx"); err == nil {
		t.Fatal("-h accepted, want usage error")
	}
}
