package redis

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	v1alpha1 "github.com/google/ax/pkg/apis/v1alpha1"
)

func newNilStatusStore(t *testing.T) *Store {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return NewStore(client, Options{StreamName: "ax-task-events"})
}

// A nil status argument must not round-trip as a nil Status on the stored
// record: SaveTask defaults it, so UpdateTaskStatus must too, otherwise the
// store's non-nil Status invariant breaks.
func TestUpdateTaskStatusNilDefaults(t *testing.T) {
	s := newNilStatusStore(t)
	ctx := context.Background()

	if err := s.SaveTask(ctx, &v1alpha1.Task{
		Metadata: &v1alpha1.ObjectMeta{Atespace: "default", Name: "nilstatus"},
	}); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}

	if err := s.UpdateTaskStatus(ctx, "default", "nilstatus", nil); err != nil {
		t.Fatalf("UpdateTaskStatus(nil): %v", err)
	}

	got, err := s.GetTask(ctx, "default", "nilstatus")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status == nil {
		t.Fatal("Status is nil after UpdateTaskStatus(nil); want defaulted TaskStatus")
	}
}
