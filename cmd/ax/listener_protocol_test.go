package main

import (
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// An empty listener protocol printed raw ("80 ()" in describe, "80/" in get)
// reads as a rendering bug. The Gateway API defaults an unspecified listener
// protocol to HTTP.
func TestListenerProtocolDefaultsEmptyToHTTP(t *testing.T) {
	tests := []struct {
		name string
		in   *v1alpha1.Listener
		want string
	}{
		{"nil listener", nil, "HTTP"},
		{"empty protocol", &v1alpha1.Listener{Name: "web", Port: 80}, "HTTP"},
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
