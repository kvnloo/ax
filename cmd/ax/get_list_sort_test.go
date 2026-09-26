package main

import (
	"context"
	"strings"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

// `ax get` list output must be deterministic: the server returns items in
// store order, which can reshuffle across restarts. kubectl sorts by name;
// ax must too.
func TestRunGetTasksSortedByName(t *testing.T) {
	fc := &fakeAXClient{listedTasks: []*v1alpha1.Task{
		{Metadata: &v1alpha1.ObjectMeta{Name: "bravo", Atespace: "default"}},
		{Metadata: &v1alpha1.ObjectMeta{Name: "alpha", Atespace: "default"}},
	}}
	out := captureStdout(t, func() {
		if err := runGetWithClient(context.Background(), fc, "default", []string{"tasks"}); err != nil {
			t.Errorf("runGetWithClient: %v", err)
		}
	})
	ia := strings.Index(out, "alpha")
	ib := strings.Index(out, "bravo")
	if ia < 0 || ib < 0 {
		t.Fatalf("both rows must appear, got %q", out)
	}
	if ia > ib {
		t.Errorf("rows not sorted by name (alpha should precede bravo), got %q", out)
	}
}

// A nil task in the list must not panic the sort.
func TestRunGetTasksSortNilSafe(t *testing.T) {
	fc := &fakeAXClient{listedTasks: []*v1alpha1.Task{
		{Metadata: &v1alpha1.ObjectMeta{Name: "zeta", Atespace: "default"}},
		{},
	}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("sort panicked on nil metadata: %v", r)
		}
	}()
	out := captureStdout(t, func() {
		if err := runGetWithClient(context.Background(), fc, "default", []string{"tasks"}); err != nil {
			t.Errorf("runGetWithClient: %v", err)
		}
	})
	if !strings.Contains(out, "zeta") {
		t.Errorf("zeta row missing, got %q", out)
	}
}

func TestSortByNameStable(t *testing.T) {
	got := sortByName([]string{"b", "a", "c"}, func(s string) string { return s })
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("sortByName = %v", got)
	}
}
