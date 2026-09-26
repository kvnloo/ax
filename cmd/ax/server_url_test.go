package main

import "testing"

func TestNormalizeServerURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"http://localhost:8080", "http://localhost:8080"},
		{"http://localhost:8080/", "http://localhost:8080"},
		{"http://localhost:8080///", "http://localhost:8080"},
		{"  http://localhost:8080  ", "http://localhost:8080"},
		{"http://localhost:8080/ \n", "http://localhost:8080"},
		{"https://ax.example.com/", "https://ax.example.com"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeServerURL(tt.in); got != tt.want {
			t.Errorf("normalizeServerURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
