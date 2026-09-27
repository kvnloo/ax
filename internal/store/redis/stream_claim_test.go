package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// A consumer that reads an event and crashes before Ack leaves the entry in the
// group's pending list. A restarted worker joins with a fresh consumer name; it
// must reclaim the idle pending entry instead of never seeing it.
func TestSubscriptionReclaimsPendingAfterConsumerCrash(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	st := NewStore(client, Options{ReadBlock: 50 * time.Millisecond, ClaimIdle: 50 * time.Millisecond})

	ctx := context.Background()
	subA, err := st.Subscribe(ctx, "g", "consumer-a")
	if err != nil {
		t.Fatalf("subscribe A: %v", err)
	}
	if err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: defaultStreamName,
		Values: map[string]interface{}{"action": "delete", "atespace": "default", "name": "t1"},
	}).Err(); err != nil {
		t.Fatalf("xadd: %v", err)
	}

	if _, err := subA.Next(ctx); err != nil {
		t.Fatalf("consumer A next: %v", err)
	}
	_ = subA.Close() // crash: no Ack

	time.Sleep(150 * time.Millisecond) // let the pending entry go idle

	subB, err := st.Subscribe(ctx, "g", "consumer-b")
	if err != nil {
		t.Fatalf("subscribe B: %v", err)
	}
	ctx2, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ev, err := subB.Next(ctx2)
	if err != nil {
		t.Fatalf("consumer B never received the crashed consumer's event: %v", err)
	}
	if ev.Name != "t1" || ev.Action != "delete" || ev.Atespace != "default" {
		t.Fatalf("reclaimed wrong event: %+v", ev)
	}
	if err := subB.Ack(ctx, ev); err != nil {
		t.Fatalf("ack: %v", err)
	}

	// The normal path still works: a brand-new entry is delivered via ">".
	if err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: defaultStreamName,
		Values: map[string]interface{}{"action": "reconcile", "atespace": "default", "name": "t2"},
	}).Err(); err != nil {
		t.Fatalf("xadd: %v", err)
	}
	ev2, err := subB.Next(ctx2)
	if err != nil {
		t.Fatalf("consumer B next (new entry): %v", err)
	}
	if ev2.Name != "t2" {
		t.Fatalf("expected t2, got %+v", ev2)
	}
}

// An entry claimed moments ago (idle < ClaimIdle) must not be stolen from a
// consumer that is still processing it.
func TestSubscriptionDoesNotStealFreshPending(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	st := NewStore(client, Options{ReadBlock: 50 * time.Millisecond, ClaimIdle: 5 * time.Second})

	ctx := context.Background()
	subD, err := st.Subscribe(ctx, "g", "consumer-d")
	if err != nil {
		t.Fatalf("subscribe D: %v", err)
	}
	if err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: defaultStreamName,
		Values: map[string]interface{}{"action": "reconcile", "atespace": "default", "name": "t1"},
	}).Err(); err != nil {
		t.Fatalf("xadd: %v", err)
	}

	if _, err := subD.Next(ctx); err != nil {
		t.Fatalf("consumer D next: %v", err)
	}
	// D is still processing; E joins immediately.
	subE, err := st.Subscribe(ctx, "g", "consumer-e")
	if err != nil {
		t.Fatalf("subscribe E: %v", err)
	}
	ctx2, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	if _, err := subE.Next(ctx2); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("consumer E should not steal D's fresh entry, got err=%v", err)
	}
}
