package main

import "testing"

// normalizeKind must accept singular/plural in any case. The old code ran
// TrimSuffix before ToLower, and TrimSuffix is case-sensitive, so
// "ax delete TASKS foo" errored while "ax delete tasks foo" worked.
func TestNormalizeKind(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"task", "Task", false},
		{"tasks", "Task", false},
		{"Task", "Task", false},
		{"TASKS", "Task", false},
		{"Tasks", "Task", false},
		{"gateway", "Gateway", false},
		{"gateways", "Gateway", false},
		{"GATEWAYS", "Gateway", false},
		{"Gateway", "Gateway", false},
		{"workspace", "Workspace", false},
		{"WORKSPACES", "Workspace", false},
		{"model", "Model", false},
		{"MODELS", "Model", false},
		{"Models", "Model", false},
		{"frobnicate", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := normalizeKind(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeKind(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeKind(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeKind(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
