package main

import "testing"

func TestHelpRequested(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"bare help flag", []string{"--help"}, true},
		{"bare short flag", []string{"-h"}, true},
		{"before command", []string{"-h", "version"}, true},
		{"after command", []string{"get", "tasks", "--help"}, true},
		{"after subcommand args", []string{"get", "task", "foo", "-h"}, true},
		{"no help flag", []string{"get", "tasks"}, false},
		{"no args", nil, false},
		{"global flags only", []string{"-a", "foo", "get"}, false},
		// "--" starts ssh passthrough: a help flag after it belongs to the
		// remote command, not the CLI.
		{"ssh passthrough", []string{"ssh", "mytask", "--", "--help"}, false},
		{"double dash then short", []string{"ssh", "mytask", "--", "-h"}, false},
		{"lone double dash", []string{"--", "--help"}, false},
		// =form values containing a help-looking suffix are not flags.
		{"equals value", []string{"apply", "--file=-h.yaml"}, false},
		// A help-looking token consumed as a flag value still counts as a
		// help request: help wins over an absurd flag value.
		{"help as flag value", []string{"-a", "-h", "get"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := helpRequested(tc.args); got != tc.want {
				t.Fatalf("helpRequested(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}
