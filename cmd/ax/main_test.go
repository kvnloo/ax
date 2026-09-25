package main

import "testing"

// parseWatchArgs: watch streams only tasks, so the kind word is optional —
// `ax watch mytask` works like suspend/resume's bare-name form. A two-arg
// form with a non-task kind is rejected instead of being silently ignored
// (base used args[1] regardless of args[0]).
func TestParseWatchArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"bare name", []string{"mytask"}, "mytask", false},
		{"kind and name", []string{"task", "mytask"}, "mytask", false},
		{"plural kind and name", []string{"tasks", "mytask"}, "mytask", false},
		{"no args", nil, "", true},
		{"empty args", []string{}, "", true},
		{"unknown kind ignored on base", []string{"banana", "mytask"}, "", true},
		{"non-task kind", []string{"gateway", "mygw"}, "", true},
		{"extra args", []string{"task", "mytask", "extra"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWatchArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseWatchArgs(%v) err = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("parseWatchArgs(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
