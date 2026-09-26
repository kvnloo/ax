package main

import "testing"

// `ax describe model` used to print nested parameter values with %v, leaking
// Go syntax ("map[x:y]", "[p q]") into user-facing output.
func TestFormatParamValue(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"scalar string", "fast", "fast"},
		{"scalar number", 0.7, "0.7"},
		{"scalar bool", true, "true"},
		{"flat slice", []any{"a", "b"}, "[a, b]"},
		{"nested map", map[string]any{"x": "y"}, "{x=y}"},
		{
			"deep nesting",
			map[string]any{"a": 1, "nested": map[string]any{"x": "y"}, "list": []any{"p", "q"}},
			"{a=1, list=[p, q], nested={x=y}}",
		},
		{"empty map", map[string]any{}, "{}"},
		{"empty slice", []any{}, "[]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatParamValue(c.value); got != c.want {
				t.Fatalf("formatParamValue(%v) = %q, want %q", c.value, got, c.want)
			}
		})
	}
}
