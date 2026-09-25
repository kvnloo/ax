package main

import "testing"

// TestParseSuspendArgs pins the suspend/resume argument contract:
// bare name or "task"/"tasks" kind prefix; anything more is an error.
func TestParseSuspendArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr string
	}{
		{"no args", nil, "", "usage"},
		{"bare name", []string{"foo"}, "foo", ""},
		{"lone kind word is a bare name", []string{"task"}, "task", ""},
		{"kind prefix singular", []string{"task", "foo"}, "foo", ""},
		{"kind prefix plural", []string{"tasks", "foo"}, "foo", ""},
		{"extra after bare name", []string{"foo", "bar"}, "", `unexpected argument "bar"`},
		{"extra after kind prefix", []string{"task", "foo", "bar"}, "", `unexpected argument "bar"`},
		{"three extras", []string{"task", "foo", "bar", "baz"}, "", `unexpected argument "bar"`},
		{"non-task kind word", []string{"gateway", "foo"}, "", `unexpected argument "foo"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSuspendArgs(tt.args)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("parseSuspendArgs(%v) unexpected error: %v", tt.args, err)
				}
				if got != tt.want {
					t.Fatalf("parseSuspendArgs(%v) = %q, want %q", tt.args, got, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("parseSuspendArgs(%v) = %q, want error containing %q", tt.args, got, tt.wantErr)
			}
			if got != "" {
				t.Fatalf("parseSuspendArgs(%v) name = %q on error, want empty", tt.args, got)
			}
			if got := err.Error(); !contains(got, tt.wantErr) {
				t.Fatalf("parseSuspendArgs(%v) error = %q, want it to contain %q", tt.args, got, tt.wantErr)
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestSuspendExtraArgsErrorBeforeDial proves runSuspend surfaces the parse
// error without attempting any network I/O: pointed at a dead server, the
// error must be the parse error, not a connection error.
func TestSuspendExtraArgsErrorBeforeDial(t *testing.T) {
	err := runSuspend("http://127.0.0.1:1", "test", []string{"foo", "bar"})
	if err == nil {
		t.Fatal("runSuspend(foo bar) expected error, got nil")
	}
	if got := err.Error(); !contains(got, `unexpected argument "bar"`) {
		t.Fatalf("runSuspend(foo bar) error = %q, want parse error", got)
	}

	err = runResume("http://127.0.0.1:1", "test", []string{"task", "foo", "bar"})
	if err == nil {
		t.Fatal("runResume(task foo bar) expected error, got nil")
	}
	if got := err.Error(); !contains(got, `unexpected argument "bar"`) {
		t.Fatalf("runResume(task foo bar) error = %q, want parse error", got)
	}
}
