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
	"strings"
	"testing"
)

// TestParseSSHArgs is the regression test for the silent-drop defect: tokens
// between the task name and the "--" separator used to be discarded without
// error (e.g. `ax ssh foo typo -- ls` ran `ls` and dropped "typo"). Red on
// base (parseSSHArgs is undefined there; and the old inline loop dropped the
// tokens). Green with the fail-fast parse.
func TestParseSSHArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantName string
		wantCmd  []string
		wantErr  string // "" = no error
	}{
		{"no args", nil, "", nil, "usage: ax ssh <task-name> [-- command...]"},
		{"bare task name defaults to shell", []string{"foo"}, "foo", []string{"/bin/sh"}, ""},
		{"no separator passes command through", []string{"foo", "ls", "-la"}, "foo", []string{"ls", "-la"}, ""},
		{"separator then command", []string{"foo", "--", "ls", "-la"}, "foo", []string{"ls", "-la"}, ""},
		{"separator alone defaults to shell", []string{"foo", "--"}, "foo", []string{"/bin/sh"}, ""},
		{"tokens before separator are an error", []string{"foo", "typo", "--", "ls"}, "", nil, "unexpected argument(s) before --"},
		{"command word before separator is an error", []string{"foo", "ls", "--", "-la"}, "", nil, "unexpected argument(s) before --"},
		{"separator in command passes through", []string{"foo", "--", "grep", "--", "-x"}, "foo", []string{"grep", "--", "-x"}, ""},
		{"double separator passes through", []string{"foo", "--", "--", "ls"}, "foo", []string{"--", "ls"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, cmd, err := parseSSHArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got name=%q cmd=%v err=%v", tt.wantErr, name, cmd, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if name != tt.wantName {
				t.Errorf("task name = %q, want %q", name, tt.wantName)
			}
			if !reflect.DeepEqual(cmd, tt.wantCmd) {
				t.Errorf("command = %v, want %v", cmd, tt.wantCmd)
			}
		})
	}
}
