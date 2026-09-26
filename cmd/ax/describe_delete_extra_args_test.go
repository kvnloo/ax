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

// `ax describe task foo bar` silently ignored the trailing argument;
// `ax delete task foo bar` did the same. Like watch's extra-positional
// check, these run before any dial, so an unreachable server URL still
// yields the usage error. Red on base: got a connection error instead.
func TestRunDescribeRejectsExtraArgs(t *testing.T) {
	err := runDescribe("http://127.0.0.1:1", "default", []string{"task", "foo", "bar"})
	if err == nil || !strings.Contains(err.Error(), "unexpected extra argument") {
		t.Fatalf("runDescribe err = %v, want usage error for extra argument", err)
	}
}

func TestRunDeleteRejectsExtraArgs(t *testing.T) {
	err := runDelete("http://127.0.0.1:1", "default", []string{"task", "foo", "bar"})
	if err == nil || !strings.Contains(err.Error(), "unexpected extra argument") {
		t.Fatalf("runDelete err = %v, want usage error for extra argument", err)
	}
}
