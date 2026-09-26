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
	"reflect"
	"testing"
)

// Words before "--" are part of the remote command, not flag parsing: the
// old loop replaced them with the words after "--", silently dropping the
// command itself (`ax ssh mytask echo -- -n foo` ran `-n foo`).
func TestParseSSHCommandDashDashAppends(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"command before dashes", []string{"echo", "--", "-n", "foo"}, []string{"echo", "-n", "foo"}},
		{"no command words", []string{}, nil},
		{"only dashes", []string{"--"}, nil},
		{"dashes first", []string{"--", "echo", "hi"}, []string{"echo", "hi"}},
		{"no dashes", []string{"echo", "hello"}, []string{"echo", "hello"}},
		{"help passthrough", []string{"--", "--help"}, []string{"--help"}},
		{"trailing dashes keep command", []string{"echo", "--"}, []string{"echo"}},
		{"second dashes verbatim", []string{"echo", "--", "--", "x"}, []string{"echo", "--", "x"}},
	}
	for _, c := range cases {
		got := parseSSHCommand(c.args)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: parseSSHCommand(%q) = %q, want %q", c.name, c.args, got, c.want)
		}
	}
}
