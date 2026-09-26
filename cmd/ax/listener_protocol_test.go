package main

import (
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// An empty listener protocol printed raw ("80 ()" in describe, "80/" in get)
// reads as a rendering bug. The protocol field is optional on the wire and
// nothing in this codebase defaults an empty value, so we render it only
// when present rather than fabricating a default.
func TestListenerProtocolOmitsEmpty(t *testing.T) {
	tests := []struct {
		name string
		in   *v1alpha1.Listener
		want string
	}{
		{"nil listener", nil, ""},
		{"empty protocol", &v1alpha1.Listener{Name: "web", Port: 80}, ""},
		{"explicit protocol preserved", &v1alpha1.Listener{Name: "web", Port: 80, Protocol: "HTTPS"}, "HTTPS"},
		{"nonstandard protocol preserved", &v1alpha1.Listener{Name: "db", Port: 5432, Protocol: "TCP"}, "TCP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := listenerProtocol(tt.in); got != tt.want {
				t.Errorf("listenerProtocol() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A padded protocol (" http ") rendered with stray spaces in the get-list
// "80/ http " and describe "80 ( http )" views. displayPhase trims padded
// phases, so the protocol rule trims too. Red on base: listenerProtocol
// returned the raw value, so padded rendered untrimmed.
func TestListenerProtocolTrimsPadded(t *testing.T) {
	for in, want := range map[string]string{
		" http ":  "http",
		"\tTCP\n": "TCP",
		"HTTPS":   "HTTPS",
		"   ":     "",
	} {
		l := &v1alpha1.Listener{Name: "web", Port: 80, Protocol: in}
		if got := listenerProtocol(l); got != want {
			t.Errorf("listenerProtocol(%q) = %q, want %q", in, got, want)
		}
	}
}
