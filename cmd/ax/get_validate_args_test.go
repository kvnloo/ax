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

	"github.com/google/ax/pkg/apis/v1alpha1"
)


func TestValidateGetArgs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		kind    string
		wantErr string
	}{
		{"no args", nil, "", "specify resource to get"},
		{"unsupported kind", []string{"bogus"}, "", "unsupported kind"},
		{"extra arg", []string{"tasks", "a", "b"}, "", "unexpected extra argument"},
		{"extra arg gateway", []string{"gateway", "g", "x"}, "", "unexpected extra argument"},
		{"list", []string{"tasks"}, v1alpha1.KindTask, ""},
		{"get one", []string{"task", "t1"}, v1alpha1.KindTask, ""},
		{"whitespace kind", []string{" task"}, v1alpha1.KindTask, ""},
		{"plural model", []string{"models"}, v1alpha1.KindModel, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind, err := validateGetArgs(tc.args)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if kind != tc.kind {
					t.Fatalf("kind = %q, want %q", kind, tc.kind)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

