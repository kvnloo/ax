package main

import "testing"

// parseDescribeArgs: an unknown kind must error, not silently describe a task
// (base fell through to GetTask for any unrecognized kind).
func TestParseDescribeArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantKind string
		wantName string
		wantErr  bool
	}{
		{"singular kind", []string{"task", "mytask"}, "Task", "mytask", false},
		{"plural kind", []string{"tasks", "mytask"}, "Task", "mytask", false},
		{"gateway plural", []string{"gateways", "mygw"}, "Gateway", "mygw", false},
		{"case-insensitive", []string{"Model", "m1"}, "Model", "m1", false},
		{"no args", nil, "", "", true},
		{"kind only", []string{"task"}, "", "", true},
		{"unknown kind falls through on base", []string{"banana", "mytask"}, "", "", true},
		{"extra args", []string{"task", "mytask", "extra"}, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, name, err := parseDescribeArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseDescribeArgs(%v) err = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
			if !tt.wantErr && (kind != tt.wantKind || name != tt.wantName) {
				t.Fatalf("parseDescribeArgs(%v) = (%q, %q), want (%q, %q)",
					tt.args, kind, name, tt.wantKind, tt.wantName)
			}
		})
	}
}
