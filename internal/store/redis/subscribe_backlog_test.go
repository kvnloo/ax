package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func newTestStore(t *testing.T) (*Store, *goredis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return NewStore(client, Options{
		StreamName:    "ax-task-events",
		ReadBatchSize: 10,
		ReadBlock:     50 * time.Millisecond,
	}), client
}

// A brand-new consumer group must see events published before the group was
// created. The worker loop is purely event-driven with no resync, so a task
// created while no worker was running would otherwise never be reconciled.
func TestSubscribeReplaysPreExistingBacklog(t *testing.T) {
	s, client := newTestStore(t)
	ctx := context.Background()

	// Publish BEFORE the group exists (e.g. task applied while no worker ran).
	if err := client.XAdd(ctx, &goredis.XAddArgs{
		Stream: "ax-task-events",
		Values: map[string]any{"atespace": "default", "name": "t1", "action": "reconcile"},
	}).Err(); err != nil {
		t.Fatal(err)
	}

	sub, err := s.Subscribe(ctx, "backlog-group", "c1")
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	rctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ev, err := sub.Next(rctx)
	if err != nil {
		t.Fatalf("new group missed pre-existing backlog event: %v", err)
	}
	if ev.Name != "t1" || ev.Atespace != "default" || ev.Action != "reconcile" {
		t.Fatalf("unexpected event: %+v", ev)
	}
	if err := sub.Ack(ctx, ev); err != nil {
		t.Fatal(err)
	}
}

// Existing groups keep their position: a restart must not replay the whole
// stream, only entries the group never delivered.
func TestSubscribeExistingGroupKeepsPosition(t *testing.T) {
	s, client := newTestStore(t)
	ctx := context.Background()

	add := func(name string) {
		t.Helper()
		if err := client.XAdd(ctx, &goredis.XAddArgs{
			Stream: "ax-task-events",
			Values: map[string]any{"atespace": "default", "name": name, "action": "reconcile"},
		}).Err(); err != nil {
			t.Fatal(err)
		}
	}
	add("before")

	sub1, err := s.Subscribe(ctx, "restart-group", "c1")
	if err != nil {
		t.Fatal(err)
	}
	rctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := sub1.Next(rctx); err != nil {
		t.Fatalf("first consumer missed event: %v", err)
	}
	sub1.Close() // group now exists with its position past "before"

	// A second consumer joining the existing group must not see "before" again.
	sub2, err := s.Subscribe(ctx, "restart-group", "c2")
	if err != nil {
		t.Fatal(err)
	}
	defer sub2.Close()
	add("after")

	got, err := sub2.Next(rctx)
	if err != nil {
		t.Fatalf("second consumer missed new event: %v", err)
	}
	if got.Name != "after" {
		t.Fatalf("existing group replayed old backlog: %+v", got)
	}
}
