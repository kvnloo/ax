package main

import (
	"testing"
	"time"
)

// A task whose CreationTimestamp lies in the future (client/server clock
// skew, restored backups) must not render as a negative age like "-5s".
func TestFormatAgeNegativeClamped(t *testing.T) {
	for _, d := range []time.Duration{
		-5 * time.Second,
		-90 * time.Second,
		-3 * time.Hour,
	} {
		if got := formatAge(d); got != "0s" {
			t.Errorf("formatAge(%v) = %q, want %q (negative age)", d, got, "0s")
		}
	}
}

// Pin the existing buckets so the clamp doesn't shift normal rendering.
func TestFormatAgeBuckets(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{45 * time.Second, "45s"},
		{90 * time.Second, "1m"},
		{59 * time.Minute, "59m"},
		{2 * time.Hour, "2h"},
		{23 * time.Hour, "23h"},
		{30 * time.Hour, "1d"},
	}
	for _, c := range cases {
		if got := formatAge(c.d); got != c.want {
			t.Errorf("formatAge(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
