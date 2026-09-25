package main

import "testing"

// parseGlobalFlags must reject a value flag with no following value. The old
// inline loop in main silently swallowed a trailing -a/--server/--context/-n
// (the "i+1 < len(args)" guard had no else), so "ax get tasks -a" ran against
// the default atespace with no error. It must also stop flag parsing at
// "--": the old loop ate flags after "--", so "ax ssh mytask -- -a" silently
// dropped the "-a" and opened /bin/sh instead of running the "-a" command.
func TestParseGlobalFlags(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantCmd   string
		wantFlags globalFlags
		wantArgs  []string
		wantErr   bool
	}{
		{
			"flags before command",
			[]string{"-a", "prod", "get", "tasks"},
			"get",
			globalFlags{atespace: "prod", axNamespace: "ax-system"},
			[]string{"tasks"},
			false,
		},
		{
			"flags after command",
			[]string{"get", "tasks", "--server", "http://x:8080"},
			"get",
			globalFlags{atespace: "default", explicitServer: "http://x:8080", axNamespace: "ax-system"},
			[]string{"tasks"},
			false,
		},
		{
			"equals form",
			[]string{"--atespace=prod", "--namespace=kubens", "describe", "task", "x"},
			"describe",
			globalFlags{atespace: "prod", axNamespace: "kubens"},
			[]string{"task", "x"},
			false,
		},
		{
			"context flag",
			[]string{"--context", "myctx", "ssh", "mytask"},
			"ssh",
			globalFlags{atespace: "default", kubeContext: "myctx", axNamespace: "ax-system"},
			[]string{"mytask"},
			false,
		},
		{
			"double dash passes through verbatim",
			[]string{"ssh", "mytask", "--", "-a"},
			"ssh",
			globalFlags{atespace: "default", axNamespace: "ax-system"},
			[]string{"mytask", "--", "-a"},
			false,
		},
		{
			"double dash with command",
			[]string{"ssh", "mytask", "--", "ls", "-la"},
			"ssh",
			globalFlags{atespace: "default", axNamespace: "ax-system"},
			[]string{"mytask", "--", "ls", "-la"},
			false,
		},
		{"trailing -a errors", []string{"get", "tasks", "-a"}, "", globalFlags{}, nil, true},
		{"trailing --atespace errors", []string{"--atespace"}, "", globalFlags{}, nil, true},
		{"trailing --server errors", []string{"get", "--server"}, "", globalFlags{}, nil, true},
		{"trailing --context errors", []string{"--context"}, "", globalFlags{}, nil, true},
		{"trailing -n errors", []string{"-n"}, "", globalFlags{}, nil, true},
		{
			"no command",
			[]string{"-a", "prod"},
			"",
			globalFlags{atespace: "prod", axNamespace: "ax-system"},
			nil,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, flags, args, err := parseGlobalFlags(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseGlobalFlags(%v) = %q, want error", tt.args, cmd)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseGlobalFlags(%v) unexpected error: %v", tt.args, err)
			}
			if cmd != tt.wantCmd {
				t.Errorf("parseGlobalFlags(%v) cmd = %q, want %q", tt.args, cmd, tt.wantCmd)
			}
			if flags != tt.wantFlags {
				t.Errorf("parseGlobalFlags(%v) flags = %+v, want %+v", tt.args, flags, tt.wantFlags)
			}
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("parseGlobalFlags(%v) args = %v, want %v", tt.args, args, tt.wantArgs)
			}
			for i := range args {
				if args[i] != tt.wantArgs[i] {
					t.Fatalf("parseGlobalFlags(%v) args = %v, want %v", tt.args, args, tt.wantArgs)
				}
			}
		})
	}
}
